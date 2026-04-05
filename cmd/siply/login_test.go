package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLoginCmd(t *testing.T) {
	cmd := newLoginCmd()
	assert.Equal(t, "login", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotNil(t, cmd.RunE)
}

func TestNewLogoutCmd(t *testing.T) {
	cmd := newLogoutCmd()
	assert.Equal(t, "logout", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotNil(t, cmd.RunE)
}

func TestDefaultConfigDir(t *testing.T) {
	dir, err := defaultConfigDir()
	assert.NoError(t, err)
	assert.Contains(t, dir, ".siply")
}
