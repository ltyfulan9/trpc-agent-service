//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.

package tenant

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEnvSecretResolverResolvesOnlyPrefixedReferences(t *testing.T) {
	resolver, err := NewEnvSecretResolver("TRPC_SECRET_")
	if err != nil {
		t.Fatal(err)
	}
	lookup := map[string]string{"TRPC_SECRET_OPENAI": "operator-value"}
	resolver.lookupEnv = func(name string) (string, bool) {
		value, ok := lookup[name]
		return value, ok
	}
	got, err := resolver.Resolve(context.Background(), "env://TRPC_SECRET_OPENAI")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "operator-value" {
		t.Fatalf("resolved secret = %q", got)
	}
	got[0] = 'x'
	if lookup["TRPC_SECRET_OPENAI"] != "operator-value" {
		t.Fatal("resolver returned mutable backing storage")
	}
}

func TestEnvSecretResolverRejectsMalformedOrUnscopedReferences(t *testing.T) {
	resolver, err := NewEnvSecretResolver("TRPC_SECRET_")
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []SecretRef{"plain-token", "file:///tmp/key", "env://OTHER_KEY", "env://TRPC_SECRET_bad", "env://TRPC_SECRET_A\nB"} {
		_, err := resolver.Resolve(context.Background(), ref)
		if !errors.Is(err, ErrInvalidSecretRef) {
			t.Errorf("ref %q error = %v, want ErrInvalidSecretRef", ref, err)
		}
	}
}

func TestEnvSecretResolverDoesNotLeakNamesOrValues(t *testing.T) {
	resolver, err := NewEnvSecretResolver("TRPC_SECRET_")
	if err != nil {
		t.Fatal(err)
	}
	resolver.lookupEnv = func(string) (string, bool) { return "", false }
	_, err = resolver.Resolve(context.Background(), "env://TRPC_SECRET_MISSING")
	if !errors.Is(err, ErrSecretUnavailable) {
		t.Fatalf("missing secret error = %v", err)
	}
	if strings.Contains(err.Error(), "TRPC_SECRET_MISSING") || strings.Contains(err.Error(), "operator-value") {
		t.Fatalf("secret details leaked in error: %v", err)
	}
}

func TestEnvSecretResolverHonorsCancellation(t *testing.T) {
	resolver, err := NewEnvSecretResolver("TRPC_SECRET_")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = resolver.Resolve(ctx, "env://TRPC_SECRET_OPENAI")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled resolve error = %v", err)
	}
}

func TestNewEnvSecretResolverRequiresScopedPrefix(t *testing.T) {
	for _, prefix := range []string{"", "TRPC", "trpc_", "TRPC-"} {
		if _, err := NewEnvSecretResolver(prefix); !errors.Is(err, ErrInvalidSecretRef) {
			t.Errorf("prefix %q error = %v, want ErrInvalidSecretRef", prefix, err)
		}
	}
}

func TestValidateConfigAcceptsAPIKeyRefAndRejectsBothForms(t *testing.T) {
	config := validConfig()
	config.Models[0].APIKey = ""
	config.Models[0].APIKeyRef = "env://TRPC_SECRET_OPENAI"
	if err := ValidateConfig(config); err != nil {
		t.Fatalf("API key reference rejected: %v", err)
	}
	config.Models[0].APIKey = "legacy-encrypted-value"
	if err := ValidateConfig(config); err == nil {
		t.Fatal("configuration containing both API key forms was accepted")
	}
}
