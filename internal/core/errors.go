package core

import "errors"

var (
	ErrNotFound         = errors.New("not found")
	ErrPermissionDenied = errors.New("permission denied")
	ErrPluginCrashed    = errors.New("plugin crashed")
	ErrProviderTimeout  = errors.New("provider timeout")
	ErrToolNotFound     = errors.New("tool not found")

	// Open Core sentinel errors
	ErrFeatureGated    = errors.New("feature requires Pro subscription")
	ErrLicenseExpired  = errors.New("Pro license expired")
	ErrLicenseOffline  = errors.New("license validation offline beyond grace period")
	ErrMachineLimitHit = errors.New("Pro active on maximum machines")
	ErrHookFailed      = errors.New("agent hook failed")
	ErrHookTimeout     = errors.New("agent hook timed out")
)
