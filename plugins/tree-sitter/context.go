// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxContextTokens = 500

type ContextStats struct {
	FileCount    int
	SymbolCount  int
	ParseTimeMS  int64
	CacheHitRate float64
}

type fileSymbols struct {
	relPath string
	symbols []Symbol
	mtime   time.Time
}

func GenerateContext(cache *FileCache, workspacePath string) (string, ContextStats) {
	start := time.Now()

	var files []fileSymbols

	_ = filepath.WalkDir(workspacePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", ".siply", "vendor", ".cache", "testdata":
				return filepath.SkipDir
			}
			return nil
		}

		if !cache.parser.SupportsFile(path) {
			return nil
		}

		symbols, parseErr := cache.GetOrParse(path)
		if parseErr != nil {
			return nil
		}

		relPath, _ := filepath.Rel(workspacePath, path)
		info, _ := d.Info()
		var mtime time.Time
		if info != nil {
			mtime = info.ModTime()
		}

		files = append(files, fileSymbols{
			relPath: relPath,
			symbols: symbols,
			mtime:   mtime,
		})

		return nil
	})

	if len(files) == 0 {
		return "", ContextStats{}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].mtime.After(files[j].mtime)
	})

	var b strings.Builder
	b.WriteString("[Code Context — tree-sitter]\n")

	totalSymbols := 0
	tokenEstimate := 0
	filesIncluded := 0

	for _, f := range files {
		if len(f.symbols) == 0 {
			continue
		}

		section := formatFileSection(f.relPath, f.symbols)
		sectionTokens := estimateTokens(section)

		if tokenEstimate+sectionTokens > maxContextTokens && filesIncluded > 0 {
			break
		}

		b.WriteString(section)
		tokenEstimate += sectionTokens
		totalSymbols += len(f.symbols)
		filesIncluded++
	}

	elapsed := time.Since(start)

	stats := ContextStats{
		FileCount:    filesIncluded,
		SymbolCount:  totalSymbols,
		ParseTimeMS:  elapsed.Milliseconds(),
		CacheHitRate: cache.CacheHitRate(),
	}

	return b.String(), stats
}

func formatFileSection(relPath string, symbols []Symbol) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n## %s\n", relPath)

	var pkg string
	var types []string
	var funcs []string
	var imports []string

	for _, s := range symbols {
		switch s.Kind {
		case "import":
			imports = append(imports, s.Name)
		case "struct", "interface", "type", "class":
			types = append(types, s.Name)
		case "function", "method":
			if s.Signature != "" {
				funcs = append(funcs, fmt.Sprintf("  - %s%s", s.Name, s.Signature))
			} else {
				funcs = append(funcs, fmt.Sprintf("  - %s", s.Name))
			}
		}
	}

	if strings.HasSuffix(relPath, ".go") {
		pkg = guessGoPackage(relPath)
	}

	if pkg != "" {
		fmt.Fprintf(&b, "Package: %s\n", pkg)
	}
	if len(types) > 0 {
		fmt.Fprintf(&b, "Types: %s\n", strings.Join(types, ", "))
	}
	if len(funcs) > 0 {
		b.WriteString("Functions:\n")
		for _, f := range funcs {
			b.WriteString(f)
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func guessGoPackage(relPath string) string {
	dir := filepath.Dir(relPath)
	if dir == "." {
		return "main"
	}
	return filepath.Base(dir)
}

func estimateTokens(text string) int {
	return len(text) / 4
}
