// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

import "context"

// MemoryBackend defines the contract for memory storage subsystems.
type MemoryBackend interface {
	Lifecycle
	Remember(ctx context.Context, key string, value []byte) error
	Recall(ctx context.Context, key string) ([]byte, error)
	Forget(ctx context.Context, key string) error
	Search(ctx context.Context, query string) ([][]byte, error)
}
