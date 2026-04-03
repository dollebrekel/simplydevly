package core

import "context"

// Storage defines the contract for persistent key-value storage.
type Storage interface {
	Lifecycle
	Get(ctx context.Context, path string) ([]byte, error)
	Put(ctx context.Context, path string, data []byte) error
	List(ctx context.Context, prefix string) ([]string, error)
	Delete(ctx context.Context, path string) error
}
