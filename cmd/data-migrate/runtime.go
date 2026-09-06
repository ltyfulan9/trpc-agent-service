package main

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"time"

	_ "github.com/lib/pq"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/migrationruntime"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/runtimeplane"
	"trpc.group/trpc-go/trpc-agent-go/enterprise/pkg/storage"
)

func runCommand(ctx context.Context, options commandOptions, output io.Writer) error {
	dsn := os.Getenv("DATABASE_URL")
	if err := storage.ValidateServicePostgresURL(dsn, os.Getenv("DATABASE_ALLOW_INSECURE") == "true"); err != nil {
		return errors.New("invalid DATABASE_URL")
	}
	profiles, err := storage.LoadBackendProfiles(os.Getenv("STORAGE_BACKEND_PROFILES"), os.LookupEnv)
	if err != nil {
		return errors.New("invalid storage profiles")
	}
	manifest := os.Getenv("DATA_PLANE_PROFILES")
	if manifest == "" {
		manifest = "[]"
	}
	dataProfiles, err := runtimeplane.LoadProfiles(manifest, os.LookupEnv)
	if err != nil {
		return errors.New("invalid data-plane profiles")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return errors.New("cannot open control database")
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	pingCtx, cancelPing := context.WithTimeout(ctx, 10*time.Second)
	err = db.PingContext(pingCtx)
	cancelPing()
	if err != nil {
		return errors.New("control database unavailable")
	}
	runtime, err := migrationruntime.New(migrationruntime.Options{
		DB: db, StorageProfiles: profiles, DataPlaneProfiles: dataProfiles,
		Owner: options.owner, LeaseTTL: options.lease, BatchSize: options.batchSize,
	})
	if err != nil {
		return err
	}
	defer runtime.Close()
	return execute(ctx, runtime.Coordinator, options, output)
}
