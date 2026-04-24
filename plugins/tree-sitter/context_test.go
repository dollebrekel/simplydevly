// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateContext_BasicGo(t *testing.T) {
	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import "fmt"

func main() {
	fmt.Println("hello")
}

type App struct {
	Name string
}
`), 0o644)
	require.NoError(t, err)

	parser := NewParser()
	cache := NewFileCache(parser, 100)

	context, stats := GenerateContext(cache, dir)

	assert.NotEmpty(t, context)
	assert.Contains(t, context, "[Code Context — tree-sitter]")
	assert.Contains(t, context, "main.go")
	assert.Contains(t, context, "main")
	assert.Contains(t, context, "App")
	assert.Greater(t, stats.FileCount, 0)
	assert.Greater(t, stats.SymbolCount, 0)
}

func TestGenerateContext_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	parser := NewParser()
	cache := NewFileCache(parser, 100)

	context, stats := GenerateContext(cache, dir)

	assert.Empty(t, context)
	assert.Equal(t, 0, stats.FileCount)
}

func TestGenerateContext_MixedLanguages(t *testing.T) {
	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() {}
`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "app.py"), []byte(`class App:
    pass

def run():
    pass
`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Hello"), 0o644)
	require.NoError(t, err)

	parser := NewParser()
	cache := NewFileCache(parser, 100)

	context, stats := GenerateContext(cache, dir)

	assert.NotEmpty(t, context)
	assert.Equal(t, 2, stats.FileCount)
}

func TestGenerateContext_TokenLimit(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 50; i++ {
		name := filepath.Join(dir, "file_"+string(rune('a'+i%26))+string(rune('0'+i/26))+".go")
		content := "package main\n\n"
		for j := 0; j < 20; j++ {
			content += "func Function" + string(rune('A'+j)) + "() {}\n"
		}
		err := os.WriteFile(name, []byte(content), 0o644)
		require.NoError(t, err)
	}

	parser := NewParser()
	cache := NewFileCache(parser, 1000)

	context, stats := GenerateContext(cache, dir)

	assert.NotEmpty(t, context)
	tokens := estimateTokens(context)
	assert.LessOrEqual(t, tokens, maxContextTokens+500)
	assert.Less(t, stats.FileCount, 50)
}

func TestGenerateContext_SkipsVendor(t *testing.T) {
	dir := t.TempDir()

	err := os.MkdirAll(filepath.Join(dir, "vendor"), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "vendor", "lib.go"), []byte(`package vendor
func Lib() {}
`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main
func main() {}
`), 0o644)
	require.NoError(t, err)

	parser := NewParser()
	cache := NewFileCache(parser, 100)

	context, stats := GenerateContext(cache, dir)

	assert.NotEmpty(t, context)
	assert.Equal(t, 1, stats.FileCount)
	assert.NotContains(t, context, "vendor")
}

func TestEstimateTokens(t *testing.T) {
	assert.Equal(t, 0, estimateTokens(""))
	assert.Equal(t, 1, estimateTokens("abcd"))
	assert.Equal(t, 2, estimateTokens("abcdefgh"))
}

func TestGuessGoPackage(t *testing.T) {
	assert.Equal(t, "main", guessGoPackage("main.go"))
	assert.Equal(t, "routing", guessGoPackage("internal/routing/provider.go"))
	assert.Equal(t, "core", guessGoPackage("internal/core/hooks.go"))
}

func TestFormatFileSection(t *testing.T) {
	symbols := []Symbol{
		{Name: "import \"fmt\"", Kind: "import", Line: 3},
		{Name: "App", Kind: "struct", Line: 5},
		{Name: "NewApp", Kind: "function", Line: 10, Signature: "() *App"},
		{Name: "Run", Kind: "method", Line: 15, Signature: "(a *App) ()"},
	}

	result := formatFileSection("internal/app/app.go", symbols)

	assert.Contains(t, result, "## internal/app/app.go")
	assert.Contains(t, result, "Package: app")
	assert.Contains(t, result, "App")
	assert.Contains(t, result, "NewApp")
	assert.Contains(t, result, "Run")
}
