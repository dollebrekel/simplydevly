// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_LanguageForFile(t *testing.T) {
	p := NewParser()

	tests := []struct {
		path string
		want string
		ok   bool
	}{
		{"main.go", "go", true},
		{"server.py", "python", true},
		{"app.js", "", false},
		{"README.md", "", false},
		{"file.GO", "go", true},
		{"", "", false},
	}

	for _, tt := range tests {
		lang, ok := p.LanguageForFile(tt.path)
		assert.Equal(t, tt.want, lang, "path=%s", tt.path)
		assert.Equal(t, tt.ok, ok, "path=%s", tt.path)
	}
}

func TestParser_SupportsFile(t *testing.T) {
	p := NewParser()

	assert.True(t, p.SupportsFile("main.go"))
	assert.True(t, p.SupportsFile("server.py"))
	assert.False(t, p.SupportsFile("app.js"))
	assert.False(t, p.SupportsFile("README.md"))
}

func TestParser_ParseGo(t *testing.T) {
	p := NewParser()

	source := []byte(`package main

import "fmt"

func main() {
	fmt.Println("hello")
}

func add(a, b int) int {
	return a + b
}
`)

	tree, lang, err := p.Parse("main.go", source)
	require.NoError(t, err)
	assert.Equal(t, "go", lang)
	assert.NotNil(t, tree)

	root := tree.RootNode()
	assert.NotNil(t, root)
	assert.Equal(t, "source_file", root.Type())
}

func TestParser_ParsePython(t *testing.T) {
	p := NewParser()

	source := []byte(`import os

class MyClass:
    def __init__(self):
        pass

def hello(name):
    print(f"Hello {name}")
`)

	tree, lang, err := p.Parse("app.py", source)
	require.NoError(t, err)
	assert.Equal(t, "python", lang)
	assert.NotNil(t, tree)

	root := tree.RootNode()
	assert.NotNil(t, root)
	assert.Equal(t, "module", root.Type())
}

func TestParser_ParseUnsupported(t *testing.T) {
	p := NewParser()

	_, _, err := p.Parse("file.rs", []byte("fn main() {}"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported file extension")
}

func TestParser_ParseGoWithSyntaxError(t *testing.T) {
	p := NewParser()

	source := []byte(`package main

func valid() int {
	return 42
}

func broken( {
	// syntax error — missing closing paren
}

func alsoValid() string {
	return "ok"
}
`)

	tree, lang, err := p.Parse("broken.go", source)
	require.NoError(t, err)
	assert.Equal(t, "go", lang)
	assert.NotNil(t, tree)

	root := tree.RootNode()
	assert.True(t, root.HasError())
}
