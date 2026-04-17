// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadIndex(t *testing.T) {
	fixtureFile := filepath.Join("testdata", "marketplace-index.json")

	tests := []struct {
		name      string
		path      string
		wantErr   bool
		errIs     error
		wantItems int
		wantVer   int
	}{
		{
			name:      "loads fixture successfully",
			path:      fixtureFile,
			wantItems: 12,
			wantVer:   1,
		},
		{
			name:    "missing file returns ErrIndexNotFound",
			path:    filepath.Join("testdata", "nonexistent.json"),
			wantErr: true,
			errIs:   ErrIndexNotFound,
		},
		{
			name:    "empty path returns ErrIndexNotFound",
			path:    "",
			wantErr: true,
			errIs:   ErrIndexNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx, err := LoadIndex(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.True(t, errors.Is(err, tt.errIs), "expected errors.Is(%v, %v)", err, tt.errIs)
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, idx)
			assert.Equal(t, tt.wantVer, idx.Version)
			assert.Len(t, idx.Items, tt.wantItems)
		})
	}
}

func TestLoadIndex_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte(`{not valid json`), 0600))

	_, err := LoadIndex(bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse index")
}

func TestBundleComponent_JSONRoundtrip(t *testing.T) {
	idx, err := LoadIndex(filepath.Join("testdata", "marketplace-index.json"))
	require.NoError(t, err)

	var bundle *Item
	for i := range idx.Items {
		if idx.Items[i].Name == "fullstack-starter" {
			bundle = &idx.Items[i]
			break
		}
	}
	require.NotNil(t, bundle, "fullstack-starter bundle must be present in fixture")
	assert.Equal(t, "bundles", bundle.Category)
	require.Len(t, bundle.Components, 3)
	assert.Equal(t, "memory-default", bundle.Components[0].Name)
	assert.Equal(t, "1.2.0", bundle.Components[0].Version)
	assert.Equal(t, "code-review-skill", bundle.Components[1].Name)
	assert.Equal(t, "golang-defaults", bundle.Components[2].Name)
}

func TestLoadIndex_ItemFields(t *testing.T) {
	idx, err := LoadIndex(filepath.Join("testdata", "marketplace-index.json"))
	require.NoError(t, err)

	// Find memory-default plugin and verify all fields are populated.
	var memDefault *Item
	for i := range idx.Items {
		if idx.Items[i].Name == "memory-default" {
			memDefault = &idx.Items[i]
			break
		}
	}
	require.NotNil(t, memDefault, "memory-default item must be present in fixture")

	assert.Equal(t, "plugins", memDefault.Category)
	assert.Equal(t, "simplydevly", memDefault.Author)
	assert.Equal(t, 4.8, memDefault.Rating)
	assert.Equal(t, int64(12500), memDefault.InstallCount)
	assert.True(t, memDefault.Verified)
	assert.NotEmpty(t, memDefault.Tags)
	assert.Equal(t, "Apache-2.0", memDefault.License)
}
