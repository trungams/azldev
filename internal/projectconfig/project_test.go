// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package projectconfig_test

import (
	"testing"

	"github.com/microsoft/azure-linux-dev-tools/internal/projectconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ProjectInfo_MergeUpdatesFrom(t *testing.T) {
	// Create a project info with some initial values.
	projectInfo := &projectconfig.ProjectInfo{
		Description: "A",
		DefaultDistro: projectconfig.DistroReference{
			Name:    "distro",
			Version: "42.0",
		},
	}

	// Create another project info with some updates.
	updates := &projectconfig.ProjectInfo{
		Description: "B",
		DefaultDistro: projectconfig.DistroReference{
			Name:    "otherdistro",
			Version: "1.5",
		},
	}

	// Merge the updates into the original project info.
	err := projectInfo.MergeUpdatesFrom(updates)
	require.NoError(t, err)

	// Verify that the original project info has been updated correctly.
	assert.Equal(t, "B", projectInfo.Description)
	assert.Equal(t, "otherdistro", projectInfo.DefaultDistro.Name)
	assert.Equal(t, "1.5", projectInfo.DefaultDistro.Version)
}

func TestProjectInfoSpecEditor(t *testing.T) {
	t.Run("defaults to legacy", func(t *testing.T) {
		info := projectconfig.ProjectInfo{}
		info.ApplyProjectDefaults("/project")

		assert.Equal(t, projectconfig.SpecEditorLegacy, info.SpecEditor)
	})

	t.Run("accepts supported modes and rejects unknown mode", func(t *testing.T) {
		for _, mode := range []projectconfig.SpecEditor{
			projectconfig.SpecEditorLegacy,
			projectconfig.SpecEditorStructural,
		} {
			config := projectconfig.NewProjectConfig()
			config.Project.SpecEditor = mode
			require.NoError(t, config.Validate())
		}

		config := projectconfig.NewProjectConfig()
		config.Project.SpecEditor = "unknown"
		assert.ErrorContains(t, config.Validate(), "SpecEditor")
	})
}
