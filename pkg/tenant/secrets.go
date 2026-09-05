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
	"fmt"
	"os"
	"strings"
)

// SecretRef identifies an operator-owned secret without embedding the secret
// value in tenant configuration. References are deliberately scheme-qualified
// so an inline token cannot be mistaken for a resolver handle.
type SecretRef string

const envSecretScheme = "env://"

// Validate checks the non-secret shape of a reference. Resolver-specific
// policy, such as the environment prefix allowlist, is enforced by Resolve.
func (r SecretRef) Validate() error {
	value := string(r)
	if len(value) == 0 || len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
		return ErrInvalidSecretRef
	}
	if !strings.Contains(value, "://") {
		return ErrInvalidSecretRef
	}
	return nil
}

var (
	// ErrInvalidSecretRef indicates that a reference is malformed or uses an
	// unsupported scheme. The error never includes the supplied reference.
	ErrInvalidSecretRef = errors.New("invalid secret reference")
	// ErrSecretUnavailable indicates that a valid reference has no injected
	// value. The resolver does not expose the missing environment variable.
	ErrSecretUnavailable = errors.New("secret is unavailable")
)

// SecretResolver resolves operator-owned references at the narrow runtime
// boundary that needs a credential. Implementations must not log or return
// secret values in errors. Callers own the returned bytes and should release
// them as soon as the downstream SDK has copied the credential.
type SecretResolver interface {
	Resolve(context.Context, SecretRef) ([]byte, error)
}

// EnvSecretResolver resolves env://NAME references from secrets injected into
// the process environment by the operator (for example a Kubernetes Secret or
// a Docker secret wrapper). Prefix is an allowlist: an empty prefix is rejected
// to prevent arbitrary process-environment reads.
type EnvSecretResolver struct {
	prefix    string
	lookupEnv func(string) (string, bool)
}

// NewEnvSecretResolver constructs an environment resolver restricted to prefix.
// Prefix must be a valid environment-name prefix such as "TRPC_SECRET_".
func NewEnvSecretResolver(prefix string) (*EnvSecretResolver, error) {
	if !validEnvPrefix(prefix) {
		return nil, fmt.Errorf("%w: environment prefix is invalid", ErrInvalidSecretRef)
	}
	return &EnvSecretResolver{prefix: prefix, lookupEnv: os.LookupEnv}, nil
}

// Resolve implements SecretResolver. Only env:// references whose name starts
// with the configured operator prefix are accepted.
func (r *EnvSecretResolver) Resolve(ctx context.Context, ref SecretRef) ([]byte, error) {
	if r == nil || r.lookupEnv == nil || !validEnvPrefix(r.prefix) {
		return nil, ErrSecretUnavailable
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	value := string(ref)
	if !strings.HasPrefix(value, envSecretScheme) {
		return nil, ErrInvalidSecretRef
	}
	name := strings.TrimPrefix(value, envSecretScheme)
	if !validEnvName(name) || !strings.HasPrefix(name, r.prefix) {
		return nil, ErrInvalidSecretRef
	}
	secret, ok := r.lookupEnv(name)
	if !ok || secret == "" {
		return nil, ErrSecretUnavailable
	}
	return []byte(secret), nil
}

func validEnvPrefix(prefix string) bool {
	if prefix == "" || !validEnvName(strings.TrimSuffix(prefix, "_")) {
		return false
	}
	return strings.HasSuffix(prefix, "_")
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, char := range name {
		if (char >= 'A' && char <= 'Z') || char == '_' || (i > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return name[0] != '_'
}
