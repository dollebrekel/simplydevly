package core

import (
	"context"
	"time"
)

// Credential holds a stored credential value with optional expiration.
// A nil ExpiresAt means the credential never expires.
type Credential struct {
	Value     string
	ExpiresAt *time.Time
}

// CredentialStore manages provider and plugin credentials.
type CredentialStore interface {
	Lifecycle
	GetProvider(ctx context.Context, provider string) (Credential, error)
	SetProvider(ctx context.Context, provider string, cred Credential) error
	GetPluginCredential(ctx context.Context, pluginName string, key string) (Credential, error)
	SetPluginCredential(ctx context.Context, pluginName string, key string, cred Credential) error
}
