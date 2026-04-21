// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package extensions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"siply.dev/siply/internal/fileutil"
	"siply.dev/siply/internal/plugins"
)

// ScaffoldExtension creates a new extension project directory with manifest, main.go, and README.
func ScaffoldExtension(parentDir, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("extension: name is required")
	}

	dir := filepath.Join(parentDir, name)
	if _, err := os.Stat(dir); err == nil {
		return "", fmt.Errorf("extension: directory %q already exists", dir)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("extension: create directory: %w", err)
	}

	manifestData, err := buildExtensionManifest(name)
	if err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("extension: build manifest: %w", err)
	}

	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := fileutil.AtomicWriteFile(manifestPath, manifestData, 0o644); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("extension: write manifest.yaml: %w", err)
	}

	mainPath := filepath.Join(dir, "main.go")
	if err := fileutil.AtomicWriteFile(mainPath, []byte(buildExtensionMain(name)), 0o644); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("extension: write main.go: %w", err)
	}

	readmePath := filepath.Join(dir, "README.md")
	if err := fileutil.AtomicWriteFile(readmePath, []byte(buildExtensionReadme(name)), 0o644); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("extension: write README.md: %w", err)
	}

	m, err := plugins.LoadManifestFromDir(dir)
	if err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("extension: scaffolded manifest failed validation: %w", err)
	}
	_ = m

	return dir, nil
}

func buildExtensionManifest(name string) ([]byte, error) {
	m := plugins.Manifest{
		APIVersion: "siply/v1",
		Kind:       "Plugin",
		Metadata: plugins.Metadata{
			Name:        name,
			Version:     "0.1.0",
			SiplyMin:    "0.1.0",
			Description: fmt.Sprintf("%s extension for siply", toTitleCase(name)),
			Author:      "developer",
			License:     "MIT",
			Updated:     time.Now().UTC().Format("2006-01-02"),
		},
		Spec: plugins.Spec{
			Tier:         3,
			Capabilities: map[string]string{},
			Extensions: &plugins.ManifestExtensions{
				Panels: []plugins.ManifestPanel{
					{
						Name:        name + "-panel",
						Position:    "right",
						Collapsible: true,
						MenuLabel:   toTitleCase(name),
					},
				},
				MenuItems: []plugins.ManifestMenuItem{
					{
						Label:    toTitleCase(name),
						Category: "Extensions",
					},
				},
				Keybinds: []plugins.ManifestKeybind{
					{
						Key:         "ctrl+e",
						Description: "Toggle " + name,
					},
				},
			},
		},
	}

	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("scaffold: invalid manifest: %w", err)
	}
	return yaml.Marshal(m)
}

func buildExtensionMain(name string) string {
	title := toTitleCase(name)
	return fmt.Sprintf(`package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("Hello from %s extension")
	os.Exit(0)
}
`, title)
}

func buildExtensionReadme(name string) string {
	title := toTitleCase(name)
	return fmt.Sprintf("# %s\n\nA siply extension.\n\n## Development\n\n```bash\nsiply dev watch --plugin %s\n```\n", title, name)
}

func toTitleCase(name string) string {
	parts := strings.Split(name, "-")
	filtered := parts[:0]
	for _, p := range parts {
		if len(p) > 0 {
			filtered = append(filtered, strings.ToUpper(p[:1])+p[1:])
		}
	}
	return strings.Join(filtered, " ")
}
