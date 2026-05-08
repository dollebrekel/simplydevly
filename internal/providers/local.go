// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package providers

import (
	"os"
	"strings"

	"siply.dev/siply/internal/core"
)

// ResolveLocalModel returns the model to use in local mode.
// Priority: explicit override > SIPLY_MODEL env var > config local_model.
// Returns empty string if no model is configured.
func ResolveLocalModel(override string, cfg core.ProviderConfig) string {
	if o := strings.TrimSpace(override); o != "" {
		return o
	}
	if envModel := strings.TrimSpace(os.Getenv("SIPLY_MODEL")); envModel != "" {
		return envModel
	}
	return strings.TrimSpace(cfg.LocalModel)
}

// IsLocalEnv returns true if SIPLY_LOCAL is set to a truthy value.
func IsLocalEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("SIPLY_LOCAL")))
	return v == "1" || v == "true" || v == "yes"
}
