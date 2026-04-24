// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

type Symbol struct {
	Name      string
	Kind      string
	Line      int
	EndLine   int
	Signature string
}

func FormatSymbols(symbols []Symbol) string {
	var b strings.Builder
	for _, s := range symbols {
		if s.Signature != "" {
			fmt.Fprintf(&b, "%s %s: %s (L%d)\n", s.Kind, s.Name, s.Signature, s.Line)
		} else {
			fmt.Fprintf(&b, "%s %s (L%d)\n", s.Kind, s.Name, s.Line)
		}
	}
	return b.String()
}

func ExtractSymbols(tree *sitter.Tree, source []byte, lang string) []Symbol {
	root := tree.RootNode()
	var symbols []Symbol

	switch lang {
	case "go":
		symbols = extractGoSymbols(root, source)
	case "python":
		symbols = extractPythonSymbols(root, source)
	}

	return symbols
}

func extractGoSymbols(root *sitter.Node, source []byte) []Symbol {
	var symbols []Symbol
	walkNodes(root, func(node *sitter.Node) bool {
		if node.IsError() {
			return false
		}

		switch node.Type() {
		case "function_declaration":
			name := childByFieldName(node, "name", source)
			params := childByFieldName(node, "parameters", source)
			result := childByFieldName(node, "result", source)
			sig := params
			if result != "" {
				sig += " " + result
			}
			if name != "" {
				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      "function",
					Line:      int(node.StartPoint().Row) + 1,
					EndLine:   int(node.EndPoint().Row) + 1,
					Signature: sig,
				})
			}

		case "method_declaration":
			name := childByFieldName(node, "name", source)
			receiver := childByFieldName(node, "receiver", source)
			params := childByFieldName(node, "parameters", source)
			result := childByFieldName(node, "result", source)
			sig := receiver + " " + params
			if result != "" {
				sig += " " + result
			}
			if name != "" {
				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      "method",
					Line:      int(node.StartPoint().Row) + 1,
					EndLine:   int(node.EndPoint().Row) + 1,
					Signature: sig,
				})
			}

		case "type_declaration":
			for i := 0; i < int(node.NamedChildCount()); i++ {
				spec := node.NamedChild(i)
				if spec.Type() != "type_spec" {
					continue
				}
				typeName := childByFieldName(spec, "name", source)
				typeBody := spec.NamedChild(1)
				kind := "type"
				if typeBody != nil {
					switch typeBody.Type() {
					case "struct_type":
						kind = "struct"
					case "interface_type":
						kind = "interface"
					}
				}
				if typeName != "" {
					symbols = append(symbols, Symbol{
						Name:    typeName,
						Kind:    kind,
						Line:    int(spec.StartPoint().Row) + 1,
						EndLine: int(spec.EndPoint().Row) + 1,
					})
				}
			}

		case "import_declaration":
			text := nodeText(node, source)
			symbols = append(symbols, Symbol{
				Name: text,
				Kind: "import",
				Line: int(node.StartPoint().Row) + 1,
			})
		}

		return true
	})

	return symbols
}

func extractPythonSymbols(root *sitter.Node, source []byte) []Symbol {
	var symbols []Symbol
	walkNodes(root, func(node *sitter.Node) bool {
		if node.IsError() {
			return false
		}

		switch node.Type() {
		case "function_definition":
			name := childByFieldName(node, "name", source)
			params := childByFieldName(node, "parameters", source)
			if name != "" {
				symbols = append(symbols, Symbol{
					Name:      name,
					Kind:      "function",
					Line:      int(node.StartPoint().Row) + 1,
					EndLine:   int(node.EndPoint().Row) + 1,
					Signature: params,
				})
			}

		case "class_definition":
			name := childByFieldName(node, "name", source)
			if name != "" {
				symbols = append(symbols, Symbol{
					Name:    name,
					Kind:    "class",
					Line:    int(node.StartPoint().Row) + 1,
					EndLine: int(node.EndPoint().Row) + 1,
				})
			}

		case "import_statement", "import_from_statement":
			text := nodeText(node, source)
			symbols = append(symbols, Symbol{
				Name: text,
				Kind: "import",
				Line: int(node.StartPoint().Row) + 1,
			})

		case "decorated_definition":
			for i := 0; i < int(node.NamedChildCount()); i++ {
				child := node.NamedChild(i)
				if child.Type() == "function_definition" {
					name := childByFieldName(child, "name", source)
					params := childByFieldName(child, "parameters", source)
					if name != "" {
						symbols = append(symbols, Symbol{
							Name:      name,
							Kind:      "function",
							Line:      int(child.StartPoint().Row) + 1,
							EndLine:   int(child.EndPoint().Row) + 1,
							Signature: params,
						})
					}
					return false
				}
				if child.Type() == "class_definition" {
					name := childByFieldName(child, "name", source)
					if name != "" {
						symbols = append(symbols, Symbol{
							Name:    name,
							Kind:    "class",
							Line:    int(child.StartPoint().Row) + 1,
							EndLine: int(child.EndPoint().Row) + 1,
						})
					}
					return false
				}
			}
		}

		return true
	})

	return symbols
}

func walkNodes(node *sitter.Node, fn func(*sitter.Node) bool) {
	if node == nil {
		return
	}
	if !fn(node) {
		return
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkNodes(node.NamedChild(i), fn)
	}
}

func childByFieldName(node *sitter.Node, field string, source []byte) string {
	child := node.ChildByFieldName(field)
	if child == nil {
		return ""
	}
	return child.Content(source)
}

func nodeText(node *sitter.Node, source []byte) string {
	return node.Content(source)
}
