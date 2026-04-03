package core

import "time"

// Credential holds a stored credential value with optional expiration.
// A nil ExpiresAt means the credential never expires.
type Credential struct {
	Value     string
	ExpiresAt *time.Time
}

// CredentialStore manages provider and plugin credentials.
type CredentialStore interface {
	Lifecycle
	GetProvider(provider string) (Credential, error)
	SetProvider(provider string, cred Credential) error
	GetPluginCredential(pluginName string, key string) (Credential, error)
	SetPluginCredential(pluginName string, key string, cred Credential) error
}
