// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewKVStore(t *testing.T) {
	store := NewKVStore("/tmp/test")
	assert.NotNil(t, store)
	assert.Equal(t, "/tmp/test", store.baseDir)
}

func TestKVStore_InitCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "memory")

	store := NewKVStore(subDir)
	err := store.Init(context.Background())
	require.NoError(t, err)

	info, err := os.Stat(subDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestKVStore_HealthBeforeInit(t *testing.T) {
	store := NewKVStore(t.TempDir())
	err := store.Health()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestKVStore_HealthAfterInit(t *testing.T) {
	store := NewKVStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	assert.NoError(t, store.Health())
}

func TestKVStore_RememberAndRecall(t *testing.T) {
	store := NewKVStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	ctx := context.Background()

	err := store.Remember(ctx, "greeting", []byte("hello"))
	require.NoError(t, err)

	val, err := store.Recall(ctx, "greeting")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), val)
}

func TestKVStore_RecallNonexistent(t *testing.T) {
	store := NewKVStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	_, err := store.Recall(context.Background(), "missing")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestKVStore_Forget(t *testing.T) {
	store := NewKVStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	ctx := context.Background()

	require.NoError(t, store.Remember(ctx, "temp", []byte("data")))
	require.NoError(t, store.Forget(ctx, "temp"))

	_, err := store.Recall(ctx, "temp")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestKVStore_ForgetNonexistent(t *testing.T) {
	store := NewKVStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	err := store.Forget(context.Background(), "never-existed")
	assert.NoError(t, err)
}

func TestKVStore_Search(t *testing.T) {
	store := NewKVStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	ctx := context.Background()

	require.NoError(t, store.Remember(ctx, "user:name", []byte("alice")))
	require.NoError(t, store.Remember(ctx, "user:email", []byte("alice@example.com")))
	require.NoError(t, store.Remember(ctx, "project:name", []byte("siply")))

	results, err := store.Search(ctx, "user:")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestKVStore_SearchEmptyQuery(t *testing.T) {
	store := NewKVStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	ctx := context.Background()

	require.NoError(t, store.Remember(ctx, "a", []byte("1")))
	require.NoError(t, store.Remember(ctx, "b", []byte("2")))

	results, err := store.Search(ctx, "")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestKVStore_SearchNoResults(t *testing.T) {
	store := NewKVStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	results, err := store.Search(context.Background(), "nothing")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestKVStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Write data with first store instance.
	store1 := NewKVStore(dir)
	require.NoError(t, store1.Init(ctx))
	require.NoError(t, store1.Remember(ctx, "persistent", []byte("value")))
	require.NoError(t, store1.Stop(ctx))

	// Read data with new store instance.
	store2 := NewKVStore(dir)
	require.NoError(t, store2.Init(ctx))
	val, err := store2.Recall(ctx, "persistent")
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), val)
}

func TestKVStore_ValidateKey_Empty(t *testing.T) {
	store := NewKVStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	err := store.Remember(context.Background(), "", []byte("val"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty key")
}

func TestKVStore_ValidateKey_PathTraversal(t *testing.T) {
	store := NewKVStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))

	err := store.Remember(context.Background(), "../escape", []byte("val"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path traversal")
}

func TestKVStore_OperationsBeforeInit(t *testing.T) {
	store := NewKVStore(t.TempDir())
	ctx := context.Background()

	err := store.Remember(ctx, "key", []byte("val"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")

	_, err = store.Recall(ctx, "key")
	assert.Error(t, err)

	err = store.Forget(ctx, "key")
	assert.Error(t, err)

	_, err = store.Search(ctx, "query")
	assert.Error(t, err)
}

func TestKVStore_ValueCopyOnWrite(t *testing.T) {
	store := NewKVStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	ctx := context.Background()

	original := []byte("original")
	require.NoError(t, store.Remember(ctx, "key", original))

	// Mutate the original — should not affect stored value.
	original[0] = 'X'

	val, err := store.Recall(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("original"), val)
}

func TestKVStore_ValueCopyOnRead(t *testing.T) {
	store := NewKVStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	ctx := context.Background()

	require.NoError(t, store.Remember(ctx, "key", []byte("value")))

	val1, err := store.Recall(ctx, "key")
	require.NoError(t, err)

	// Mutate the returned value — should not affect stored value.
	val1[0] = 'X'

	val2, err := store.Recall(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), val2)
}

func TestKVStore_Overwrite(t *testing.T) {
	store := NewKVStore(t.TempDir())
	require.NoError(t, store.Init(context.Background()))
	ctx := context.Background()

	require.NoError(t, store.Remember(ctx, "key", []byte("v1")))
	require.NoError(t, store.Remember(ctx, "key", []byte("v2")))

	val, err := store.Recall(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("v2"), val)
}

func TestKVStore_CorruptFileRecovery(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Write corrupt data to the store file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, storeFileName), []byte("not json"), 0644))

	// Init should handle corrupt file gracefully.
	store := NewKVStore(dir)
	err := store.Init(ctx)
	require.NoError(t, err)

	// Should be empty (started fresh).
	results, err := store.Search(ctx, "")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestKVStore_WorkspaceScoping(t *testing.T) {
	ctx := context.Background()

	dir1 := filepath.Join(t.TempDir(), "workspace1", "memory")
	dir2 := filepath.Join(t.TempDir(), "workspace2", "memory")

	store1 := NewKVStore(dir1)
	store2 := NewKVStore(dir2)
	require.NoError(t, store1.Init(ctx))
	require.NoError(t, store2.Init(ctx))

	require.NoError(t, store1.Remember(ctx, "key", []byte("ws1")))
	require.NoError(t, store2.Remember(ctx, "key", []byte("ws2")))

	val1, err := store1.Recall(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("ws1"), val1)

	val2, err := store2.Recall(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, []byte("ws2"), val2)
}
