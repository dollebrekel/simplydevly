package core

import "errors"

var (
	ErrNotFound         = errors.New("not found")
	ErrPermissionDenied = errors.New("permission denied")
	ErrPluginCrashed    = errors.New("plugin crashed")
	ErrProviderTimeout  = errors.New("provider timeout")
	ErrToolNotFound     = errors.New("tool not found")
)
