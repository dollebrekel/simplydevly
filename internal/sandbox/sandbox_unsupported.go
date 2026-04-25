// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

//go:build !linux && !darwin

package sandbox

import "context"

// UnsupportedSandbox is used on platforms where no sandbox runtime exists.
type UnsupportedSandbox struct{}

func NewProvider(_ Config) SandboxProvider {
	return &UnsupportedSandbox{}
}

func (u *UnsupportedSandbox) Execute(_ context.Context, _ string, _ SandboxOptions) (SandboxResult, error) {
	return SandboxResult{}, ErrUnavailable
}

func (u *UnsupportedSandbox) Available() bool        { return false }
func (u *UnsupportedSandbox) Capabilities() SandboxCaps {
	return SandboxCaps{Platform: "unsupported"}
}
func (u *UnsupportedSandbox) Close() error { return nil }
