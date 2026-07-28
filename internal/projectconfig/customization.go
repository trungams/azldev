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
	// ComponentCustomizeBuildSystem onboards the spec to a declarative RPM build system
	// (rpm >= 4.20) by setting the BuildSystem tag and removing the redundant build phases.
	ComponentCustomizeBuildSystem ComponentCustomizationType = "customize-build-system"
	// ComponentCustomizeDependency adds, removes, or re-constrains a dependency relationship
	// (Requires/BuildRequires/Conflicts/Recommends) on the main package.
	ComponentCustomizeDependency ComponentCustomizationType = "customize-dependency"
)

// CustomizationBuildOption is a single BuildOption(<phase>) argument passed to a declarative
// build system.
type CustomizationBuildOption struct {
	// The build phase the option applies to (e.g. "conf", "build"). Empty defaults to "conf".
	Phase string `toml:"phase,omitempty" json:"phase,omitempty" jsonschema:"title=Phase,description=The build phase the option applies to (defaults to conf)"`
	// The argument string to pass to that phase.
	Args string `toml:"args" json:"args" validate:"required" jsonschema:"title=Args,description=The argument string to pass to the build phase"`
}

// ComponentCustomization represents a declarative spec customization that is applied after
// overlays by shelling out to the rpm-spec-customize tool. Unlike overlays (which perform
// structural spec edits in-process), customizations express higher-level intent such as
// toggling a build option, enabling/disabling tests, or removing a sub-package.
type ComponentCustomization struct {
	// The type of customization to apply.
	Type ComponentCustomizationType `toml:"type" json:"type" validate:"required" jsonschema:"enum=customize-build-option,enum=customize-tests,enum=customize-remove-subpackage,enum=customize-build-system,enum=customize-dependency,title=Customization type,description=The type of customization to apply"`
	// Human readable description of customization; primarily present to document the need for the change.
	Description string `toml:"description,omitempty" json:"description,omitempty" jsonschema:"title=Description,description=Human readable description of customization" fingerprint:"-"`

	// For customize-build-option, the name of the build option to enable or disable (e.g. "mingw").
	Option string `toml:"option,omitempty" json:"option,omitempty" jsonschema:"title=Build option,description=For customize-build-option, the name of the build option to enable or disable"`
	// For customize-remove-subpackage, the name of the sub-package to remove.
	Package string `toml:"package,omitempty" json:"package,omitempty" jsonschema:"title=Package name,description=For customize-remove-subpackage, the name of the sub-package to remove"`
	// For customize-build-option and customize-tests, whether the target is enabled. A pointer so
	// that an absent value is distinguishable from an explicit false.
	Enabled *bool `toml:"enabled,omitempty" json:"enabled,omitempty" jsonschema:"title=Enabled,description=For customize-build-option and customize-tests, whether the target is enabled"`
	// For customize-build-system, the declarative build system name to onboard to (e.g. "cmake").
	System string `toml:"system,omitempty" json:"system,omitempty" jsonschema:"title=Build system,description=For customize-build-system, the declarative build system to onboard to (e.g. cmake)"`
	// For customize-build-system, optional BuildOption(<phase>) arguments.
	BuildOptions []CustomizationBuildOption `toml:"build-options,omitempty" json:"buildOptions,omitempty" jsonschema:"title=Build options,description=For customize-build-system, optional BuildOption(phase) arguments"`
	// For customize-build-system, whether to also drop the explicit %check section.
	DropCheck bool `toml:"drop-check,omitempty" json:"dropCheck,omitempty" jsonschema:"title=Drop check,description=For customize-build-system, whether to also remove the explicit %check section"`
	// For customize-dependency, the relationship to edit: requires, buildrequires, conflicts, or recommends.
	Relationship string `toml:"relationship,omitempty" json:"relationship,omitempty" jsonschema:"enum=requires,enum=buildrequires,enum=conflicts,enum=recommends,title=Relationship,description=For customize-dependency, the dependency relationship to edit"`
	// For customize-dependency, the action to take: add, remove, or set-constraint.
	Action string `toml:"action,omitempty" json:"action,omitempty" jsonschema:"enum=add,enum=remove,enum=set-constraint,title=Action,description=For customize-dependency, whether to add, remove, or re-constrain the dependency"`
	// For customize-dependency, the dependency (package or capability) name to match/add.
	Name string `toml:"name,omitempty" json:"name,omitempty" jsonschema:"title=Dependency name,description=For customize-dependency, the dependency name to match or add"`
	// For customize-dependency, the version comparison operator (e.g. ">=", "=").
	Op string `toml:"op,omitempty" json:"op,omitempty" jsonschema:"title=Operator,description=For customize-dependency, the version comparison operator (e.g. >=)"`
	// For customize-dependency, the version/EVR string for the constraint. Omitting it on
	// set-constraint keeps the existing version (relax in place).
	Version string `toml:"version,omitempty" json:"version,omitempty" jsonschema:"title=Version,description=For customize-dependency, the version for the constraint; omit on set-constraint to keep the existing one"`
}

// Validate checks that required fields are set based on the customization type. This catches
// configuration errors at load time rather than at apply time.
//
//nolint:cyclop // complexity is inherent to the number of customization types.
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
	case ComponentCustomizeBuildSystem:
		if c.System == "" {
			return missingField("system")
		}
	case ComponentCustomizeDependency:
		if c.Relationship == "" {
			return missingField("relationship")
		}

		if c.Name == "" {
			return missingField("name")
		}

		switch c.Action {
		case "add", "remove", "set-constraint":
		case "":
			return missingField("action")
		default:
			return fmt.Errorf(
				"customization type %#q has invalid action %#q (want add/remove/set-constraint): %s",
				c.Type, c.Action, desc)
		}

		if c.Action == "set-constraint" && c.Op == "" && c.Version == "" {
			return fmt.Errorf(
				"customization type %#q action \"set-constraint\" requires %#q or %#q: %s",
				c.Type, "op", "version", desc)
		}
	default:
		return fmt.Errorf("unknown customization type %#q: %#q", c.Type, desc)
	}

	return nil
}
