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
