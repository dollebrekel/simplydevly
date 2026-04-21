// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	lua "github.com/yuin/gopher-lua"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateHTTPURL_AllowedExact(t *testing.T) {
	t.Parallel()
	err := validateHTTPURL("https://api.example.com/data", []string{"api.example.com"})
	assert.NoError(t, err)
}

func TestValidateHTTPURL_AllowedWildcard(t *testing.T) {
	t.Parallel()
	err := validateHTTPURL("https://sub.github.com/path", []string{"*.github.com"})
	assert.NoError(t, err)
}

func TestValidateHTTPURL_WildcardDoesNotMatchBase(t *testing.T) {
	t.Parallel()
	err := validateHTTPURL("https://github.com/path", []string{"*.github.com"})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrLuaSandboxViolation))
}

func TestValidateHTTPURL_Blocked(t *testing.T) {
	t.Parallel()
	err := validateHTTPURL("https://evil.com/data", []string{"api.example.com"})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrLuaSandboxViolation))
}

func TestValidateHTTPURL_EmptyAllowlist(t *testing.T) {
	t.Parallel()
	err := validateHTTPURL("https://anything.com", nil)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrLuaSandboxViolation))
}

func TestValidateHTTPURL_InvalidURL(t *testing.T) {
	t.Parallel()
	err := validateHTTPURL("not a url", []string{"example.com"})
	assert.Error(t, err)
}

func TestMatchHost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		host, pattern string
		want          bool
	}{
		{"example.com", "example.com", true},
		{"sub.example.com", "*.example.com", true},
		{"deep.sub.example.com", "*.example.com", true},
		{"example.com", "*.example.com", false},
		{"other.com", "example.com", false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.host, tt.pattern), func(t *testing.T) {
			assert.Equal(t, tt.want, matchHost(tt.host, tt.pattern))
		})
	}
}

func TestSiplyHTTPGet_AllowedURL(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, `{"ok": true}`)
	}))
	defer server.Close()

	L := NewSandboxedState(context.Background())
	defer L.Close()

	// Allowlist the hostname (127.0.0.1) — validateHTTPURL uses url.Hostname() which strips port.
	plugin := &Tier2Plugin{Name: "http-test", HTTPAllowlist: []string{"127.0.0.1"}}
	registerSiplyAPI(L, plugin, nil, nil)

	err := L.DoString(fmt.Sprintf(`
		result = siply.http.get("%s/data")
	`, server.URL))
	require.NoError(t, err)

	val := L.GetGlobal("result")
	assert.NotNil(t, val)
	assert.IsType(t, &lua.LTable{}, val)
}

func TestSiplyHTTPGet_BlockedURL(t *testing.T) {
	t.Parallel()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "http-blocked", HTTPAllowlist: []string{"allowed.com"}}
	registerSiplyAPI(L, plugin, nil, nil)

	err := L.DoString(`
		result, err_msg = siply.http.get("https://evil.com/steal")
	`)
	require.NoError(t, err)

	errMsg := L.GetGlobal("err_msg")
	assert.Contains(t, errMsg.String(), "not on allowlist")
}

func TestSiplyHTTPPost_BlockedURL(t *testing.T) {
	t.Parallel()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "http-post-blocked", HTTPAllowlist: []string{"allowed.com"}}
	registerSiplyAPI(L, plugin, nil, nil)

	err := L.DoString(`
		result, err_msg = siply.http.post("https://evil.com/data", "{}")
	`)
	require.NoError(t, err)

	errMsg := L.GetGlobal("err_msg")
	assert.Contains(t, errMsg.String(), "not on allowlist")
}
