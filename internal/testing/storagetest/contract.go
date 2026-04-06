// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// Package storagetest provides reusable contract tests for any core.Storage implementation.
// Import this package from your implementation's _test.go to verify interface compliance.
package storagetest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
)

// RunContractTests runs the reusable contract test suite against any Storage implementation.
// Factory must return a new, initialized Storage instance for each call.
func RunContractTests(t *testing.T, factory func() core.Storage) {
	t.Helper()
	ctx := context.Background()

	// AC#6: contract tests for Storage interface
	t.Run("PutThenGet", func(t *testing.T) {
		s := factory()
		data := []byte("hello world")
		err := s.Put(ctx, "test/key", data)
		require.NoError(t, err)

		got, err := s.Get(ctx, "test/key")
		require.NoError(t, err)
		assert.Equal(t, data, got)
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		s := factory()
		_, err := s.Get(ctx, "no/such/key")
		require.Error(t, err)
	})

	t.Run("DeleteThenGet", func(t *testing.T) {
		s := factory()
		err := s.Put(ctx, "del/key", []byte("data"))
		require.NoError(t, err)

		err = s.Delete(ctx, "del/key")
		require.NoError(t, err)

		_, err = s.Get(ctx, "del/key")
		require.Error(t, err)
	})

	t.Run("ListPrefix", func(t *testing.T) {
		s := factory()
		require.NoError(t, s.Put(ctx, "list/a", []byte("1")))
		require.NoError(t, s.Put(ctx, "list/b", []byte("2")))
		require.NoError(t, s.Put(ctx, "list/c", []byte("3")))
		require.NoError(t, s.Put(ctx, "other/x", []byte("4")))

		got, err := s.List(ctx, "list")
		require.NoError(t, err)
		assert.Len(t, got, 3)
		assert.Contains(t, got, "list/a")
		assert.Contains(t, got, "list/b")
		assert.Contains(t, got, "list/c")
		assert.NotContains(t, got, "other/x")
	})

	t.Run("ListEmpty", func(t *testing.T) {
		s := factory()
		got, err := s.List(ctx, "empty-prefix")
		require.NoError(t, err)
		require.NotNil(t, got, "List must return empty slice, not nil")
		assert.Empty(t, got)
	})

	t.Run("PutOverwrite", func(t *testing.T) {
		s := factory()
		require.NoError(t, s.Put(ctx, "overwrite/key", []byte("first")))
		require.NoError(t, s.Put(ctx, "overwrite/key", []byte("second")))

		got, err := s.Get(ctx, "overwrite/key")
		require.NoError(t, err)
		assert.Equal(t, []byte("second"), got)
	})

	t.Run("DeleteNonExistent", func(t *testing.T) {
		s := factory()
		err := s.Delete(ctx, "never/existed")
		assert.NoError(t, err, "Delete of non-existent key must be idempotent")
	})
}
