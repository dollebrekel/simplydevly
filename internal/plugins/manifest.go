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
	Tier         int                 `yaml:"tier"`
	Capabilities map[string]string   `yaml:"capabilities"`
	Category     string              `yaml:"category,omitempty"`
	Components   []ManifestComponent `yaml:"components,omitempty"`
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
	if m.Kind != "Plugin" && m.Kind != "Bundle" {
		errs = append(errs, fmt.Errorf("kind must be \"Plugin\" or \"Bundle\", got %q", m.Kind))
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

	if m.Kind == "Bundle" {
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
	} else {
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

	if len(errs) > 0 {
		return fmt.Errorf("%w: %v", ErrInvalidManifest, errors.Join(errs...))
	}
	return nil
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
