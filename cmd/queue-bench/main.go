package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/migrations"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/queuebench"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/reliable"
)

func main() {
	var dsn, output string
	var messages int
	var work, timeout time.Duration
	flag.StringVar(&dsn, "database-url", os.Getenv("CAPACITY_DATABASE_URL"), "isolated PostgreSQL URL; database name must start with queuebench_")
	flag.IntVar(&messages, "messages-per-tenant", 64, "finite backlog per tenant")
	flag.StringVar(&output, "output", "", "write JSON results to this path instead of stdout")
	flag.DurationVar(&work, "work", time.Millisecond, "synthetic claim-to-complete work")
	flag.DurationVar(&timeout, "timeout", 5*time.Minute, "timeout for each case")
	flag.Parse()
	if err := run(dsn, messages, output, work, timeout); err != nil {
		log.Fatal(err)
	}
}

func run(dsn string, messages int, output string, work, timeout time.Duration) error {
	if strings.TrimSpace(dsn) == "" {
		return errors.New("CAPACITY_DATABASE_URL or -database-url is required")
	}
	dbName, err := databaseName(dsn)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(dbName, "queuebench_") {
		return fmt.Errorf("capacity database %q must have queuebench_ prefix", dbName)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("open capacity database: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(64)
	db.SetMaxIdleConns(16)
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping capacity database: %w", err)
	}
	if err := migrations.NewRunner(db).Up(ctx); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	var locked bool
	if err := db.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended('trpc-agent-queuebench', 0))`).Scan(&locked); err != nil {
		return fmt.Errorf("acquire benchmark lock: %w", err)
	}
	if !locked {
		return errors.New("another queue benchmark is using this database")
	}
	defer db.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtextextended('trpc-agent-queuebench', 0))`)
	var existing int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM tenants WHERE id LIKE 'queuebench-%'`).Scan(&existing); err != nil {
		return fmt.Errorf("check benchmark tenant namespace: %w", err)
	}
	if existing != 0 {
		return fmt.Errorf("benchmark namespace is not empty: %d tenant rows", existing)
	}
	ids := []string{"queuebench-" + uuid.NewString(), "queuebench-" + uuid.NewString()}
	createTenants := func() error {
		for _, id := range ids {
			if _, err := db.ExecContext(context.Background(), `INSERT INTO tenants(id,name,status,config) VALUES($1,$2,'active','{}')`, id, id); err != nil {
				return fmt.Errorf("create benchmark tenant: %w", err)
			}
		}
		return nil
	}
	if err := createTenants(); err != nil {
		return err
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM tenants WHERE id = ANY($1)`, pqArray(ids))
	}()
	store := reliable.NewPostgresStore(db)
	cfg := queuebench.DefaultConfig()
	cfg.MessagesPerTenant, cfg.WorkDuration, cfg.Timeout = messages, work, timeout
	consumers := append([]int(nil), cfg.Consumers...)
	sort.Ints(consumers)
	var results []queuebench.Result
	for _, scenario := range []string{"uniform", "weighted", "weighted_hotspot"} {
		for _, mode := range []string{"ordinary", "fair"} {
			for _, count := range consumers {
				caseCtx, caseCancel := context.WithTimeout(context.Background(), timeout)
				result, execErr := queuebench.Execute(caseCtx, store, cfg, ids, scenario, mode, count)
				caseCancel()
				if execErr != nil {
					return fmt.Errorf("scenario=%s mode=%s consumers=%d: %w", scenario, mode, count, execErr)
				}
				var persistedCompleted, persistedInflight, replies int
				if err := db.QueryRowContext(context.Background(), `SELECT count(*) FILTER (WHERE status='COMPLETED'), count(*) FILTER (WHERE status IN ('RECEIVED','PROCESSING','RETRY_WAIT','WAITING_APPROVAL')) FROM inbox_messages WHERE tenant_id = ANY($1)`, pqArray(ids)).Scan(&persistedCompleted, &persistedInflight); err != nil {
					return fmt.Errorf("verify persisted queue state: %w", err)
				}
				if err := db.QueryRowContext(context.Background(), `SELECT count(*) FROM outbox_messages WHERE tenant_id = ANY($1)`, pqArray(ids)).Scan(&replies); err != nil {
					return fmt.Errorf("verify outbox state: %w", err)
				}
				result.PersistedCompleted, result.PersistedReplies, result.PersistedInflight = persistedCompleted, replies, persistedInflight
				result.Verified = result.Completed == result.Messages && persistedCompleted == result.Messages && replies == result.Messages && persistedInflight == 0
				if !result.Verified {
					return fmt.Errorf("scenario=%s mode=%s consumers=%d failed persisted verification: %#v", scenario, mode, count, result)
				}
				results = append(results, result)
				if _, err := db.ExecContext(context.Background(), `DELETE FROM tenants WHERE id = ANY($1)`, pqArray(ids)); err != nil {
					return fmt.Errorf("reset benchmark backlog: %w", err)
				}
				if err := createTenants(); err != nil {
					return err
				}
			}
		}
	}
	encoded, err := json.MarshalIndent(map[string]any{"database": dbName, "tenants": ids, "generated_at": time.Now().UTC(), "results": results}, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if output == "" {
		_, err = os.Stdout.Write(encoded)
		return err
	}
	return os.WriteFile(output, encoded, 0o600)
}

func databaseName(dsn string) (string, error) {
	for _, field := range strings.Fields(dsn) {
		if strings.HasPrefix(field, "dbname=") {
			return strings.Trim(field[len("dbname="):], `"'`), nil
		}
	}
	parts := strings.Split(strings.TrimRight(strings.SplitN(dsn, "?", 2)[0], "/"), "/")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return "", errors.New("database URL must include a database name")
	}
	return parts[len(parts)-1], nil
}

type arrayParam []string

func pqArray(values []string) driver.Valuer { return arrayParam(values) }

func (a arrayParam) Value() (driver.Value, error) {
	parts := make([]string, len(a))
	for i, value := range a {
		parts[i] = `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}
