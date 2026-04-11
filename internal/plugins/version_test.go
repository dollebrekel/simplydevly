// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name    string
		a, b    string
		want    int
		wantErr bool
	}{
		{"equal", "1.0.0", "1.0.0", 0, false},
		{"equal with v prefix", "v1.0.0", "1.0.0", 0, false},
		{"a greater", "2.0.0", "1.0.0", 1, false},
		{"a less", "1.0.0", "2.0.0", -1, false},
		{"patch greater", "1.0.1", "1.0.0", 1, false},
		{"minor greater", "1.1.0", "1.0.0", 1, false},
		{"pre-release less than release", "v1.0.0-alpha", "v1.0.0", -1, false},
		{"pre-release comparison", "v1.0.0-alpha", "v1.0.0-beta", -1, false},
		{"both with v prefix", "v1.2.3", "v1.2.3", 0, false},
		{"mixed v prefix", "v1.0.0", "1.0.0", 0, false},
		{"build metadata ignored", "1.0.0+build1", "1.0.0+build2", 0, false},
		{"empty a less than valid", "", "1.0.0", -1, false},
		{"empty b greater than valid", "1.0.0", "", 1, false},
		{"both empty equal", "", "", 0, false},
		{"invalid a", "not-semver", "1.0.0", 0, true},
		{"invalid b", "1.0.0", "garbage", 0, true},
		{"both invalid", "abc", "xyz", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareVersions(tt.a, tt.b)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestIsCompatible(t *testing.T) {
	tests := []struct {
		name           string
		siplyMin       string
		currentVersion string
		want           bool
	}{
		{"compatible — equal", "1.0.0", "1.0.0", true},
		{"compatible — current greater", "1.0.0", "2.0.0", true},
		{"incompatible — current less", "2.0.0", "1.0.0", false},
		{"dev version always compatible", "5.0.0", "dev", true},
		{"empty current always compatible", "1.0.0", "", true},
		{"empty siplyMin always compatible", "", "1.0.0", true},
		{"both empty", "", "", true},
		{"patch level compatible", "1.0.0", "1.0.1", true},
		{"minor level compatible", "1.0.0", "1.1.0", true},
		{"pre-release incompatible", "1.0.0", "1.0.0-alpha", false},
		{"invalid siplyMin", "not-semver", "1.0.0", false},
		{"invalid currentVersion", "1.0.0", "garbage", false},
		{"both invalid", "abc", "xyz", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCompatible(tt.siplyMin, tt.currentVersion)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatIncompatibleMessage(t *testing.T) {
	msg := FormatIncompatibleMessage("my-plugin", "1.2.0", "0.9.0", "1.0.0")
	assert.Contains(t, msg, "my-plugin")
	assert.Contains(t, msg, "v1.2.0")
	assert.Contains(t, msg, "v0.9.0")
	assert.Contains(t, msg, ">=1.0.0")
}

func TestGetSiplyVersion(t *testing.T) {
	v := GetSiplyVersion()
	// In test context, build info returns "(devel)", so we expect "dev".
	assert.Equal(t, "dev", v)
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.0.0", "v1.0.0"},
		{"v1.0.0", "v1.0.0"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeVersion(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
