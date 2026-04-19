// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package profiles

import (
	"context"
	"fmt"
	"io"

	"siply.dev/siply/internal/core"
)

// InstallerFunc installs a named marketplace item (by name and version) into the target directory.
type InstallerFunc func(ctx context.Context, name, version string) error

// ExistingItems holds currently installed items per category for conflict detection.
type ExistingItems struct {
	Plugins []core.PluginMeta
	Skills  []ProfileItem
	Agents  []ProfileItem
}

// InstallOptions configures the InstallProfile operation.
type InstallOptions struct {
	Profile         *Profile
	PluginInstaller InstallerFunc
	SkillInstaller  InstallerFunc
	AgentInstaller  InstallerFunc
	Existing        ExistingItems
	Force           bool
	Writer          io.Writer
}

// InstallResult summarizes the outcome of a profile install operation.
type InstallResult struct {
	Installed         []string
	Skipped           []string
	Failed            []InstallError
	Conflicts         []ItemConflict
	NeedsConfirmation bool
}

// ItemConflict describes a version mismatch between installed and profile item.
type ItemConflict struct {
	Name           string
	Category       string
	CurrentVersion string
	ProfileVersion string
}

// InstallError wraps an installation failure for a single item.
type InstallError struct {
	Name     string
	Category string
	Err      error
}

// InstallProfile installs all items from a profile, routing each to the correct installer.
// When conflicts exist and Force is false, it returns early with NeedsConfirmation=true
// so the caller can prompt the user before re-invoking with Force=true.
func InstallProfile(ctx context.Context, opts InstallOptions) (*InstallResult, error) {
	if opts.Profile == nil {
		return nil, fmt.Errorf("profiles: install: profile is required")
	}

	existing := buildExistingIndex(opts.Existing)

	result := &InstallResult{}
	var toInstall []ProfileItem

	for _, item := range opts.Profile.Items {
		key := itemKey(item.Category, item.Name)
		currentVer, installed := existing[key]
		switch {
		case installed && currentVer == item.Version:
			result.Skipped = append(result.Skipped, item.Name)
		case installed && currentVer != item.Version:
			result.Conflicts = append(result.Conflicts, ItemConflict{
				Name:           item.Name,
				Category:       item.Category,
				CurrentVersion: currentVer,
				ProfileVersion: item.Version,
			})
			if opts.Force {
				toInstall = append(toInstall, item)
			}
		default:
			toInstall = append(toInstall, item)
		}
	}

	if len(result.Conflicts) > 0 && !opts.Force {
		result.NeedsConfirmation = true
		return result, nil
	}

	w := opts.Writer
	if w == nil {
		w = io.Discard
	}

	for _, item := range toInstall {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		installer := opts.installerFor(item.Category)
		if installer == nil {
			result.Failed = append(result.Failed, InstallError{
				Name:     item.Name,
				Category: item.Category,
				Err:      fmt.Errorf("no installer configured for category %q", item.Category),
			})
			continue
		}

		if err := installer(ctx, item.Name, item.Version); err != nil {
			fmt.Fprintf(w, "  ✗ Failed to install %s (%s): %v\n", item.Name, item.Category, err)
			result.Failed = append(result.Failed, InstallError{
				Name:     item.Name,
				Category: item.Category,
				Err:      err,
			})
			continue
		}

		fmt.Fprintf(w, "  ✓ Installed %s (%s) v%s\n", item.Name, item.Category, item.Version)
		result.Installed = append(result.Installed, item.Name)
	}

	return result, nil
}

func (o InstallOptions) installerFor(category string) InstallerFunc {
	switch category {
	case "plugins":
		return o.PluginInstaller
	case "skills":
		return o.SkillInstaller
	case "agents":
		return o.AgentInstaller
	}
	return nil
}

// buildExistingIndex creates a fast lookup map "category/name" → version.
func buildExistingIndex(e ExistingItems) map[string]string {
	idx := make(map[string]string)
	for _, p := range e.Plugins {
		idx[itemKey("plugins", p.Name)] = p.Version
	}
	for _, s := range e.Skills {
		idx[itemKey("skills", s.Name)] = s.Version
	}
	for _, a := range e.Agents {
		idx[itemKey("agents", a.Name)] = a.Version
	}
	return idx
}

func itemKey(category, name string) string {
	return category + "/" + name
}
