// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package licensing

import (
	"errors"

	"siply.dev/siply/internal/core"
)

// ErrNotAuthenticated is returned when an operation requires authentication.
var ErrNotAuthenticated = errors.New("Authentication required. Run 'siply login' first.")

// RequireAuth checks if the user is logged in and returns an actionable error if not.
func RequireAuth(validator core.LicenseValidator) error {
	status := validator.Validate()
	if !status.LoggedIn {
		return ErrNotAuthenticated
	}
	return nil
}

// AccountToken returns the stored account token for API calls.
// Requires the concrete *licenseValidator type (type-assert at call site).
func AccountToken(validator core.LicenseValidator) (string, error) {
	v, ok := validator.(*licenseValidator)
	if !ok {
		return "", errors.New("licensing: unsupported validator type for token access")
	}
	return v.Token()
}
