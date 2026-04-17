// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package licensing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
)

func newTestValidator(t *testing.T, configDir string) core.LicenseValidator {
	t.Helper()
	bus := events.NewBus()
	require.NoError(t, bus.Init(context.Background()))
	require.NoError(t, bus.Start(context.Background()))
	t.Cleanup(func() { _ = bus.Stop(context.Background()) })

	v := NewLicenseValidator(bus, configDir)
	require.NoError(t, v.Init(context.Background()))
	require.NoError(t, v.Start(context.Background()))
	t.Cleanup(func() { _ = v.Stop(context.Background()) })
	return v
}

func TestRequireAuth_NotLoggedIn(t *testing.T) {
	dir := t.TempDir()
	v := newTestValidator(t, dir)

	err := RequireAuth(v)
	assert.ErrorIs(t, err, ErrNotAuthenticated)
}

func TestRequireAuth_LoggedIn(t *testing.T) {
	dir := t.TempDir()
	v := newTestValidator(t, dir)

	// Manually inject a cached account to simulate logged-in state.
	lv := v.(*licenseValidator)
	lv.mu.Lock()
	lv.cached = &accountData{
		AuthProvider: "github",
		DisplayName:  "Test User",
		AccountEmail: "test@example.com",
		InstanceID:   "test-instance",
		Token:        "fake-token",
	}
	lv.mu.Unlock()

	err := RequireAuth(v)
	assert.NoError(t, err)
}

func TestAccountToken_LoggedIn(t *testing.T) {
	dir := t.TempDir()
	v := newTestValidator(t, dir)

	lv := v.(*licenseValidator)
	lv.mu.Lock()
	lv.cached = &accountData{Token: "my-secret-token"}
	lv.mu.Unlock()

	token, err := AccountToken(v)
	require.NoError(t, err)
	assert.Equal(t, "my-secret-token", token)
}

func TestAccountToken_NotLoggedIn(t *testing.T) {
	dir := t.TempDir()
	v := newTestValidator(t, dir)

	_, err := AccountToken(v)
	assert.ErrorIs(t, err, ErrNotAuthenticated)
}
