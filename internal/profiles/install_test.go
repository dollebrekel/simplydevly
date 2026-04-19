// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package profiles

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
)

func makeInstallerOK() InstallerFunc {
	return func(_ context.Context, name, _ string) error { return nil }
}

func makeProfile(items ...ProfileItem) *Profile {
	return &Profile{Name: "test", Version: "0.1.0", Items: items}
}

func TestInstallProfile_AllInstalled(t *testing.T) {
	profile := makeProfile(
		ProfileItem{Name: "plugA", Version: "1.0.0", Category: "plugins"},
		ProfileItem{Name: "skillB", Version: "2.0.0", Category: "skills"},
	)
	opts := InstallOptions{
		Profile:         profile,
		PluginInstaller: makeInstallerOK(),
		SkillInstaller:  makeInstallerOK(),
		Writer:          io.Discard,
	}
	result, err := InstallProfile(context.Background(), opts)
	require.NoError(t, err)
	assert.Len(t, result.Installed, 2)
	assert.Empty(t, result.Skipped)
	assert.Empty(t, result.Failed)
	assert.Empty(t, result.Conflicts)
}

func TestInstallProfile_SkipAlreadyAtSameVersion(t *testing.T) {
	profile := makeProfile(
		ProfileItem{Name: "plugA", Version: "1.0.0", Category: "plugins"},
	)
	opts := InstallOptions{
		Profile:         profile,
		PluginInstaller: makeInstallerOK(),
		Existing: ExistingItems{
			Plugins: []core.PluginMeta{{Name: "plugA", Version: "1.0.0"}},
		},
	}
	result, err := InstallProfile(context.Background(), opts)
	require.NoError(t, err)
	assert.Empty(t, result.Installed)
	assert.Equal(t, []string{"plugA"}, result.Skipped)
}

func TestInstallProfile_ConflictDetectionNoForce(t *testing.T) {
	profile := makeProfile(
		ProfileItem{Name: "plugA", Version: "1.0.0", Category: "plugins"},
	)
	opts := InstallOptions{
		Profile:         profile,
		PluginInstaller: makeInstallerOK(),
		Force:           false,
		Existing: ExistingItems{
			Plugins: []core.PluginMeta{{Name: "plugA", Version: "2.0.0"}},
		},
	}
	result, err := InstallProfile(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, result.NeedsConfirmation)
	require.Len(t, result.Conflicts, 1)
	assert.Equal(t, "plugA", result.Conflicts[0].Name)
	assert.Equal(t, "2.0.0", result.Conflicts[0].CurrentVersion)
	assert.Equal(t, "1.0.0", result.Conflicts[0].ProfileVersion)
	assert.Empty(t, result.Installed)
}

func TestInstallProfile_ForceOverwritesConflicts(t *testing.T) {
	profile := makeProfile(
		ProfileItem{Name: "plugA", Version: "1.0.0", Category: "plugins"},
	)
	opts := InstallOptions{
		Profile:         profile,
		PluginInstaller: makeInstallerOK(),
		Force:           true,
		Existing: ExistingItems{
			Plugins: []core.PluginMeta{{Name: "plugA", Version: "2.0.0"}},
		},
	}
	result, err := InstallProfile(context.Background(), opts)
	require.NoError(t, err)
	assert.False(t, result.NeedsConfirmation)
	assert.Equal(t, []string{"plugA"}, result.Installed)
}

func TestInstallProfile_PartialFailureContinues(t *testing.T) {
	called := 0
	installer := func(_ context.Context, name, _ string) error {
		called++
		if name == "failItem" {
			return errors.New("install error")
		}
		return nil
	}
	profile := makeProfile(
		ProfileItem{Name: "failItem", Version: "1.0.0", Category: "plugins"},
		ProfileItem{Name: "okItem", Version: "1.0.0", Category: "plugins"},
	)
	opts := InstallOptions{
		Profile:         profile,
		PluginInstaller: installer,
	}
	result, err := InstallProfile(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 2, called)
	assert.Equal(t, []string{"okItem"}, result.Installed)
	require.Len(t, result.Failed, 1)
	assert.Equal(t, "failItem", result.Failed[0].Name)
}

func TestInstallProfile_CategoryRouting(t *testing.T) {
	var pluginCalled, skillCalled, agentCalled bool
	profile := makeProfile(
		ProfileItem{Name: "p", Version: "1.0.0", Category: "plugins"},
		ProfileItem{Name: "s", Version: "1.0.0", Category: "skills"},
		ProfileItem{Name: "a", Version: "1.0.0", Category: "agents"},
	)
	opts := InstallOptions{
		Profile: profile,
		PluginInstaller: func(_ context.Context, _, _ string) error {
			pluginCalled = true
			return nil
		},
		SkillInstaller: func(_ context.Context, _, _ string) error {
			skillCalled = true
			return nil
		},
		AgentInstaller: func(_ context.Context, _, _ string) error {
			agentCalled = true
			return nil
		},
	}
	_, err := InstallProfile(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, pluginCalled)
	assert.True(t, skillCalled)
	assert.True(t, agentCalled)
}

func TestInstallProfile_NilProfileReturnsError(t *testing.T) {
	_, err := InstallProfile(context.Background(), InstallOptions{})
	require.Error(t, err)
}

func TestInstallProfile_NilInstallerForCategoryFails(t *testing.T) {
	profile := makeProfile(ProfileItem{Name: "p", Version: "1.0.0", Category: "plugins"})
	opts := InstallOptions{
		Profile:         profile,
		PluginInstaller: nil,
	}
	result, err := InstallProfile(context.Background(), opts)
	require.NoError(t, err)
	require.Len(t, result.Failed, 1)
	assert.Equal(t, "p", result.Failed[0].Name)
}
