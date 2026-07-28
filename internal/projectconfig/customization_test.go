// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package projectconfig_test

import (
	"testing"

	"github.com/microsoft/azure-linux-dev-tools/internal/projectconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentCustomization_Validate(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	testCases := []struct {
		name          string
		customization projectconfig.ComponentCustomization
		errorExpected bool
		errorContains string
	}{
		// customize-build-option tests
		{
			name: "customize-build-option valid enabled",
			customization: projectconfig.ComponentCustomization{
				Type:    projectconfig.ComponentCustomizeBuildOption,
				Option:  "mingw",
				Enabled: boolPtr(true),
			},
			errorExpected: false,
		},
		{
			name: "customize-build-option valid disabled",
			customization: projectconfig.ComponentCustomization{
				Type:    projectconfig.ComponentCustomizeBuildOption,
				Option:  "mingw",
				Enabled: boolPtr(false),
			},
			errorExpected: false,
		},
		{
			name: "customize-build-option missing option",
			customization: projectconfig.ComponentCustomization{
				Type:    projectconfig.ComponentCustomizeBuildOption,
				Enabled: boolPtr(true),
			},
			errorExpected: true,
			errorContains: "option",
		},
		{
			name: "customize-build-option missing enabled",
			customization: projectconfig.ComponentCustomization{
				Type:   projectconfig.ComponentCustomizeBuildOption,
				Option: "mingw",
			},
			errorExpected: true,
			errorContains: "enabled",
		},
		// customize-tests tests
		{
			name: "customize-tests valid enabled",
			customization: projectconfig.ComponentCustomization{
				Type:    projectconfig.ComponentCustomizeTests,
				Enabled: boolPtr(true),
			},
			errorExpected: false,
		},
		{
			name: "customize-tests valid disabled",
			customization: projectconfig.ComponentCustomization{
				Type:    projectconfig.ComponentCustomizeTests,
				Enabled: boolPtr(false),
			},
			errorExpected: false,
		},
		{
			name: "customize-tests missing enabled",
			customization: projectconfig.ComponentCustomization{
				Type: projectconfig.ComponentCustomizeTests,
			},
			errorExpected: true,
			errorContains: "enabled",
		},
		// customize-remove-subpackage tests
		{
			name: "customize-remove-subpackage valid",
			customization: projectconfig.ComponentCustomization{
				Type:    projectconfig.ComponentCustomizeRemoveSubpackage,
				Package: "doc",
			},
			errorExpected: false,
		},
		{
			name: "customize-remove-subpackage missing package",
			customization: projectconfig.ComponentCustomization{
				Type: projectconfig.ComponentCustomizeRemoveSubpackage,
			},
			errorExpected: true,
			errorContains: "package",
		},
		// customize-dependency tests
		{
			name: "customize-dependency valid set-constraint relax",
			customization: projectconfig.ComponentCustomization{
				Type:         projectconfig.ComponentCustomizeDependency,
				Relationship: "buildrequires",
				Action:       "set-constraint",
				Name:         "qt6-qtbase-private-devel",
				Op:           ">=",
			},
			errorExpected: false,
		},
		{
			name: "customize-dependency valid add",
			customization: projectconfig.ComponentCustomization{
				Type:         projectconfig.ComponentCustomizeDependency,
				Relationship: "requires",
				Action:       "add",
				Name:         "libfoo",
				Op:           ">=",
				Version:      "1.0",
			},
			errorExpected: false,
		},
		{
			name: "customize-dependency valid remove",
			customization: projectconfig.ComponentCustomization{
				Type:         projectconfig.ComponentCustomizeDependency,
				Relationship: "requires",
				Action:       "remove",
				Name:         "libfoo",
			},
			errorExpected: false,
		},
		{
			name: "customize-dependency missing relationship",
			customization: projectconfig.ComponentCustomization{
				Type:   projectconfig.ComponentCustomizeDependency,
				Action: "remove",
				Name:   "libfoo",
			},
			errorExpected: true,
			errorContains: "relationship",
		},
		{
			name: "customize-dependency missing name",
			customization: projectconfig.ComponentCustomization{
				Type:         projectconfig.ComponentCustomizeDependency,
				Relationship: "requires",
				Action:       "remove",
			},
			errorExpected: true,
			errorContains: "name",
		},
		{
			name: "customize-dependency invalid action",
			customization: projectconfig.ComponentCustomization{
				Type:         projectconfig.ComponentCustomizeDependency,
				Relationship: "requires",
				Action:       "frobnicate",
				Name:         "libfoo",
			},
			errorExpected: true,
			errorContains: "invalid action",
		},
		{
			name: "customize-dependency set-constraint without op or version",
			customization: projectconfig.ComponentCustomization{
				Type:         projectconfig.ComponentCustomizeDependency,
				Relationship: "buildrequires",
				Action:       "set-constraint",
				Name:         "libfoo",
			},
			errorExpected: true,
			errorContains: "set-constraint",
		},
		// unknown type
		{
			name: "unknown customization type",
			customization: projectconfig.ComponentCustomization{
				Type: "customize-bogus",
			},
			errorExpected: true,
			errorContains: "unknown customization type",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.customization.Validate()

			if testCase.errorExpected {
				require.Error(t, err)

				if testCase.errorContains != "" {
					assert.Contains(t, err.Error(), testCase.errorContains)
				}

				return
			}

			require.NoError(t, err)
		})
	}
}
