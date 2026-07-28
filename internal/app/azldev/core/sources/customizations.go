// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/microsoft/azure-linux-dev-tools/internal/app/azldev/core/components"
	"github.com/microsoft/azure-linux-dev-tools/internal/projectconfig"
	"github.com/microsoft/azure-linux-dev-tools/internal/utils/fileperms"
	"github.com/spf13/afero"
)

// customizeToolEnvVar is the environment variable used to override the path to the
// rpm-spec-customize tool. When unset, [defaultCustomizeTool] is looked up on PATH.
const customizeToolEnvVar = "AZLDEV_RPM_SPEC_CUSTOMIZE"

// defaultCustomizeTool is the tool name looked up on PATH when [customizeToolEnvVar] is unset.
const defaultCustomizeTool = "rpm-spec-customize"

// applyCustomizations applies all declarative customizations for a component by shelling out
// to the rpm-spec-customize tool. Customizations are applied to the already-prepared spec
// (after overlays) in-place. If the component has no customizations, this is a no-op.
func (p *sourcePreparerImpl) applyCustomizations(
	ctx context.Context, component components.Component, absSpecPath string,
) error {
	customizations := component.GetConfig().Customizations
	if len(customizations) == 0 {
		return nil
	}

	event := p.eventListener.StartEvent("Applying customizations", "component", component.GetName())
	defer event.End()

	jsonBytes, err := buildCustomizationsDoc(customizations)
	if err != nil {
		return fmt.Errorf("failed to build customizations document for component %#q:\n%w",
			component.GetName(), err)
	}

	jsonPath, cleanup, err := p.writeCustomizationsFile(jsonBytes)
	if err != nil {
		return fmt.Errorf("failed to write customizations file for component %#q:\n%w",
			component.GetName(), err)
	}
	defer cleanup()

	tool := os.Getenv(customizeToolEnvVar)
	if tool == "" {
		tool = defaultCustomizeTool
	}

	args := []string{
		"--spec", absSpecPath,
		"--customizations", jsonPath,
		"--in-place",
		"--strict",
	}

	if err := p.runCustomizeTool(ctx, tool, args); err != nil {
		return fmt.Errorf("failed to apply customizations for component %#q:\n%w",
			component.GetName(), err)
	}

	return nil
}

// buildCustomizationsDoc serializes the component's customizations into the JSON document
// format expected by the rpm-spec-customize CLI.
func buildCustomizationsDoc(customizations []projectconfig.ComponentCustomization) ([]byte, error) {
	entries := make([]map[string]any, 0, len(customizations))

	for _, customization := range customizations {
		switch customization.Type {
		case projectconfig.ComponentCustomizeBuildOption:
			entries = append(entries, map[string]any{
				"kind":    "build-option",
				"option":  customization.Option,
				"enabled": *customization.Enabled,
			})
		case projectconfig.ComponentCustomizeTests:
			entries = append(entries, map[string]any{
				"kind":    "tests",
				"enabled": *customization.Enabled,
			})
		case projectconfig.ComponentCustomizeRemoveSubpackage:
			entries = append(entries, map[string]any{
				"kind":    "remove-subpackage",
				"package": customization.Package,
			})
		case projectconfig.ComponentCustomizeBuildSystem:
			options := make([]map[string]any, 0, len(customization.BuildOptions))
			for _, opt := range customization.BuildOptions {
				options = append(options, map[string]any{
					"phase": opt.Phase,
					"args":  opt.Args,
				})
			}

			entries = append(entries, map[string]any{
				"kind":       "build-system",
				"system":     customization.System,
				"options":    options,
				"drop_check": customization.DropCheck,
			})
		case projectconfig.ComponentCustomizeDependency:
			entry := map[string]any{
				"kind":         "dependency",
				"action":       customization.Action,
				"relationship": customization.Relationship,
				"name":         customization.Name,
			}
			if customization.Op != "" {
				entry["op"] = customization.Op
			}

			if customization.Version != "" {
				entry["version"] = customization.Version
			}

			entries = append(entries, entry)
		default:
			return nil, fmt.Errorf("unknown customization type %#q", customization.Type)
		}
	}

	doc := map[string]any{"customizations": entries}

	jsonBytes, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal customizations:\n%w", err)
	}

	return jsonBytes, nil
}

// writeCustomizationsFile writes the customizations JSON to a temporary file and returns its
// path along with a cleanup function that removes it. The file is written via the preparer's
// filesystem so that unit tests using an in-memory filesystem can inspect it.
func (p *sourcePreparerImpl) writeCustomizationsFile(jsonBytes []byte) (string, func(), error) {
	tmpFile, err := afero.TempFile(p.fs, "", "azldev-customizations-*.json")
	if err != nil {
		return "", func() {}, fmt.Errorf("failed to create temp file:\n%w", err)
	}

	jsonPath := tmpFile.Name()

	cleanup := func() {
		_ = p.fs.Remove(jsonPath)
	}

	if _, err := tmpFile.Write(jsonBytes); err != nil {
		_ = tmpFile.Close()

		cleanup()

		return "", func() {}, fmt.Errorf("failed to write temp file %#q:\n%w", jsonPath, err)
	}

	if err := tmpFile.Close(); err != nil {
		cleanup()

		return "", func() {}, fmt.Errorf("failed to close temp file %#q:\n%w", jsonPath, err)
	}

	// Ensure the file is readable by the external tool.
	_ = p.fs.Chmod(jsonPath, fileperms.PrivateFile)

	return jsonPath, cleanup, nil
}

// runCustomizeTool invokes the rpm-spec-customize tool. When a command factory is available it
// is used (so the invocation participates in the standard command plumbing); otherwise the
// command is run directly. In both cases stdout/stderr are wired to os.Stderr so the tool's
// per-operation log is visible.
func (p *sourcePreparerImpl) runCustomizeTool(ctx context.Context, tool string, args []string) error {
	execCmd := exec.CommandContext(ctx, tool, args...)
	execCmd.Stdout = os.Stderr
	execCmd.Stderr = os.Stderr

	if p.cmdFactory != nil {
		cmd, err := p.cmdFactory.Command(execCmd)
		if err != nil {
			return fmt.Errorf("failed to create command for %#q:\n%w", tool, err)
		}

		if err := cmd.Run(ctx); err != nil {
			return fmt.Errorf("%#q failed:\n%w", tool, err)
		}

		return nil
	}

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("%#q failed:\n%w", tool, err)
	}

	return nil
}
