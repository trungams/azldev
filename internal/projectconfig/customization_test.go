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
		// build-option tests
		{
			name: "build-option valid enabled",
			customization: projectconfig.ComponentCustomization{
				Type:    projectconfig.ComponentCustomizeBuildOption,
				Option:  "mingw",
				Enabled: boolPtr(true),
			},
			errorExpected: false,
		},
		{
			name: "build-option valid disabled",
			customization: projectconfig.ComponentCustomization{
				Type:    projectconfig.ComponentCustomizeBuildOption,
				Option:  "mingw",
				Enabled: boolPtr(false),
			},
			errorExpected: false,
		},
		{
			name: "build-option missing option",
			customization: projectconfig.ComponentCustomization{
				Type:    projectconfig.ComponentCustomizeBuildOption,
				Enabled: boolPtr(true),
			},
			errorExpected: true,
			errorContains: "option",
		},
		{
			name: "build-option missing enabled",
			customization: projectconfig.ComponentCustomization{
				Type:   projectconfig.ComponentCustomizeBuildOption,
				Option: "mingw",
			},
			errorExpected: true,
			errorContains: "enabled",
		},
		// tests
		{
			name: "tests valid enabled",
			customization: projectconfig.ComponentCustomization{
				Type:    projectconfig.ComponentCustomizeTests,
				Enabled: boolPtr(true),
			},
			errorExpected: false,
		},
		{
			name: "tests valid disabled",
			customization: projectconfig.ComponentCustomization{
				Type:    projectconfig.ComponentCustomizeTests,
				Enabled: boolPtr(false),
			},
			errorExpected: false,
		},
		{
			name: "tests missing enabled",
			customization: projectconfig.ComponentCustomization{
				Type: projectconfig.ComponentCustomizeTests,
			},
			errorExpected: true,
			errorContains: "enabled",
		},
		// remove-subpackage tests
		{
			name: "remove-subpackage valid",
			customization: projectconfig.ComponentCustomization{
				Type:    projectconfig.ComponentCustomizeRemoveSubpackage,
				Package: "doc",
			},
			errorExpected: false,
		},
		{
			name: "remove-subpackage missing package",
			customization: projectconfig.ComponentCustomization{
				Type: projectconfig.ComponentCustomizeRemoveSubpackage,
			},
			errorExpected: true,
			errorContains: "package",
		},
		// dependency tests
		{
			name: "dependency valid set-constraint relax",
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
			name: "dependency valid add",
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
			name: "dependency valid remove",
			customization: projectconfig.ComponentCustomization{
				Type:         projectconfig.ComponentCustomizeDependency,
				Relationship: "requires",
				Action:       "remove",
				Name:         "libfoo",
			},
			errorExpected: false,
		},
		{
			name: "dependency missing relationship",
			customization: projectconfig.ComponentCustomization{
				Type:   projectconfig.ComponentCustomizeDependency,
				Action: "remove",
				Name:   "libfoo",
			},
			errorExpected: true,
			errorContains: "relationship",
		},
		{
			name: "dependency missing name",
			customization: projectconfig.ComponentCustomization{
				Type:         projectconfig.ComponentCustomizeDependency,
				Relationship: "requires",
				Action:       "remove",
			},
			errorExpected: true,
			errorContains: "name",
		},
		{
			name: "dependency invalid action",
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
			name: "dependency set-constraint without op or version",
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
				Type: "bogus",
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
