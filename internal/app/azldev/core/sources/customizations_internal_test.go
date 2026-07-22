// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package sources

import (
	"context"
	"encoding/json"
	"os/exec"
	"slices"
	"testing"

	"github.com/microsoft/azure-linux-dev-tools/internal/global/testctx"
	"github.com/microsoft/azure-linux-dev-tools/internal/projectconfig"
	"github.com/microsoft/azure-linux-dev-tools/internal/utils/fileutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// argValue returns the value that follows the given flag in args, or "" if not found.
func argValue(args []string, flag string) string {
	idx := slices.Index(args, flag)
	if idx < 0 || idx+1 >= len(args) {
		return ""
	}

	return args[idx+1]
}

func TestApplyCustomizations_BuildsArgsAndJSON(t *testing.T) {
	t.Setenv(customizeToolEnvVar, "")

	ctrl := gomock.NewController(t)
	ctx := testctx.NewCtx()
	memFS := ctx.FS()

	var (
		capturedArgs []string
		capturedJSON []byte
		invoked      int
	)

	ctx.CmdFactory.RunHandler = func(cmd *exec.Cmd) error {
		invoked++
		capturedArgs = slices.Clone(cmd.Args)

		jsonPath := argValue(cmd.Args, "--customizations")
		require.NotEmpty(t, jsonPath, "expected --customizations arg")

		data, err := fileutils.ReadFile(memFS, jsonPath)
		require.NoError(t, err)

		capturedJSON = data

		return nil
	}

	preparer := &sourcePreparerImpl{
		fs:            memFS,
		eventListener: ctx,
		cmdFactory:    ctx.CmdFactory,
	}

	enabled := true
	disabled := false
	config := &projectconfig.ComponentConfig{
		Customizations: []projectconfig.ComponentCustomization{
			{Type: projectconfig.ComponentCustomizeBuildOption, Option: "mingw", Enabled: &disabled},
			{Type: projectconfig.ComponentCustomizeTests, Enabled: &enabled},
			{Type: projectconfig.ComponentCustomizeRemoveSubpackage, Package: "doc"},
		},
	}
	comp := mockComponent(ctrl, "fltk", config)

	const specPath = "/sources/fltk/fltk.spec"

	err := preparer.applyCustomizations(context.Background(), comp, specPath)
	require.NoError(t, err)

	require.Equal(t, 1, invoked, "tool should be invoked exactly once")

	// Assert the CLI argument vector.
	require.NotEmpty(t, capturedArgs)
	assert.Equal(t, defaultCustomizeTool, capturedArgs[0], "tool name should default to rpm-spec-customize")
	assert.Equal(t, specPath, argValue(capturedArgs, "--spec"))
	assert.NotEmpty(t, argValue(capturedArgs, "--customizations"))
	assert.Contains(t, capturedArgs, "--in-place")
	assert.Contains(t, capturedArgs, "--strict")

	// Assert the JSON document mapping.
	var doc struct {
		Customizations []map[string]any `json:"customizations"`
	}
	require.NoError(t, json.Unmarshal(capturedJSON, &doc))
	require.Len(t, doc.Customizations, 3)

	assert.Equal(t, map[string]any{
		"kind":    "build-option",
		"option":  "mingw",
		"enabled": false,
	}, doc.Customizations[0])
	assert.Equal(t, map[string]any{
		"kind":    "tests",
		"enabled": true,
	}, doc.Customizations[1])
	assert.Equal(t, map[string]any{
		"kind":    "remove-subpackage",
		"package": "doc",
	}, doc.Customizations[2])
}

func TestApplyCustomizations_EmptyDoesNotInvokeTool(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := testctx.NewCtx()

	invoked := false
	ctx.CmdFactory.RunHandler = func(_ *exec.Cmd) error {
		invoked = true

		return nil
	}

	preparer := &sourcePreparerImpl{
		fs:            ctx.FS(),
		eventListener: ctx,
		cmdFactory:    ctx.CmdFactory,
	}

	comp := mockComponent(ctrl, "no-customizations", &projectconfig.ComponentConfig{})

	err := preparer.applyCustomizations(context.Background(), comp, "/sources/x/x.spec")
	require.NoError(t, err)
	assert.False(t, invoked, "tool must not be invoked when there are no customizations")
}

func TestApplyCustomizations_ToolEnvOverride(t *testing.T) {
	const customTool = "/opt/tools/my-rpm-spec-customize"

	t.Setenv(customizeToolEnvVar, customTool)

	ctrl := gomock.NewController(t)
	ctx := testctx.NewCtx()

	var toolName string

	ctx.CmdFactory.RunHandler = func(cmd *exec.Cmd) error {
		toolName = cmd.Args[0]

		return nil
	}

	preparer := &sourcePreparerImpl{
		fs:            ctx.FS(),
		eventListener: ctx,
		cmdFactory:    ctx.CmdFactory,
	}

	enabled := true
	config := &projectconfig.ComponentConfig{
		Customizations: []projectconfig.ComponentCustomization{
			{Type: projectconfig.ComponentCustomizeTests, Enabled: &enabled},
		},
	}
	comp := mockComponent(ctrl, "pkg", config)

	err := preparer.applyCustomizations(context.Background(), comp, "/sources/pkg/pkg.spec")
	require.NoError(t, err)
	assert.Equal(t, customTool, toolName)
}
