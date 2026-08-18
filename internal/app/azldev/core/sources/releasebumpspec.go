// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package sources

// PROOF OF CONCEPT — not production code.
//
// Evaluates replacing azldev's declarative release counters with Fedora's
// 'rpmdev-bumpspec'. rpmdevtools is GPL-2.0 and azldev is MIT, so the script
// cannot be vendored; it is invoked as an external tool.

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/microsoft/azure-linux-dev-tools/internal/app/azldev/core/components"
)

const (
	// bumpspecBinEnv overrides the rpmdev-bumpspec executable path.
	bumpspecBinEnv = "AZLDEV_BUMPSPEC_BIN"

	// bumpspecPoCEnv routes every component that would use a release counter
	// through rpmdev-bumpspec instead, so a whole tree can be rendered both
	// ways without editing per-component config.
	bumpspecPoCEnv = "AZLDEV_POC_BUMPSPEC"

	// rpmdev-bumpspec stamps today's date and the local packager identity into
	// the changelog entry it writes. Both must be pinned or every re-render
	// produces a different file and the rendered-spec check can never pass.
	bumpspecUserString = "Azure Linux Packaging Team <azurelinux@microsoft.com>"
	bumpspecDatestamp  = "Mon Jan 06 2025"
	bumpspecComment    = "- rebuilt"
)

// errBumpspecUnavailable is returned when the external tool cannot be located.
var errBumpspecUnavailable = errors.New("rpmdev-bumpspec not found")

// bumpspecPoCEnabled reports whether the experiment switch is set.
func bumpspecPoCEnabled() bool {
	return os.Getenv(bumpspecPoCEnv) == "1"
}

// bumpspecBinary resolves the rpmdev-bumpspec executable.
func bumpspecBinary() string {
	if override := os.Getenv(bumpspecBinEnv); override != "" {
		return override
	}

	return "rpmdev-bumpspec"
}

// bumpReleaseWithBumpspec bumps a component's Release tag by invoking
// rpmdev-bumpspec once per synthetic commit. The tool increments by one per
// run and appends a changelog entry each time, so N runs yield N entries.
func (p *sourcePreparerImpl) bumpReleaseWithBumpspec(
	component components.Component,
	sourcesDirPath string,
	commitCount int,
) error {
	specPath, err := p.resolveSpecPath(component, sourcesDirPath)
	if err != nil {
		return err
	}

	releaseValue, err := GetReleaseTagValue(p.fs, specPath)
	if err != nil {
		return fmt.Errorf("failed to read Release tag for component %#q:\n%w",
			component.GetName(), err)
	}

	// rpmdev-bumpspec rewrites '%autorelease' to '%autorelease.1', which breaks
	// rpmautospec expansion downstream.
	if ReleaseUsesAutorelease(releaseValue) {
		slog.Debug("Spec uses %%autorelease; skipping bumpspec",
			"component", component.GetName())

		return nil
	}

	binary := bumpspecBinary()

	resolved, err := exec.LookPath(binary)
	if err != nil {
		return fmt.Errorf("%w: %#q (set %s):\n%w",
			errBumpspecUnavailable, binary, bumpspecBinEnv, err)
	}

	absSpec, err := filepath.Abs(specPath)
	if err != nil {
		return fmt.Errorf("resolving spec path %#q:\n%w", specPath, err)
	}

	for range commitCount {
		if err := runBumpspec(resolved, absSpec); err != nil {
			return fmt.Errorf("component %#q:\n%w", component.GetName(), err)
		}
	}

	newValue, err := GetReleaseTagValue(p.fs, specPath)
	if err != nil {
		return fmt.Errorf("failed to re-read Release tag for component %#q:\n%w",
			component.GetName(), err)
	}

	slog.Info("Bumped release with rpmdev-bumpspec",
		"component", component.GetName(),
		"oldRelease", releaseValue,
		"newRelease", newValue,
		"commitCount", commitCount)

	return nil
}

// runBumpspec performs a single bump. rpmdev-bumpspec calls rpmdev-packager
// unconditionally at startup, so that sibling script must also be on PATH even
// though '-u' supplies the identity.
func runBumpspec(binary, specPath string) error {
	cmd := exec.Command(binary,
		"-c", bumpspecComment,
		"-u", bumpspecUserString,
		"-d", bumpspecDatestamp,
		specPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rpmdev-bumpspec failed on %#q: %s:\n%w",
			specPath, strings.TrimSpace(string(output)), err)
	}

	// The tool reports "No release value matched" on stdout and still exits 0
	// for some spec shapes, so the output has to be inspected explicitly.
	if text := string(output); strings.Contains(text, "No release value matched") {
		return fmt.Errorf("rpmdev-bumpspec could not find a release value in %#q: %s",
			specPath, strings.TrimSpace(text))
	}

	return nil
}
