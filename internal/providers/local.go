// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package providers

import (
	"os"
	"strings"

	"siply.dev/siply/internal/core"
)

const DefaultLocalModel = "qwen2.5-coder:7b"

// ResolveLocalModel returns the model to use in local mode.
// Priority: explicit override > SIPLY_MODEL env var > config local_model > default.
func ResolveLocalModel(override string, cfg core.ProviderConfig) string {
	if o := strings.TrimSpace(override); o != "" {
		return o
	}
	if envModel := strings.TrimSpace(os.Getenv("SIPLY_MODEL")); envModel != "" {
		return envModel
	}
	if cfg.LocalModel != "" {
		return cfg.LocalModel
	}
	return DefaultLocalModel
}

// IsLocalEnv returns true if SIPLY_LOCAL is set to a truthy value.
func IsLocalEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("SIPLY_LOCAL")))
	return v == "1" || v == "true" || v == "yes"
}
