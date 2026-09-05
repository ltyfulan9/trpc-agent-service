package tenant

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type channelSecretResolver struct {
	values map[SecretRef][]byte
}

func (r channelSecretResolver) Resolve(_ context.Context, ref SecretRef) ([]byte, error) {
	value, ok := r.values[ref]
	if !ok {
		return nil, ErrSecretUnavailable
	}
	return append([]byte(nil), value...), nil
}

func TestValidateConfigAcceptsChannelSecretReferences(t *testing.T) {
	config := validConfig()
	binding := &config.Channels[0]
	binding.Token = ""
	binding.Secret = ""
	binding.TokenRef = "env://TRPC_SECRET_TELEGRAM_TOKEN"
	binding.SecretRef = "env://TRPC_SECRET_TELEGRAM_WEBHOOK"
	if err := ValidateConfig(config); err != nil {
		t.Fatalf("channel secret references rejected: %v", err)
	}

	binding.Token = "inline-token"
	if err := ValidateConfig(config); err == nil {
		t.Fatal("inline value and secret reference were accepted together")
	}
}

func TestValidateConfigAcceptsWeWorkAESKeyReference(t *testing.T) {
	config := validConfig()
	config.Channels[0] = ChannelBinding{
		Type: "wework", AgentApp: "support",
		TokenRef:          "env://TRPC_SECRET_WECOM_TOKEN",
		SecretRef:         "env://TRPC_SECRET_WECOM_CORP_SECRET",
		EncodingAESKeyRef: "env://TRPC_SECRET_WECOM_AES",
		AppID:             "1000002",
		Config:            map[string]string{"corp_id": "ww" + strings.Repeat("a", 16)},
		AccessPolicy:      ChannelAccessPolicy{AllowDirectMessages: true, AllowedUsers: []string{"user-1"}},
	}
	if err := ValidateConfig(config); err != nil {
		t.Fatalf("WeCom secret references rejected: %v", err)
	}
}

func TestResolveChannelSecretRefsMaterializesOnlySelectedCredentials(t *testing.T) {
	service := &TenantService{secretResolver: channelSecretResolver{values: map[SecretRef][]byte{
		"env://TRPC_SECRET_TOKEN":   []byte("123456789:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"),
		"env://TRPC_SECRET_WEBHOOK": []byte(strings.Repeat("s", 32)),
	}}}
	binding := &ChannelBinding{
		Type:      "telegram",
		TokenRef:  "env://TRPC_SECRET_TOKEN",
		SecretRef: "env://TRPC_SECRET_WEBHOOK",
	}
	if err := service.resolveChannelSecretRefs(context.Background(), binding); err != nil {
		t.Fatalf("resolve channel references: %v", err)
	}
	if binding.Token == "" || binding.Secret == "" || binding.TokenRef != "" || binding.SecretRef != "" {
		t.Fatalf("resolved binding = %+v, want values with refs cleared", binding)
	}
}

func TestResolveChannelSecretRefsFailsClosedWithoutResolver(t *testing.T) {
	service := &TenantService{}
	binding := &ChannelBinding{TokenRef: "env://TRPC_SECRET_TOKEN"}
	err := service.resolveChannelSecretRefs(context.Background(), binding)
	if !errors.Is(err, ErrSecretUnavailable) {
		t.Fatalf("error = %v, want ErrSecretUnavailable", err)
	}
}

func TestResolveChannelSecretRefsRedactsResolverErrors(t *testing.T) {
	service := &TenantService{secretResolver: failingSecretResolver{}}
	binding := &ChannelBinding{TokenRef: "env://TRPC_SECRET_TOKEN"}
	err := service.resolveChannelSecretRefs(context.Background(), binding)
	if !errors.Is(err, ErrSecretUnavailable) || strings.Contains(err.Error(), "provider-secret") {
		t.Fatalf("error = %v, want stable redacted unavailable error", err)
	}
}

type failingSecretResolver struct{}

func (failingSecretResolver) Resolve(context.Context, SecretRef) ([]byte, error) {
	return nil, errors.New("provider-secret=do-not-leak")
}
