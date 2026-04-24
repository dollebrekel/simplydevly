// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/python"
)

var langExtensions = map[string]string{
	".go": "go",
	".py": "python",
}

type Parser struct {
	languages map[string]*sitter.Language
}

func NewParser() *Parser {
	return &Parser{
		languages: map[string]*sitter.Language{
			"go":     golang.GetLanguage(),
			"python": python.GetLanguage(),
		},
	}
}

func (p *Parser) LanguageForFile(path string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	lang, ok := langExtensions[ext]
	return lang, ok
}

func (p *Parser) Parse(path string, content []byte) (*sitter.Tree, string, error) {
	lang, ok := p.LanguageForFile(path)
	if !ok {
		return nil, "", fmt.Errorf("unsupported file extension: %s", filepath.Ext(path))
	}

	sitterLang, ok := p.languages[lang]
	if !ok {
		return nil, "", fmt.Errorf("language not registered: %s", lang)
	}

	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(sitterLang)

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, "", fmt.Errorf("parse %s: %w", path, err)
	}

	return tree, lang, nil
}

func (p *Parser) SupportsFile(path string) bool {
	_, ok := p.LanguageForFile(path)
	return ok
}
