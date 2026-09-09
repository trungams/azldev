// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package sources

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/microsoft/azure-linux-dev-tools/internal/app/azldev/core/components"
	"github.com/microsoft/azure-linux-dev-tools/internal/global/opctx"
	"github.com/microsoft/azure-linux-dev-tools/internal/projectconfig"
	"github.com/microsoft/azure-linux-dev-tools/internal/rpm/spec"
	"github.com/microsoft/azure-linux-dev-tools/internal/utils/fileperms"
	"github.com/microsoft/azure-linux-dev-tools/internal/utils/fileutils"
)

const (
	rpmDevBumpspecIdentity = "Azure Linux Packaging Team <azurelinux@microsoft.com>"
	rpmDevBumpspecComment  = "- rebuilt"
	rpmDevBumpspecDate     = "Mon Jan 06 2025"
)

var autoreleasePattern = regexp.MustCompile(`%(\{[?]?autorelease($|[}\s])|autorelease($|\s))`)

// GetReleaseTagValue reads the Release tag value from the spec file at specPath.
func GetReleaseTagValue(fs opctx.FS, specPath string) (string, error) {
	specFile, err := fs.Open(specPath)
	if err != nil {
		return "", fmt.Errorf("failed to open spec %#q:\n%w", specPath, err)
	}
	defer specFile.Close()

	openedSpec, err := spec.OpenSpec(specFile)
	if err != nil {
		return "", fmt.Errorf("failed to parse spec %#q:\n%w", specPath, err)
	}

	var releaseValue string

	err = openedSpec.VisitTagsPackage("", func(tagLine *spec.TagLine, _ *spec.Context) error {
		if strings.EqualFold(tagLine.Tag, "Release") {
			releaseValue = tagLine.Value
		}

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to visit tags in spec %#q:\n%w", specPath, err)
	}

	if releaseValue == "" {
		return "", fmt.Errorf("release tag not found in spec %#q:\n%w", specPath, spec.ErrNoSuchTag)
	}

	return releaseValue, nil
}

// ReleaseUsesAutorelease reports whether the given Release tag value uses the %autorelease macro.
func ReleaseUsesAutorelease(releaseValue string) bool {
	return autoreleasePattern.MatchString(releaseValue)
}

//nolint:cyclop,funlen // The transaction boundaries preserve all four release modes and failure paths.
func (p *sourcePreparerImpl) tryBumpStaticRelease(
	ctx context.Context, component components.Component, sourcesDirPath string, changes []FingerprintChange,
) (err error) {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("release bump cancelled before execution:\n%w", err)
	}

	if len(changes) == 0 {
		return nil
	}

	calc := component.GetConfig().Release.Calculation
	switch calc {
	case projectconfig.ReleaseCalculationManual, projectconfig.ReleaseCalculationAutorelease:
		return nil
	case projectconfig.ReleaseCalculationAuto, projectconfig.ReleaseCalculationStatic:
		// Continue with automatic release handling.
	default:
		return fmt.Errorf("component %#q has unknown release calculation mode %#q", component.GetName(), calc)
	}

	specPath, err := p.resolveSpecPath(component, sourcesDirPath)
	if err != nil {
		return err
	}

	releaseValue, err := GetReleaseTagValue(p.fs, specPath)
	if err != nil {
		return fmt.Errorf("failed to read Release tag for component %#q:\n%w", component.GetName(), err)
	}

	if ReleaseUsesAutorelease(releaseValue) {
		if calc == projectconfig.ReleaseCalculationStatic {
			return fmt.Errorf("component %#q has 'release.calculation = \"static\"' but its Release tag uses %%autorelease; "+
				"set 'release.calculation = \"autorelease\"' instead", component.GetName())
		}

		return nil
	}

	if p.bumpspecCtx == nil || p.bumpspecScratchDir == "" {
		return fmt.Errorf("component %#q requires rpmdev-bumpspec release handling, but its host runner is not configured",
			component.GetName())
	}

	if err := fileutils.MkdirAll(p.bumpspecCtx.FS(), p.bumpspecScratchDir); err != nil {
		return fmt.Errorf("failed to create rpmdev-bumpspec scratch parent %#q:\n%w", p.bumpspecScratchDir, err)
	}

	originalBytes, err := fileutils.ReadFile(p.fs, specPath)
	if err != nil {
		return fmt.Errorf("failed to save original spec %#q before release bumps:\n%w", specPath, err)
	}

	defer func() {
		if err == nil {
			return
		}

		if restoreErr := fileutils.WriteFile(p.fs, specPath, originalBytes, fileperms.PublicFile); restoreErr != nil {
			err = fmt.Errorf("release bump failed and restoring original spec %#q also failed:\n%w",
				specPath, errors.Join(err, restoreErr))
		}
	}()

	for operation := range changes {
		homeDir, mkErr := fileutils.MkdirTemp(p.bumpspecCtx.FS(), p.bumpspecScratchDir, "azldev-bumpspec-")
		if mkErr != nil {
			return fmt.Errorf("failed to create rpmdev-bumpspec scratch directory:\n%w", mkErr)
		}

		request := RPMDevBumpspecRequest{
			SpecPath: specPath, Comment: rpmDevBumpspecComment, Identity: rpmDevBumpspecIdentity,
			Datestamp: rpmDevBumpspecDate, HomeDir: homeDir, Build: component.GetConfig().Build,
			TargetArch: p.bumpspecTargetArch,
		}
		runErr := RunRPMDevBumpspec(p.bumpspecCtx, request) //nolint:contextcheck

		cleanupErr := p.bumpspecCtx.FS().RemoveAll(homeDir)
		if runErr != nil {
			if cleanupErr != nil {
				return fmt.Errorf("failed release bump operation %d and scratch cleanup:\n%w",
					operation+1, errors.Join(runErr, cleanupErr))
			}

			return fmt.Errorf("failed to bump release for component %#q at operation %d:\n%w",
				component.GetName(), operation+1, runErr)
		}

		if cleanupErr != nil {
			return fmt.Errorf("failed to clean rpmdev-bumpspec scratch directory %#q:\n%w", homeDir, cleanupErr)
		}
	}

	slog.Info("Bumped release with rpmdev-bumpspec", "component", component.GetName(), "count", len(changes))

	return nil
}
