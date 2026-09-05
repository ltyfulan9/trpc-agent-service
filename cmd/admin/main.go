//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/adminauth"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/controlplane"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/governance"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/health"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/platformtool"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/runtimeplane"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/storage"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/telemetry"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/tenant"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/worker"
)

func main() {
	log.Println("Starting Admin API...")
	if err := telemetry.ConfigureMetricLabelsFromEnv(); err != nil {
		log.Fatalf("configure tenant metric labels: error=%s", telemetry.StableErrorCode(err))
	}

	// Load configuration
	dbURL := requireEnv("DATABASE_URL")
	if err := storage.ValidateServicePostgresURL(dbURL, os.Getenv("DATABASE_ALLOW_INSECURE") == "true"); err != nil {
		log.Fatal("invalid DATABASE_URL")
	}
	port := getEnv("PORT", "8081")
	activeKeyID, masterKeys, err := tenant.LoadKeyRingFromEnv(32)
	if err != nil {
		log.Fatalf("configure tenant encryption key ring: error=%s", telemetry.StableErrorCode(err))
	}
	backendProfiles, err := storage.LoadBackendProfileValidator(
		requireEnv("STORAGE_BACKEND_PROFILES"),
	)
	if err != nil {
		log.Fatalf("configure storage backend profiles: error=%s", telemetry.StableErrorCode(err))
	}
	dataPlaneProfiles, err := runtimeplane.LoadProfileValidator(requireEnv("DATA_PLANE_PROFILES"))
	if err != nil {
		log.Fatalf("configure runtime data-plane profiles: error=%s", telemetry.StableErrorCode(err))
	}
	// Fail-closed admin auth: startup refuses weak or missing credentials.
	adminToken := requireSecret("ADMIN_API_TOKEN", 32)
	adminAuthenticator, err := adminauth.NewAuthenticator(adminToken, os.Getenv("ADMIN_PRINCIPALS_JSON"))
	if err != nil {
		log.Fatalf("configure admin authentication: error=%s", telemetry.StableErrorCode(err))
	}
	traceShutdown, err := telemetry.SetupTracing(context.Background(), "agent-admin")
	if err != nil {
		log.Fatalf("configure tracing: error=%s", telemetry.StableErrorCode(err))
	}
	defer traceShutdown(context.Background())

	// Initialize tenant repository
	tenantRepo, err := tenant.NewSQLRepository("postgres", dbURL)
	if err != nil {
		log.Fatalf("failed to create tenant repository: error=%s", telemetry.StableErrorCode(err))
	}
	// Coordinate graceful shutdown: fail readiness first, drain in-flight admin
	// requests, then close the database.
	shutdown := health.NewCoordinator()
	shutdown.OnShutdown("database", func(context.Context) error { return tenantRepo.Close() })

	// Initialize tenant service
	tenantService, err := tenant.NewServiceWithKeyRing(
		tenantRepo,
		activeKeyID,
		masterKeys,
		tenant.WithStorageConfigValidator(func(_ context.Context, tenantID string, config tenant.StorageConfig) error {
			if err := backendProfiles.ValidateTenantStorage(tenantID, config); err != nil {
				return err
			}
			return dataPlaneProfiles.ValidateTenantStorage(tenantID, config)
		}),
	)
	if err != nil {
		log.Fatalf("configure tenant encryption: error=%s", telemetry.StableErrorCode(err))
	}
	controlDB, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("open control-plane database: error=%s", telemetry.StableErrorCode(err))
	}
	controlDB.SetMaxOpenConns(25)
	controlDB.SetMaxIdleConns(5)
	controlDB.SetConnMaxLifetime(30 * time.Minute)
	controlDB.SetConnMaxIdleTime(5 * time.Minute)
	if err := controlDB.PingContext(context.Background()); err != nil {
		controlDB.Close()
		log.Fatalf("ping control-plane database: error=%s", telemetry.StableErrorCode(err))
	}
	shutdown.OnShutdown("control-plane-database", func(context.Context) error { return controlDB.Close() })
	toolCatalog, err := platformtool.NewMCPAdmissionResolver(os.Getenv("MCP_PROFILES"))
	if err != nil {
		log.Fatalf("configure MCP admission catalog: error=%s", telemetry.StableErrorCode(err))
	}
	// The admission registry describes the runtime implementations installed in
	// this deployment. The default composition intentionally exposes only the
	// bundled LLM runtime; future factories must be registered explicitly at
	// both the Admin and Worker composition roots.
	runtimeFactories := worker.NewRuntimeAgentRegistry()
	controlService := controlplane.NewService(controlDB, func(
		ctx context.Context,
		tenantID string,
		snapshot *controlplane.VersionSnapshot,
	) error {
		return validateVersionSnapshot(ctx, tenantService, toolCatalog, runtimeFactories, tenantID, snapshot)
	})
	// Admin reconciliation shares the same advisory-fencing authority as the
	// strict Worker. Using the compatibility recorder here would let an
	// operator decision race an in-flight Session/Memory operation.
	executionRecorder := controlplane.NewExecutionRecorderWithAdvisoryFencing(controlDB)
	approvalStore := governance.NewPostgresApprovalStore(controlDB)

	// Setup HTTP routes
	mux := http.NewServeMux()

	// Tenant CRUD
	mux.HandleFunc("/api/v1/tenants", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listTenants(w, r, tenantService)
		case http.MethodPost:
			createTenant(w, r, tenantService)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/tenants/", func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenantIDFromPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}

		switch r.Method {
		case http.MethodGet:
			getTenant(w, r, tenantService, tenantID)
		case http.MethodPut:
			updateTenant(w, r, tenantService, tenantID)
		case http.MethodDelete:
			deleteTenant(w, r, tenantService, tenantID)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	registerControlPlaneRoutes(mux, controlService, executionRecorder, approvalStore)

	healthChecker := health.New(
		health.WithDatabase(tenantRepo),
		health.WithDrainState(shutdown),
	)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		body, statusCode := healthChecker.Report(r.Context())

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(body)
	})
	mux.HandleFunc("/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	mux.Handle("/metrics", telemetry.MetricsHandlerFromEnv())

	// Only the tenant API is drain-tracked; /health and /metrics must stay
	// answerable for the whole drain.
	handler := http.NewServeMux()
	handler.Handle("/health", mux)
	handler.Handle("/live", mux)
	handler.Handle("/metrics", mux)
	protect := func(name string, permissions map[string]adminauth.Permission) http.Handler {
		tracked := shutdown.Middleware(telemetry.HTTPMiddleware(name, mux))
		return noStoreAdminResponses(adminAuthenticator.Middleware(adminauth.RequireMethods(permissions, tracked)))
	}
	handler.Handle("/api/v1/tenants", protect("admin.tenants", map[string]adminauth.Permission{
		http.MethodGet: adminauth.PermissionTenantRead, http.MethodPost: adminauth.PermissionTenantCreate,
	}))
	handler.Handle("/api/v1/tenants/", protect("admin.tenant", map[string]adminauth.Permission{
		http.MethodGet: adminauth.PermissionTenantRead, http.MethodPut: adminauth.PermissionTenantWrite,
		http.MethodDelete: adminauth.PermissionTenantDelete,
	}))
	handler.Handle("/api/v1/agent-apps", protect("admin.agent-apps", map[string]adminauth.Permission{
		http.MethodPost: adminauth.PermissionAgentWrite,
	}))
	handler.Handle("/api/v1/agent-versions", protect("admin.agent-versions", map[string]adminauth.Permission{
		http.MethodPost: adminauth.PermissionAgentWrite,
	}))
	handler.Handle("/api/v1/agent-versions/", protect("admin.agent-version", map[string]adminauth.Permission{
		http.MethodPost: adminauth.PermissionAgentPublish,
	}))
	handler.Handle("/api/v1/deployments", protect("admin.deployments", map[string]adminauth.Permission{
		http.MethodPost: adminauth.PermissionAgentDeploy,
	}))
	handler.Handle("/api/v1/execution-reconciliations", protect("admin.execution-reconciliations", map[string]adminauth.Permission{
		http.MethodPost: adminauth.PermissionExecutionReconcile,
	}))
	handler.Handle("/api/v1/tool-approvals/", protect("admin.tool-approvals", map[string]adminauth.Permission{
		http.MethodGet:  adminauth.PermissionToolApprovalRead,
		http.MethodPost: adminauth.PermissionToolApprovalGrant,
	}))
	handler.Handle("/api/v1/tool-approvals", protect("admin.tool-approvals-list", map[string]adminauth.Permission{
		http.MethodGet: adminauth.PermissionToolApprovalRead,
	}))

	// Start HTTP server
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	// Run until SIGTERM/SIGINT, then stop the listener, drain in-flight
	// work, and release dependencies. See health.ServeUntilSignal.
	if err := health.ServeUntilSignal(server, shutdown, "Admin API"); err != nil {
		log.Fatalf("admin shutdown failed: error=%s", telemetry.StableErrorCode(err))
	}
}

func listTenants(w http.ResponseWriter, r *http.Request, service tenant.Service) {
	principal, err := adminauth.PrincipalFromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var tenants []*tenant.Tenant
	if principal.Role == adminauth.RolePlatformAdmin {
		tenants, err = service.ListTenants(r.Context())
	} else {
		// Scoped principals must never trigger a broad read that decrypts every
		// tenant before the HTTP layer filters the result. Fail closed when the
		// repository does not provide the SQL-constrained capability.
		ids := principal.TenantIDs()
		if len(ids) == 0 {
			tenants = []*tenant.Tenant{}
		} else if scoped, ok := service.(interface {
			ListTenantsForIDs(context.Context, []string) ([]*tenant.Tenant, error)
		}); ok {
			ordered := make([]string, 0, len(ids))
			for id := range ids {
				ordered = append(ordered, id)
			}
			sort.Strings(ordered)
			tenants, err = scoped.ListTenantsForIDs(r.Context(), ordered)
		} else {
			err = tenant.ErrScopedTenantListingUnsupported
		}
	}
	if err != nil {
		log.Printf("failed to list tenants for principal %s: error=%s", principal.ID, telemetry.StableErrorCode(err))
		http.Error(w, "Tenant listing unavailable", http.StatusServiceUnavailable)
		return
	}

	redacted := make([]*tenant.Tenant, 0, len(tenants))
	for _, tn := range tenants {
		if principal.AllowsTenant(tn.ID) {
			redacted = append(redacted, redactTenant(tn))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(redacted)
}

func createTenant(w http.ResponseWriter, r *http.Request, service tenant.Service) {
	actor, ok := requireAdminActor(w, r)
	if !ok {
		return
	}
	var req struct {
		Name   string              `json:"name"`
		Config tenant.TenantConfig `json:"config"`
	}

	if !decodeJSON(w, r, &req) {
		return
	}
	if tenantConfigContainsRedacted(req.Config) {
		http.Error(w, "redaction placeholders are not valid credentials for a new tenant", http.StatusBadRequest)
		return
	}
	if err := tenant.ValidateDistributedStorage(req.Config.Storage); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := tenant.ContextWithAuditActor(r.Context(), actor)
	t, err := service.CreateTenant(ctx, req.Name, req.Config)
	if err != nil {
		if errors.Is(err, tenant.ErrInvalidTenantConfig) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, tenant.ErrTenantAlreadyExists) {
			http.Error(w, "Tenant already exists", http.StatusConflict)
			return
		}
		log.Printf("failed to create tenant: error=%s", telemetry.StableErrorCode(err))
		http.Error(w, "Failed to create tenant", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(redactTenant(t))
}

func getTenant(w http.ResponseWriter, r *http.Request, service tenant.Service, tenantID string) {
	if _, err := adminauth.RequireTenant(r.Context(), tenantID); err != nil {
		writeAdminAuthorizationError(w, err)
		return
	}
	t, err := service.GetTenant(r.Context(), tenantID)
	if err != nil {
		if err == tenant.ErrTenantNotFound {
			http.Error(w, "Tenant not found", http.StatusNotFound)
		} else {
			log.Printf("failed to get tenant: error=%s", telemetry.StableErrorCode(err))
			http.Error(w, "Failed to get tenant", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(redactTenant(t))
}

func updateTenant(w http.ResponseWriter, r *http.Request, service tenant.Service, tenantID string) {
	if _, err := adminauth.RequireTenant(r.Context(), tenantID); err != nil {
		writeAdminAuthorizationError(w, err)
		return
	}
	actor, ok := requireAdminActor(w, r)
	if !ok {
		return
	}
	var t tenant.Tenant
	if !decodeJSON(w, r, &t) {
		return
	}

	// Admin responses intentionally mask credentials. A client commonly edits
	// that response and PUTs it back, so treating the mask as a new credential
	// would permanently destroy the real secret. Merge only masked/omitted
	// credentials from the current plaintext snapshot; an explicit non-mask
	// value still rotates the credential.
	current, err := service.GetTenant(r.Context(), tenantID)
	if err != nil {
		if err == tenant.ErrTenantNotFound {
			http.Error(w, "Tenant not found", http.StatusNotFound)
		} else {
			log.Printf("failed to load tenant before update: error=%s", telemetry.StableErrorCode(err))
			http.Error(w, "Failed to update tenant", http.StatusInternalServerError)
		}
		return
	}
	preserveMaskedSecrets(current, &t)
	if tenantContainsRedacted(&t) {
		http.Error(w, "redaction placeholder does not match an existing credential", http.StatusBadRequest)
		return
	}
	if err := tenant.ValidateDistributedStorage(t.Storage); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	t.ID = tenantID
	ctx := tenant.ContextWithAuditActor(r.Context(), actor)
	if err := service.UpdateTenant(ctx, &t); err != nil {
		if errors.Is(err, tenant.ErrTenantConflict) {
			http.Error(w, "Tenant configuration changed; reload and retry", http.StatusConflict)
			return
		}
		if errors.Is(err, tenant.ErrInvalidTenantConfig) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("failed to update tenant: error=%s", telemetry.StableErrorCode(err))
		http.Error(w, "Failed to update tenant", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(redactTenant(&t))
}

const redactedValue = "***REDACTED***"

func preserveMaskedSecrets(current, update *tenant.Tenant) {
	models := make(map[string]tenant.ModelConfig, len(current.Models))
	for _, model := range current.Models {
		models[model.Provider+"\x00"+model.ModelName] = model
	}
	for i := range update.Models {
		key := update.Models[i].Provider + "\x00" + update.Models[i].ModelName
		if previous, ok := models[key]; ok && isMaskedOrOmitted(update.Models[i].APIKey) {
			if previous.APIKey != "" {
				update.Models[i].APIKey = previous.APIKey
			} else if update.Models[i].APIKeyRef == "" {
				// A reference-backed credential is not materialized by the
				// control-plane read. Preserve its handle when a client sends
				// back the redacted/omitted representation unchanged.
				update.Models[i].APIKeyRef = previous.APIKeyRef
			}
		}
	}

	channels := make(map[string]tenant.ChannelBinding, len(current.Channels))
	for _, binding := range current.Channels {
		channels[channelIdentity(binding)] = binding
	}
	for i := range update.Channels {
		if previous, ok := channels[channelIdentity(update.Channels[i])]; ok {
			if isMaskedOrOmitted(update.Channels[i].Token) {
				if previous.Token != "" {
					update.Channels[i].Token = previous.Token
				} else if update.Channels[i].TokenRef == "" {
					update.Channels[i].TokenRef = previous.TokenRef
				}
			}
			if isMaskedOrOmitted(update.Channels[i].Secret) {
				if previous.Secret != "" {
					update.Channels[i].Secret = previous.Secret
				} else if update.Channels[i].SecretRef == "" {
					update.Channels[i].SecretRef = previous.SecretRef
				}
			}
			if isMaskedOrOmitted(update.Channels[i].EncodingAESKey) {
				if previous.EncodingAESKey != "" {
					update.Channels[i].EncodingAESKey = previous.EncodingAESKey
				} else if update.Channels[i].EncodingAESKeyRef == "" {
					update.Channels[i].EncodingAESKeyRef = previous.EncodingAESKeyRef
				}
			}
			if update.Channels[i].WebhookKey == "" {
				update.Channels[i].WebhookKey = previous.WebhookKey
			}
			if update.Channels[i].AccountID == "" {
				update.Channels[i].AccountID = previous.AccountID
			}
			if update.Channels[i].Config == nil {
				update.Channels[i].Config = cloneStringMap(previous.Config)
			} else {
				preserveMapSecrets(previous.Config, update.Channels[i].Config, []string{"encoding_aes_key", "corp_secret", "token", "secret"})
			}
		}
	}
	if update.Storage.SessionBackend == "" {
		update.Storage.SessionBackend = current.Storage.SessionBackend
	}
	if update.Storage.MemoryBackend == "" {
		update.Storage.MemoryBackend = current.Storage.MemoryBackend
	}
	if update.Storage.SessionProfile == "" {
		update.Storage.SessionProfile = current.Storage.SessionProfile
	}
	if update.Storage.MemoryProfile == "" {
		update.Storage.MemoryProfile = current.Storage.MemoryProfile
	}
	if update.Storage.SessionConfig == nil {
		update.Storage.SessionConfig = cloneStringMap(current.Storage.SessionConfig)
	} else {
		preserveMapSecrets(current.Storage.SessionConfig, update.Storage.SessionConfig, storageSecretKeys())
	}
	if update.Storage.MemoryConfig == nil {
		update.Storage.MemoryConfig = cloneStringMap(current.Storage.MemoryConfig)
	} else {
		preserveMapSecrets(current.Storage.MemoryConfig, update.Storage.MemoryConfig, storageSecretKeys())
	}
}

func preserveMapSecrets(current, update map[string]string, keys []string) {
	seen := make(map[string]struct{}, len(keys)+len(current)+len(update))
	for _, key := range keys {
		seen[key] = struct{}{}
	}
	for key := range current {
		seen[key] = struct{}{}
	}
	for key := range update {
		seen[key] = struct{}{}
	}
	for key := range seen {
		if !isSensitiveConfigKey(key) {
			continue
		}
		if isMaskedOrOmitted(update[key]) && current[key] != "" {
			update[key] = current[key]
		}
	}
}

func isSensitiveConfigKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"), ".", "_"))
	switch normalized {
	case "token", "secret", "password", "passwd", "dsn", "url", "api_key", "apikey",
		"access_key", "access_token", "secret_key", "credential", "credentials", "authorization",
		"auth", "corp_secret", "encoding_aes_key":
		return true
	}
	for _, part := range strings.Split(normalized, "_") {
		switch part {
		case "token", "secret", "password", "passwd", "apikey", "credential", "credentials", "authorization":
			return true
		}
	}
	return false
}

func storageSecretKeys() []string {
	return []string{"dsn", "url", "password", "access_key", "secret_key", "token"}
}

func channelIdentity(binding tenant.ChannelBinding) string {
	if binding.AccountID != "" {
		return binding.Type + "\x00" + binding.AccountID
	}
	if binding.WebhookKey != "" {
		return binding.Type + "\x00" + binding.WebhookKey
	}
	return binding.Type + "\x00" + binding.AppID
}

func isMaskedOrOmitted(value string) bool {
	return value == "" || value == redactedValue
}

func tenantConfigContainsRedacted(config tenant.TenantConfig) bool {
	return tenantContainsRedacted(&tenant.Tenant{
		Models: config.Models, Channels: config.Channels, Storage: config.Storage,
	})
}

func tenantContainsRedacted(value *tenant.Tenant) bool {
	for _, model := range value.Models {
		if model.APIKey == redactedValue {
			return true
		}
	}
	for _, binding := range value.Channels {
		if binding.Token == redactedValue || binding.Secret == redactedValue || binding.EncodingAESKey == redactedValue {
			return true
		}
		for _, secret := range binding.Config {
			if secret == redactedValue {
				return true
			}
		}
	}
	for _, config := range []map[string]string{value.Storage.SessionConfig, value.Storage.MemoryConfig} {
		for _, secret := range config {
			if secret == redactedValue {
				return true
			}
		}
	}
	return false
}

func deleteTenant(w http.ResponseWriter, r *http.Request, service tenant.Service, tenantID string) {
	if _, err := adminauth.RequireTenant(r.Context(), tenantID); err != nil {
		writeAdminAuthorizationError(w, err)
		return
	}
	actor, ok := requireAdminActor(w, r)
	if !ok {
		return
	}
	ctx := tenant.ContextWithAuditActor(r.Context(), actor)
	if err := service.DeleteTenant(ctx, tenantID); err != nil {
		if err == tenant.ErrTenantNotFound {
			http.Error(w, "Tenant not found", http.StatusNotFound)
		} else {
			log.Printf("failed to delete tenant: error=%s", telemetry.StableErrorCode(err))
			http.Error(w, "Failed to delete tenant", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func requireEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("%s is required", key)
	}
	return value
}

func requireSecret(key string, minimumLength int) string {
	value := os.Getenv(key)
	if len(value) < minimumLength {
		log.Fatalf("%s must be configured with at least %d characters", key, minimumLength)
	}
	return value
}

// redactMask returns a fixed placeholder for any non-empty secret so callers
// can tell "set" from "unset" without exposing the value.
func redactMask(s string) string {
	if s == "" {
		return ""
	}
	return redactedValue
}

// redactTenant returns a copy of t with all secret-bearing fields masked, so
// the Admin API never emits decrypted model API keys or channel credentials in
// its JSON responses. Slices are copied so the caller's tenant is untouched.
func redactTenant(t *tenant.Tenant) *tenant.Tenant {
	if t == nil {
		return nil
	}
	c := *t
	c.Agents = make([]tenant.AgentConfig, len(t.Agents))
	copy(c.Agents, t.Agents)
	for i := range c.Agents {
		c.Agents[i].Tools = append([]string(nil), t.Agents[i].Tools...)
		c.Agents[i].Metadata = cloneStringMap(t.Agents[i].Metadata)
		if t.Agents[i].Runtime != nil {
			runtimeCopy := *t.Agents[i].Runtime
			runtimeCopy.Nodes = append([]tenant.AgentRuntimeNode(nil), t.Agents[i].Runtime.Nodes...)
			for nodeIndex := range runtimeCopy.Nodes {
				runtimeCopy.Nodes[nodeIndex].Tools = append(
					[]string(nil), t.Agents[i].Runtime.Nodes[nodeIndex].Tools...,
				)
			}
			runtimeCopy.Edges = append([]tenant.AgentRuntimeEdge(nil), t.Agents[i].Runtime.Edges...)
			c.Agents[i].Runtime = &runtimeCopy
		}
	}
	c.Models = make([]tenant.ModelConfig, len(t.Models))
	copy(c.Models, t.Models)
	for i := range c.Models {
		c.Models[i].APIKey = redactMask(c.Models[i].APIKey)
	}
	c.Channels = make([]tenant.ChannelBinding, len(t.Channels))
	copy(c.Channels, t.Channels)
	for i := range c.Channels {
		c.Channels[i].Config = cloneStringMap(t.Channels[i].Config)
		c.Channels[i].WebhookURL = redactWebhookURL(c.Channels[i].WebhookURL)
		c.Channels[i].Token = redactMask(c.Channels[i].Token)
		c.Channels[i].Secret = redactMask(c.Channels[i].Secret)
		c.Channels[i].EncodingAESKey = redactMask(c.Channels[i].EncodingAESKey)
		redactMapSecrets(c.Channels[i].Config, []string{"encoding_aes_key", "corp_secret", "token", "secret"})
	}
	c.Storage.SessionConfig = cloneStringMap(t.Storage.SessionConfig)
	c.Storage.MemoryConfig = cloneStringMap(t.Storage.MemoryConfig)
	redactMapSecrets(c.Storage.SessionConfig, storageSecretKeys())
	redactMapSecrets(c.Storage.MemoryConfig, storageSecretKeys())
	return &c
}

// noStoreAdminResponses prevents browsers, proxies, and shared caches from
// retaining tenant configuration or control-plane responses. It is applied at
// the protected route boundary so errors are covered as well as successful JSON.
func noStoreAdminResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// redactWebhookURL preserves a usable public route while removing URL
// components that commonly carry credentials (userinfo, query, and fragment).
// An invalid configured value is replaced wholesale rather than echoed into an
// Admin response.
func redactWebhookURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return redactedValue
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	return u.String()
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func redactMapSecrets(config map[string]string, keys []string) {
	known := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		known[key] = struct{}{}
	}
	for key, value := range config {
		if _, explicitlyKnown := known[key]; explicitlyKnown || isSensitiveConfigKey(key) {
			if value != "" {
				config[key] = redactedValue
			}
		}
	}
}

func tenantIDFromPath(path string) (string, bool) {
	const prefix = "/api/v1/tenants/"
	if len(path) <= len(prefix) || path[:len(prefix)] != prefix {
		return "", false
	}
	tenantID := path[len(prefix):]
	if tenantID == "" || len(tenantID) > 64 {
		return "", false
	}
	for _, character := range tenantID {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') && character != '-' && character != '_' {
			return "", false
		}
	}
	return tenantID, true
}

func writeAdminAuthorizationError(w http.ResponseWriter, err error) {
	if errors.Is(err, adminauth.ErrUnauthenticated) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	http.Error(w, "Forbidden", http.StatusForbidden)
}
