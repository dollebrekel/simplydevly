package core

import "context"

// Lifecycle defines the standard lifecycle contract for stateful subsystems.
type Lifecycle interface {
	Init(ctx context.Context) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health() error
}
