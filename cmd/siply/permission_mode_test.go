// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestYoloCommandPrintsUsage(t *testing.T) {
	cmd := newYoloCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "/yolo")
	assert.Contains(t, out.String(), "siply run --yolo")
}
