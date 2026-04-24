// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest represents a parsed plugin manifest.yaml file.
type Manifest struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

// Metadata holds plugin identification and authorship information.
type Metadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	SiplyMin    string `yaml:"siply_min"`
	Description string `yaml:"description"`
	Author      string `yaml:"author"`
	License     string `yaml:"license"`
	Updated     string `yaml:"updated,omitempty"`
}

// Spec holds plugin tier and capability declarations.
type Spec struct {
	Tier          int                 `yaml:"tier"`
	Capabilities  map[string]string   `yaml:"capabilities"`
	Category      string              `yaml:"category,omitempty"`
	Components    []ManifestComponent `yaml:"components,omitempty"`
	Extensions    *ManifestExtensions `yaml:"extensions,omitempty"`
	HTTPAllowlist []string            `yaml:"http_allowlist,omitempty"`
}

// ManifestExtensions declares extension registrations in a plugin manifest.
type ManifestExtensions struct {
	Panels    []ManifestPanel    `yaml:"panels,omitempty"`
	MenuItems []ManifestMenuItem `yaml:"menu_items,omitempty"`
	Keybinds  []ManifestKeybind  `yaml:"keybindings,omitempty"`
}

// ManifestPanel declares a panel registration.
type ManifestPanel struct {
	Name        string `yaml:"name"`
	Position    string `yaml:"position"`
	MinWidth    int    `yaml:"min_width,omitempty"`
	MaxWidth    int    `yaml:"max_width,omitempty"`
	Collapsible bool   `yaml:"collapsible,omitempty"`
	Keybind     string `yaml:"keybind,omitempty"`
	Icon        string `yaml:"icon,omitempty"`
	MenuLabel   string `yaml:"menu_label,omitempty"`
}

// ManifestMenuItem declares a menu item registration.
type ManifestMenuItem struct {
	Label    string `yaml:"label"`
	Icon     string `yaml:"icon,omitempty"`
	Keybind  string `yaml:"keybind,omitempty"`
	Category string `yaml:"category,omitempty"`
}

// ManifestKeybind declares a keybinding registration.
type ManifestKeybind struct {
	Key         string `yaml:"key"`
	Description string `yaml:"description,omitempty"`
}

// ManifestComponent represents a single component reference within a bundle manifest.
type ManifestComponent struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// Sentinel errors for manifest operations.
var (
	ErrInvalidManifest          = errors.New("plugins: invalid manifest")
	ErrManifestNotFound         = errors.New("plugins: manifest.yaml not found")
	ErrManifestTooLarge         = errors.New("plugins: manifest.yaml exceeds 1MB size limit")
	ErrBundleEmptyComponents    = errors.New("bundle has no components")
	ErrBundleSelfReference      = errors.New("bundle cannot reference itself as a component")
	ErrBundleDuplicateComponent = errors.New("bundle contains duplicate component names")
)

// maxManifestSize is the maximum allowed manifest file size (1MB per NFR12).
const maxManifestSize = 1 << 20

// namePattern validates plugin names: lowercase, starting with letter, containing a-z, 0-9, hyphens.
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// semverPattern validates semantic versioning strings.
var semverPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

// knownCapabilityKeys is the set of valid capability keys.
var knownCapabilityKeys = map[string]bool{
	"filesystem":  true,
	"network":     true,
	"credentials": true,
	"bash":        true,
}

// knownCapabilityValues maps each capability key to its valid values.
var knownCapabilityValues = map[string]map[string]bool{
	"filesystem":  {"read": true, "write": true, "readwrite": true, "none": true},
	"network":     {"allowed": true, "none": true},
	"credentials": {"allowed": true, "none": true},
	"bash":        {"allowed": true, "none": true},
}

// ParseManifest parses raw YAML bytes into a Manifest using strict decoding.
// Strict decoding rejects unknown fields — this is appropriate for author-controlled
// manifest files (not user config).
func ParseManifest(data []byte) (*Manifest, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty manifest data", ErrInvalidManifest)
	}

	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}

	// Reject multi-document YAML files.
	var extra interface{}
	if dec.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("%w: manifest contains multiple YAML documents", ErrInvalidManifest)
	}

	return &m, nil
}

// Validate checks all mandatory fields and value constraints on a Manifest.
func (m *Manifest) Validate() error {
	if m == nil {
		return fmt.Errorf("%w: nil manifest", ErrInvalidManifest)
	}

	var errs []error

	// apiVersion
	if m.APIVersion != "siply/v1" {
		errs = append(errs, fmt.Errorf("apiVersion must be \"siply/v1\", got %q", m.APIVersion))
	}

	// kind
	if m.Kind != "Plugin" && m.Kind != "Bundle" && m.Kind != "Profile" {
		errs = append(errs, fmt.Errorf("kind must be \"Plugin\", \"Bundle\", or \"Profile\", got %q", m.Kind))
	}

	// metadata.name
	if m.Metadata.Name == "" {
		errs = append(errs, fmt.Errorf("metadata.name is required"))
	} else if !namePattern.MatchString(m.Metadata.Name) {
		errs = append(errs, fmt.Errorf("metadata.name must match ^[a-z][a-z0-9-]{0,62}$ (max 63 chars), got %q", m.Metadata.Name))
	}

	// metadata.version
	if m.Metadata.Version == "" {
		errs = append(errs, fmt.Errorf("metadata.version is required"))
	} else if !semverPattern.MatchString(m.Metadata.Version) {
		errs = append(errs, fmt.Errorf("metadata.version must be valid semver, got %q", m.Metadata.Version))
	}

	// metadata.siply_min
	if m.Metadata.SiplyMin == "" {
		errs = append(errs, fmt.Errorf("metadata.siply_min is required"))
	} else if !semverPattern.MatchString(m.Metadata.SiplyMin) {
		errs = append(errs, fmt.Errorf("metadata.siply_min must be valid semver, got %q", m.Metadata.SiplyMin))
	}

	// metadata.description
	if m.Metadata.Description == "" {
		errs = append(errs, fmt.Errorf("metadata.description is required"))
	}

	// metadata.author
	if m.Metadata.Author == "" {
		errs = append(errs, fmt.Errorf("metadata.author is required"))
	}

	// metadata.license
	if m.Metadata.License == "" {
		errs = append(errs, fmt.Errorf("metadata.license is required"))
	}

	// metadata.updated (FR35: mandatory versioning field)
	if m.Metadata.Updated == "" {
		errs = append(errs, fmt.Errorf("metadata.updated is required"))
	}

	switch m.Kind {
	case "Bundle":
		// Bundle-specific validation: components required, tier/capabilities skipped.
		if len(m.Spec.Components) == 0 {
			errs = append(errs, fmt.Errorf("%w", ErrBundleEmptyComponents))
		} else {
			seen := make(map[string]bool, len(m.Spec.Components))
			for _, comp := range m.Spec.Components {
				if !namePattern.MatchString(comp.Name) {
					errs = append(errs, fmt.Errorf("spec.components: name must match ^[a-z][a-z0-9-]{0,62}$, got %q", comp.Name))
				}
				if !semverPattern.MatchString(comp.Version) {
					errs = append(errs, fmt.Errorf("spec.components: version must be valid semver for %q, got %q", comp.Name, comp.Version))
				}
				if comp.Name == m.Metadata.Name {
					errs = append(errs, fmt.Errorf("%w: %q", ErrBundleSelfReference, comp.Name))
				}
				if seen[comp.Name] {
					errs = append(errs, fmt.Errorf("%w: %q", ErrBundleDuplicateComponent, comp.Name))
				}
				seen[comp.Name] = true
			}
		}
	case "Profile":
		if m.Spec.Category != "profiles" {
			errs = append(errs, fmt.Errorf("spec.category must be \"profiles\" for Profile kind, got %q", m.Spec.Category))
		}
		if m.Spec.Tier != 1 {
			errs = append(errs, fmt.Errorf("spec.tier must be 1 for Profile kind, got %d", m.Spec.Tier))
		}
		if len(m.Spec.Capabilities) > 0 {
			errs = append(errs, fmt.Errorf("spec.capabilities must be empty for Profile kind"))
		}
	default:
		// Plugin-specific validation.
		// spec.tier
		if m.Spec.Tier < 1 || m.Spec.Tier > 3 {
			errs = append(errs, fmt.Errorf("spec.tier must be 1, 2, or 3, got %d", m.Spec.Tier))
		}

		// spec.capabilities
		for key, val := range m.Spec.Capabilities {
			if !knownCapabilityKeys[key] {
				errs = append(errs, fmt.Errorf("spec.capabilities has unknown key %q, allowed: filesystem, network, credentials, bash", key))
			} else if validVals, ok := knownCapabilityValues[key]; ok && !validVals[val] {
				errs = append(errs, fmt.Errorf("spec.capabilities[%q] has invalid value %q", key, val))
			}
		}
	}

	if m.Spec.Extensions != nil {
		errs = append(errs, m.validateExtensions()...)
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %v", ErrInvalidManifest, errors.Join(errs...))
	}
	return nil
}

// validPositions lists valid panel position strings.
var validPositions = map[string]bool{
	"left": true, "right": true, "bottom": true,
}

// reservedKeybinds mirrors the built-in keybinds from the TUI.
// Duplicated here to avoid import cycle with internal/extensions.
var reservedKeybinds = map[string]bool{
	"ctrl+c": true, "ctrl+@": true, "ctrl+space": true,
	"ctrl+b": true, "tab": true, "shift+tab": true, "alt+left": true,
	"alt+right": true, "ctrl+]": true, "ctrl+[": true, "esc": true, "q": true,
}

func (m *Manifest) validateExtensions() []error {
	ext := m.Spec.Extensions
	var errs []error

	panelNames := make(map[string]bool)
	for _, p := range ext.Panels {
		if p.Name == "" {
			errs = append(errs, fmt.Errorf("extensions.panels: name is required"))
		} else if panelNames[p.Name] {
			errs = append(errs, fmt.Errorf("extensions.panels: duplicate panel name %q", p.Name))
		} else {
			panelNames[p.Name] = true
		}
		if p.Position != "" && !validPositions[p.Position] {
			errs = append(errs, fmt.Errorf("extensions.panels[%q]: position must be left/right/bottom, got %q", p.Name, p.Position))
		}
	}

	type menuKey struct{ label, category string }
	menuKeys := make(map[menuKey]bool)
	for _, mi := range ext.MenuItems {
		if mi.Label == "" {
			errs = append(errs, fmt.Errorf("extensions.menu_items: label is required"))
		} else {
			cat := mi.Category
			if cat == "" {
				cat = "Extensions"
			}
			k := menuKey{mi.Label, cat}
			if menuKeys[k] {
				errs = append(errs, fmt.Errorf("extensions.menu_items: duplicate label %q in category %q", mi.Label, cat))
			}
			menuKeys[k] = true
		}
	}

	keybindKeys := make(map[string]bool)
	for _, kb := range ext.Keybinds {
		if kb.Key == "" {
			errs = append(errs, fmt.Errorf("extensions.keybindings: key is required"))
			continue
		}
		normalized := strings.ToLower(kb.Key)
		if reservedKeybinds[normalized] {
			errs = append(errs, fmt.Errorf("extensions.keybindings: key %q conflicts with built-in keybinding", kb.Key))
		}
		if keybindKeys[normalized] {
			errs = append(errs, fmt.Errorf("extensions.keybindings: duplicate key %q", kb.Key))
		} else {
			keybindKeys[normalized] = true
		}
	}

	return errs
}

// LoadManifestFromDir reads and parses manifest.yaml from a plugin directory.
func LoadManifestFromDir(pluginDir string) (*Manifest, error) {
	if pluginDir == "" {
		return nil, fmt.Errorf("plugins: pluginDir is empty")
	}

	manifestPath := filepath.Join(pluginDir, "manifest.yaml")

	// Use Lstat to reject symlinks (prevent reading arbitrary files via symlink).
	info, err := os.Lstat(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrManifestNotFound, manifestPath)
		}
		return nil, fmt.Errorf("plugins: stat manifest: %w", err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %s is a symlink", ErrInvalidManifest, manifestPath)
	}

	f, err := os.Open(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("plugins: read manifest: %w", err)
	}
	defer f.Close()

	// Read with limit to prevent large allocations (TOCTOU-safe).
	data, err := io.ReadAll(io.LimitReader(f, maxManifestSize+1))
	if err != nil {
		return nil, fmt.Errorf("plugins: read manifest: %w", err)
	}
	if int64(len(data)) > maxManifestSize {
		return nil, fmt.Errorf("%w: %s exceeds 1MB", ErrManifestTooLarge, manifestPath)
	}

	m, err := ParseManifest(data)
	if err != nil {
		return nil, fmt.Errorf("plugins: parse %s: %w", manifestPath, err)
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}

	return m, nil
}
