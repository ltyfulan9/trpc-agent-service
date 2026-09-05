package adminauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const testBootstrap = "bootstrap-token-that-is-at-least-32-bytes"
const testTenantToken = "tenant-admin-token-that-is-at-least-32-bytes"

func TestAuthenticatorBootstrapAndScopedPrincipal(t *testing.T) {
	authenticator, err := NewAuthenticator(testBootstrap, `[
		{"id":"alice","token":"`+testTenantToken+`","role":"tenant_admin","tenantIds":["tenant-a"]}
	]`)
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, err := PrincipalFromContext(request.Context())
		if err != nil {
			t.Fatal(err)
		}
		if request.URL.Path == "/bootstrap" && principal.Role != RolePlatformAdmin {
			t.Fatalf("bootstrap role = %q", principal.Role)
		}
		if request.URL.Path == "/tenant" && (principal.ID != "alice" || !principal.AllowsTenant("tenant-a") || principal.AllowsTenant("tenant-b")) {
			t.Fatalf("scoped principal = %#v", principal)
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	for path, token := range map[string]string{"/bootstrap": testBootstrap, "/tenant": testTenantToken} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		authenticator.Middleware(next).ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d", path, response.Code)
		}
	}
}

func TestAuthenticatorRejectsMalformedAndDuplicateCredentials(t *testing.T) {
	if _, err := NewAuthenticator("short", ""); err == nil {
		t.Fatal("weak bootstrap token accepted")
	}
	if _, err := NewAuthenticator(testBootstrap, `[{"id":"x","token":"`+testBootstrap+`","role":"auditor","tenantIds":["t"]}]`); err == nil {
		t.Fatal("duplicate token accepted")
	}
	if _, err := NewAuthenticator(testBootstrap, `[{"id":"x","token":"`+testTenantToken+`","role":"unknown"}]`); err == nil {
		t.Fatal("unknown role accepted")
	}
	if _, err := NewAuthenticator(testBootstrap, `[{"id":"x","token":"`+testTenantToken+`","role":"tenant_admin","tenantIds":["*"]}]`); err == nil {
		t.Fatal("wildcard tenant scope accepted")
	}
}

func TestMiddlewareRejectsMultipleAuthorizationHeaders(t *testing.T) {
	authenticator, err := NewAuthenticator(testBootstrap, "")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Add("Authorization", "Bearer "+testBootstrap)
	request.Header.Add("Authorization", "Bearer "+testBootstrap)
	response := httptest.NewRecorder()
	authenticator.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not run")
	})).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestRequireMethodsAndTenantScope(t *testing.T) {
	principal := Principal{ID: "auditor", Role: RoleAuditor, tenants: map[string]struct{}{"tenant-a": {}}}
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { writer.WriteHeader(http.StatusNoContent) })
	handler := RequireMethods(map[string]Permission{
		http.MethodGet: PermissionTenantRead,
		http.MethodPut: PermissionTenantWrite,
	}, next)

	for method, want := range map[string]int{http.MethodGet: http.StatusNoContent, http.MethodPut: http.StatusForbidden} {
		request := httptest.NewRequest(method, "/", nil)
		request = request.WithContext(ContextWithPrincipal(request.Context(), principal))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != want {
			t.Fatalf("%s status = %d, want %d", method, response.Code, want)
		}
	}

	ctx := ContextWithPrincipal(httptest.NewRequest(http.MethodGet, "/", nil).Context(), principal)
	if _, err := RequireTenant(ctx, "tenant-a"); err != nil {
		t.Fatalf("allowed tenant rejected: %v", err)
	}
	if _, err := RequireTenant(ctx, "tenant-b"); err != ErrForbidden {
		t.Fatalf("cross-tenant error = %v", err)
	}
}

func TestExecutionReconcilePermissionIsPlatformAdminOnly(t *testing.T) {
	if !(Principal{ID: "platform", Role: RolePlatformAdmin}).Has(PermissionExecutionReconcile) {
		t.Fatal("platform admin must be allowed to reconcile executions")
	}
	for _, role := range []Role{RoleTenantAdmin, RoleReleaseManager, RoleAuditor} {
		if (Principal{ID: "operator", Role: role}).Has(PermissionExecutionReconcile) {
			t.Fatalf("role %q must not reconcile executions", role)
		}
	}
}
