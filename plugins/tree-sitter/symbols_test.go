// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractSymbols_Go(t *testing.T) {
	p := NewParser()

	source := []byte(`package routing

import "context"

type RoutingProvider struct {
	providers []Provider
	policy    CostPolicy
}

type Queryable interface {
	Query(ctx context.Context) error
}

func NewRoutingProvider(providers []Provider, policy CostPolicy) *RoutingProvider {
	return &RoutingProvider{providers: providers, policy: policy}
}

func (rp *RoutingProvider) Query(ctx context.Context) error {
	return nil
}
`)

	tree, lang, err := p.Parse("provider.go", source)
	require.NoError(t, err)

	symbols := ExtractSymbols(tree, source, lang)

	var kinds []string
	var names []string
	for _, s := range symbols {
		kinds = append(kinds, s.Kind)
		names = append(names, s.Name)
	}

	assert.Contains(t, names, "import \"context\"", "should extract import")
	assert.Contains(t, names, "RoutingProvider", "should extract struct")
	assert.Contains(t, names, "Queryable", "should extract interface")
	assert.Contains(t, names, "NewRoutingProvider", "should extract function")
	assert.Contains(t, names, "Query", "should extract method")

	for _, s := range symbols {
		switch s.Name {
		case "RoutingProvider":
			assert.Equal(t, "struct", s.Kind)
		case "Queryable":
			assert.Equal(t, "interface", s.Kind)
		case "NewRoutingProvider":
			assert.Equal(t, "function", s.Kind)
			assert.NotEmpty(t, s.Signature)
		case "Query":
			assert.Equal(t, "method", s.Kind)
			assert.NotEmpty(t, s.Signature)
		}
	}
}

func TestExtractSymbols_Python(t *testing.T) {
	p := NewParser()

	source := []byte(`import os
from pathlib import Path

class FileProcessor:
    def __init__(self, path):
        self.path = path

    def process(self):
        pass

def standalone_func(x, y):
    return x + y
`)

	tree, lang, err := p.Parse("processor.py", source)
	require.NoError(t, err)

	symbols := ExtractSymbols(tree, source, lang)

	var names []string
	for _, s := range symbols {
		names = append(names, s.Name)
	}

	assert.Contains(t, names, "import os", "should extract import")
	assert.Contains(t, names, "from pathlib import Path", "should extract from-import")
	assert.Contains(t, names, "FileProcessor", "should extract class")
	assert.Contains(t, names, "standalone_func", "should extract function")

	for _, s := range symbols {
		switch s.Name {
		case "FileProcessor":
			assert.Equal(t, "class", s.Kind)
		case "standalone_func":
			assert.Equal(t, "function", s.Kind)
			assert.NotEmpty(t, s.Signature)
		}
	}
}

func TestExtractSymbols_GoWithSyntaxError(t *testing.T) {
	p := NewParser()

	source := []byte(`package main

func valid() int {
	return 42
}

func broken( {
}

func alsoValid() string {
	return "ok"
}
`)

	tree, lang, err := p.Parse("broken.go", source)
	require.NoError(t, err)

	symbols := ExtractSymbols(tree, source, lang)

	var names []string
	for _, s := range symbols {
		names = append(names, s.Name)
	}

	assert.Contains(t, names, "valid", "should extract symbol from valid region")
	assert.Contains(t, names, "alsoValid", "should extract symbol from valid region after error")
}

func TestExtractSymbols_PythonDecorated(t *testing.T) {
	p := NewParser()

	source := []byte(`from functools import wraps

def my_decorator(func):
    @wraps(func)
    def wrapper(*args, **kwargs):
        return func(*args, **kwargs)
    return wrapper

@my_decorator
def decorated_function(x):
    return x * 2

class MyClass:
    @staticmethod
    def static_method():
        pass
`)

	tree, lang, err := p.Parse("decorated.py", source)
	require.NoError(t, err)

	symbols := ExtractSymbols(tree, source, lang)

	var names []string
	for _, s := range symbols {
		names = append(names, s.Name)
	}

	assert.Contains(t, names, "my_decorator", "should extract regular function")
	assert.Contains(t, names, "decorated_function", "should extract decorated function")
	assert.Contains(t, names, "MyClass", "should extract class")
}

func TestFormatSymbols(t *testing.T) {
	symbols := []Symbol{
		{Name: "NewParser", Kind: "function", Line: 10, Signature: "() *Parser"},
		{Name: "Parser", Kind: "struct", Line: 5},
	}

	result := FormatSymbols(symbols)
	assert.Contains(t, result, "function NewParser: () *Parser (L10)")
	assert.Contains(t, result, "struct Parser (L5)")
}
