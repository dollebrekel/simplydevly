// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthCmd_HasSubcommands(t *testing.T) {
	cmd := newAuthCmd()
	assert.Equal(t, "auth", cmd.Use)
	assert.NotEmpty(t, cmd.Short)

	names := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}
	assert.True(t, names["login"], "auth should have login subcommand")
	assert.True(t, names["logout"], "auth should have logout subcommand")
	assert.True(t, names["status"], "auth should have status subcommand")
	assert.True(t, names["pro"], "auth should have pro subcommand")
}

func TestLoginAlias_Works(t *testing.T) {
	alias := newLoginAlias()
	canonical := newLoginCmd()
	assert.Equal(t, canonical.Use, alias.Use, "alias should have same Use as canonical")
	assert.True(t, alias.Hidden, "login alias should be hidden from help")
	assert.NotNil(t, alias.RunE, "alias should have a RunE handler")
	assert.Equal(t, canonical.Short, alias.Short, "alias should have same Short as canonical")
}

func TestLogoutAlias_Works(t *testing.T) {
	alias := newLogoutAlias()
	canonical := newLogoutCmd()
	assert.Equal(t, canonical.Use, alias.Use, "alias should have same Use as canonical")
	assert.True(t, alias.Hidden, "logout alias should be hidden from help")
	assert.NotNil(t, alias.RunE, "alias should have a RunE handler")
}

func TestStatusAlias_Works(t *testing.T) {
	alias := newStatusAlias()
	canonical := newStatusCmd()
	assert.Equal(t, canonical.Use, alias.Use, "alias should have same Use as canonical")
	assert.True(t, alias.Hidden, "status alias should be hidden from help")
	assert.NotNil(t, alias.RunE, "alias should have a RunE handler")
}

func TestProAlias_Works(t *testing.T) {
	alias := newProAlias()
	canonical := newProCmd()
	assert.Equal(t, canonical.Use, alias.Use, "alias should have same Use as canonical")
	assert.True(t, alias.Hidden, "pro alias should be hidden from help")
	assert.Nil(t, alias.RunE, "pro is a parent command with no direct RunE")
	assert.NotEmpty(t, alias.Commands(), "pro alias should have subcommands")
}
