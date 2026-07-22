// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package projectconfig

import "fmt"

// ComponentCustomizationType is the type of a component customization.
type ComponentCustomizationType string

const (
	// ComponentCustomizeBuildOption enables or disables a named build option (e.g. a
	// %bcond_with/%bcond_without toggle) in the spec.
	ComponentCustomizeBuildOption ComponentCustomizationType = "customize-build-option"
	// ComponentCustomizeTests enables or disables the spec's test suite (its %check section).
	ComponentCustomizeTests ComponentCustomizationType = "customize-tests"
	// ComponentCustomizeRemoveSubpackage removes a named sub-package from the spec.
	ComponentCustomizeRemoveSubpackage ComponentCustomizationType = "customize-remove-subpackage"
)

// ComponentCustomization represents a declarative spec customization that is applied after
// overlays by shelling out to the rpm-spec-customize tool. Unlike overlays (which perform
// structural spec edits in-process), customizations express higher-level intent such as
// toggling a build option, enabling/disabling tests, or removing a sub-package.
type ComponentCustomization struct {
	// The type of customization to apply.
	Type ComponentCustomizationType `toml:"type" json:"type" validate:"required" jsonschema:"enum=customize-build-option,enum=customize-tests,enum=customize-remove-subpackage,title=Customization type,description=The type of customization to apply"`
	// Human readable description of customization; primarily present to document the need for the change.
	Description string `toml:"description,omitempty" json:"description,omitempty" jsonschema:"title=Description,description=Human readable description of customization" fingerprint:"-"`

	// For customize-build-option, the name of the build option to enable or disable (e.g. "mingw").
	Option string `toml:"option,omitempty" json:"option,omitempty" jsonschema:"title=Build option,description=For customize-build-option, the name of the build option to enable or disable"`
	// For customize-remove-subpackage, the name of the sub-package to remove.
	Package string `toml:"package,omitempty" json:"package,omitempty" jsonschema:"title=Package name,description=For customize-remove-subpackage, the name of the sub-package to remove"`
	// For customize-build-option and customize-tests, whether the target is enabled. A pointer so
	// that an absent value is distinguishable from an explicit false.
	Enabled *bool `toml:"enabled,omitempty" json:"enabled,omitempty" jsonschema:"title=Enabled,description=For customize-build-option and customize-tests, whether the target is enabled"`
}

// Validate checks that required fields are set based on the customization type. This catches
// configuration errors at load time rather than at apply time.
func (c *ComponentCustomization) Validate() error {
	desc := c.Description
	if desc == "" {
		desc = "(no description)"
	}

	missingField := func(fieldName string) error {
		return fmt.Errorf("customization type %#q requires %#q field: %s", c.Type, fieldName, desc)
	}

	switch c.Type {
	case ComponentCustomizeBuildOption:
		if c.Option == "" {
			return missingField("option")
		}

		if c.Enabled == nil {
			return missingField("enabled")
		}
	case ComponentCustomizeTests:
		if c.Enabled == nil {
			return missingField("enabled")
		}
	case ComponentCustomizeRemoveSubpackage:
		if c.Package == "" {
			return missingField("package")
		}
	default:
		return fmt.Errorf("unknown customization type %#q: %#q", c.Type, desc)
	}

	return nil
}
