// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package sources

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/microsoft/azure-linux-dev-tools/internal/app/azldev/core/components/components_testutils"
	"github.com/microsoft/azure-linux-dev-tools/internal/global/testctx"
	"github.com/microsoft/azure-linux-dev-tools/internal/projectconfig"
	"github.com/microsoft/azure-linux-dev-tools/internal/utils/fileperms"
	"github.com/microsoft/azure-linux-dev-tools/internal/utils/fileutils"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const testSourcesDir = "/sources"

func releaseTestPreparer(t *testing.T) (*sourcePreparerImpl, *testctx.TestCtx) {
	t.Helper()

	ctx := newBumpspecCtx()

	return &sourcePreparerImpl{fs: ctx.FS(), bumpspecCtx: ctx, bumpspecScratchDir: "/work/missing"}, ctx
}

func writeTestSpec(t *testing.T, memFS afero.Fs, name, content string) string {
	t.Helper()

	specPath := filepath.Join(testSourcesDir, name, name+".spec")
	require.NoError(t, fileutils.MkdirAll(memFS, filepath.Dir(specPath)))
	require.NoError(t, fileutils.WriteFile(memFS, specPath, []byte(content), fileperms.PublicFile))

	return specPath
}

func mockReleaseComponent(
	ctrl *gomock.Controller, name string, calculation projectconfig.ReleaseCalculation,
) *components_testutils.MockComponent {
	comp := components_testutils.NewMockComponent(ctrl)
	comp.EXPECT().GetName().AnyTimes().Return(name)
	comp.EXPECT().GetConfig().AnyTimes().Return(&projectconfig.ComponentConfig{
		Release: projectconfig.ReleaseConfig{Calculation: calculation},
		Build:   projectconfig.ComponentBuildConfig{Defines: map[string]string{"fixture": "1"}},
	})

	return comp
}

func mockComponent(
	ctrl *gomock.Controller, name string, config *projectconfig.ComponentConfig,
) *components_testutils.MockComponent {
	comp := components_testutils.NewMockComponent(ctrl)
	comp.EXPECT().GetName().AnyTimes().Return(name)
	comp.EXPECT().GetConfig().AnyTimes().Return(config)

	return comp
}

func testChanges() []FingerprintChange {
	return []FingerprintChange{
		{CommitMetadata: CommitMetadata{Hash: "first", Timestamp: 1, Message: "different"}},
		{CommitMetadata: CommitMetadata{Hash: "second", Timestamp: 2, Message: "metadata"}},
	}
}

func TestTryBumpStaticRelease_UsesEVRTransactionsAndFixedMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	preparer, ctx := releaseTestPreparer(t)
	original := "Name: test-pkg\nVersion: 1.0\nRelease: 4%{?dist}\n%changelog\n"
	specPath := writeTestSpec(t, ctx.FS(), "test-pkg", original)
	comp := mockReleaseComponent(ctrl, "test-pkg", projectconfig.ReleaseCalculationAuto)
	queries := 0
	bumps := 0
	ctx.CmdFactory.RunHandler = func(cmd *exec.Cmd) error {
		switch {
		case isEVRQuery(cmd):
			queries++
			_, _ = fmt.Fprint(cmd.Stdout, evrOutput("test-pkg", "0", strconv.Itoa(3+(queries+1)/2)))
		case cmd.Path == RPMDevBumpspecBinary:
			bumps++

			assert.Equal(t, "- rebuilt", cmd.Args[2])
			assert.Equal(t, "Mon Jan 06 2025", cmd.Args[6])
			content := stringMustRead(t, ctx, specPath)
			content = strings.Replace(content, fmt.Sprintf("Release: %d%%{?dist}", 3+bumps),
				fmt.Sprintf("Release: %d%%{?dist}", 4+bumps), 1)
			require.NoError(t, fileutils.WriteFile(ctx.FS(), specPath, []byte(content), fileperms.PublicFile))
		case isReleaseComparison(cmd):
			_, _ = fmt.Fprintln(cmd.Stdout, "-1")
		default:
			return fmt.Errorf("unexpected command: %#v", cmd.Args)
		}

		return nil
	}

	require.NoError(t, preparer.tryBumpStaticRelease(context.Background(), comp,
		filepath.Join(testSourcesDir, "test-pkg"), testChanges()))
	assert.Equal(t, 2, bumps)
	assert.Equal(t, 4, queries)
	assert.Contains(t, stringMustRead(t, ctx, specPath), "Release: 6%{?dist}")
	entries, err := afero.ReadDir(ctx.FS(), "/work/missing")
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestTryBumpStaticRelease_RollsBackWholeSequence(t *testing.T) {
	ctrl := gomock.NewController(t)
	preparer, ctx := releaseTestPreparer(t)
	original := "Name: osbs-client\nVersion: 1.0\n%if 0\n%global release 23\n%else\n" +
		"%global release 6\n%endif\nRelease: %{release}\n"
	specPath := writeTestSpec(t, ctx.FS(), "osbs-client", original)
	comp := mockReleaseComponent(ctrl, "osbs-client", projectconfig.ReleaseCalculationAuto)
	queries := 0
	bumps := 0
	ctx.CmdFactory.RunHandler = func(cmd *exec.Cmd) error {
		switch {
		case isEVRQuery(cmd):
			queries++
			_, _ = fmt.Fprint(cmd.Stdout, evrOutput("osbs-client", "0", "6"))
		case cmd.Path == RPMDevBumpspecBinary:
			bumps++

			content := stringMustRead(t, ctx, specPath)
			if bumps == 1 {
				return fileutils.WriteFile(ctx.FS(), specPath, []byte(strings.Replace(content, "release 23", "release 24", 1)),
					fileperms.PublicFile)
			}

			return fileutils.WriteFile(ctx.FS(), specPath, []byte(strings.Replace(content, "release 6", "release 7", 1)),
				fileperms.PublicFile)
		case isReleaseComparison(cmd):
			_, _ = fmt.Fprintln(cmd.Stdout, "0")
		}

		return nil
	}

	err := preparer.tryBumpStaticRelease(
		context.Background(), comp, filepath.Join(testSourcesDir, "osbs-client"), testChanges())
	require.ErrorIs(t, err, ErrRPMDevBumpspecNoMutation)
	assert.Equal(t, original, stringMustRead(t, ctx, specPath), "inactive release mutation must be rolled back")
	assert.Equal(t, 1, bumps)
}

func TestTryBumpStaticRelease_RollsBackAfterKthFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	preparer, ctx := releaseTestPreparer(t)
	original := "Name: test-pkg\nVersion: 1.0\nRelease: 4\n"
	specPath := writeTestSpec(t, ctx.FS(), "test-pkg", original)
	comp := mockReleaseComponent(ctrl, "test-pkg", projectconfig.ReleaseCalculationAuto)
	queries := 0
	bumps := 0
	ctx.CmdFactory.RunHandler = func(cmd *exec.Cmd) error {
		switch {
		case isEVRQuery(cmd):
			queries++

			release := "4"
			if queries > 1 {
				release = "5"
			}

			_, _ = fmt.Fprint(cmd.Stdout, evrOutput("test-pkg", "0", release))
		case cmd.Path == RPMDevBumpspecBinary:
			bumps++
			if bumps == 2 {
				return errors.New("second operation failed")
			}

			return fileutils.WriteFile(ctx.FS(), specPath,
				[]byte("Name: test-pkg\nVersion: 1.0\nRelease: 5\n"), fileperms.PublicFile)
		case isReleaseComparison(cmd):
			_, _ = fmt.Fprintln(cmd.Stdout, "-1")
		}

		return nil
	}

	err := preparer.tryBumpStaticRelease(
		context.Background(), comp, filepath.Join(testSourcesDir, "test-pkg"), testChanges())
	require.Error(t, err)
	assert.Equal(t, 2, bumps)
	assert.Equal(t, original, stringMustRead(t, ctx, specPath))
}

func TestTryBumpStaticRelease_ModesAndZeroChanges(t *testing.T) {
	for _, testCase := range []struct {
		name, release string
		calculation   projectconfig.ReleaseCalculation
		changes       []FingerprintChange
		wantErr       bool
	}{
		{"zero", "4", projectconfig.ReleaseCalculationAuto, nil, false},
		{"manual", "4", projectconfig.ReleaseCalculationManual, testChanges(), false},
		{"explicit autorelease", "4", projectconfig.ReleaseCalculationAutorelease, testChanges(), false},
		{"auto autorelease", "%{autorelease -e asan}", projectconfig.ReleaseCalculationAuto, testChanges(), false},
		{"static autorelease mismatch", "%autorelease", projectconfig.ReleaseCalculationStatic, testChanges(), true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			preparer, ctx := releaseTestPreparer(t)
			comp := mockReleaseComponent(ctrl, "test-pkg", testCase.calculation)
			specPath := writeTestSpec(t, ctx.FS(), "test-pkg",
				"Name: test-pkg\nVersion: 1.0\nRelease: "+testCase.release+"\n")

			err := preparer.tryBumpStaticRelease(
				context.Background(), comp, filepath.Join(testSourcesDir, "test-pkg"), testCase.changes)
			if testCase.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Contains(t, stringMustRead(t, ctx, specPath), "Release: "+testCase.release)
			}
		})
	}
}
