// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

import "context"

// Lifecycle defines the standard lifecycle contract for stateful subsystems.
type Lifecycle interface {
	Init(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health() error
}
