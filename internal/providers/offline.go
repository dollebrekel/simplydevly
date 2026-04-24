// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package providers

import (
	"os"
	"strings"

	"siply.dev/siply/internal/core"
)

const DefaultOfflineModel = "qwen2.5-coder:7b"

// ResolveOfflineModel returns the model to use in offline mode.
// Priority: explicit override > SIPLY_MODEL env var > config offline_model > default.
func ResolveOfflineModel(override string, cfg core.ProviderConfig) string {
	if override != "" {
		return override
	}
	if envModel := os.Getenv("SIPLY_MODEL"); envModel != "" {
		return envModel
	}
	if cfg.OfflineModel != "" {
		return cfg.OfflineModel
	}
	return DefaultOfflineModel
}

// IsOfflineEnv returns true if SIPLY_OFFLINE is set to a truthy value.
func IsOfflineEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("SIPLY_OFFLINE")))
	return v == "1" || v == "true" || v == "yes"
}
