# Customizations

Customizations are declarative, **intent-level** modifications to an RPM spec — such as toggling a build option, disabling the test suite, dropping a sub-package, onboarding a package to a declarative build system, or relaxing a dependency constraint. Unlike [overlays](overlays.md) (which perform structural spec edits), customizations express *what* you want and let the engine pick the right structural change.

Customizations are defined within a component's configuration in your TOML config file and are applied **after** overlays during source preparation. azldev applies them by shelling out to the `rpm-spec-customize` tool, which edits the spec through a typed AST and refuses to write a result that no longer parses.

> **Note:** Customizations are applied in the order they appear, after all overlays for the component. Because they run on the post-overlay spec, an overlay and a customization can cooperate on the same component.

## Customization Types

| Type | Description | Required Fields |
|------|-------------|-----------------|
| `build-option` | Enables or disables a named build option so it defaults on/off in the rendered spec. Normalizes the `%bcond`, `%bcond_with`, `%bcond_without` forms and a hard-coded `%global with_<option> 0\|1`. **Does not fabricate a toggle that is not present.** | `option`, `enabled` |
| `tests` | Enables or disables the test suite. Disabling removes the `%check` section; enabling is a reported no-op (a test suite cannot be synthesized). | `enabled` |
| `remove-subpackage` | Removes every section belonging to a sub-package (`%package`, `%description`, `%files`, scriptlets). See [Build considerations](#build-considerations). | `package` |
| `build-system` | Onboards the main package to a declarative RPM `BuildSystem:` (rpm ≥ 4.20): sets the tag, appends any `BuildOption(<phase>)` args, and removes the now-redundant `%prep`/`%generate_buildrequires`/`%build`/`%install` (and optionally `%check`). See [Build considerations](#build-considerations). | `system` |
| `dependency` | Adds, removes, or re-constrains a dependency relationship (`Requires`, `BuildRequires`, `Conflicts`, `Recommends`). | `relationship`, `name`, `action` |

## Field Reference

| Field | TOML Key | Description | Used By |
|-------|----------|-------------|---------|
| Type | `type` | **Required.** The customization type to apply | All customizations |
| Description | `description` | Human-readable explanation documenting the need for the change; helps identify customizations in logs | All (optional) |
| Option | `option` | The build option name (e.g., `mingw`) | `build-option` |
| Enabled | `enabled` | Whether the target should be on (`true`) or off (`false`) | `build-option`, `tests` |
| Package | `package` | The sub-package name to remove | `remove-subpackage` |
| System | `system` | The declarative build system to onboard to (e.g., `cmake`, `pyproject`) | `build-system` |
| Build options | `build-options` | Array of `{ phase, args }` mapped to `BuildOption(<phase>): <args>` tags. `phase` defaults to `conf` when empty. | `build-system` (optional) |
| Drop check | `drop-check` | When `true`, also removes the explicit `%check` section during onboarding | `build-system` (optional) |
| Relationship | `relationship` | The dependency relationship: `requires`, `buildrequires`, `conflicts`, or `recommends` | `dependency` |
| Action | `action` | `add`, `remove`, or `set-constraint` | `dependency` |
| Name | `name` | The dependency (package or capability) name to match or add | `dependency` |
| Op | `op` | Version comparison operator (`>=`, `=`, `>`, `<`, `<=`, `!=`) | `dependency` (optional; required with `version` when creating a constraint) |
| Version | `version` | Version/EVR string for the constraint. Omit on `set-constraint` to keep the existing version (relax/tighten in place). | `dependency` (optional) |

## Examples

### Disabling a Build Option

Disable the `mingw` cross-build. This normalizes both `%bcond*` toggles and a hard-coded `%global with_mingw 1`, so it replaces the need for a `spec-search-replace` overlay:

```toml
[[components.dtc.customizations]]
type = "build-option"
description = "Disable mingw sub-packages — Azure Linux does not ship mingw toolchains"
option = "mingw"
enabled = false
```

### Disabling the Test Suite

Remove the `%check` section from the rendered spec:

```toml
[[components.jq.customizations]]
type = "tests"
description = "Disable the jq test suite — removes %check from the rendered spec"
enabled = false
```

### Removing a Sub-package

Drop a sub-package you do not want to ship:

```toml
[[components.asio.customizations]]
type = "remove-subpackage"
description = "Do not ship the asio-doc sub-package"
package = "doc"
```

The `package` value matches the sub-package as declared in the spec: use the relative name for `%package doc` (`doc`) or the full name for `%package -n libfdt-static` (`libfdt-static`).

### Onboarding to a Build System

Convert an explicit-phase spec to a declarative build system. For `pyproject`, pass the import name to the install phase so `%pyproject_save_files` (and the retained `%files -f %{pyproject_files}`) works:

```toml
[[components.ephemeral-port-reserve.customizations]]
type = "build-system"
description = "Onboard to the declarative pyproject BuildSystem (rpm >= 4.20)"
system = "pyproject"

[[components.ephemeral-port-reserve.customizations.build-options]]
phase = "install"
args = "ephemeral_port_reserve"
```

For `cmake`:

```toml
[[components.libaribcaption.customizations]]
type = "build-system"
description = "Onboard to the declarative cmake BuildSystem (rpm >= 4.20)"
system = "cmake"
```

### Relaxing a Dependency Constraint

Relax an exact pin to `>=` in place — only the operator changes, the version (here a macro) is preserved:

```toml
[[components.qt6-qthttpserver.customizations]]
type = "dependency"
description = "Relax qt6-qtbase-private-devel from '= %{version}' to '>= %{version}'"
action = "set-constraint"
relationship = "buildrequires"
name = "qt6-qtbase-private-devel"
op = ">="
```

Add or remove a dependency:

```toml
[[components.mypackage.customizations]]
type = "dependency"
action = "add"
relationship = "requires"
name = "libfoo"
op = ">="
version = "1.0"

[[components.mypackage.customizations]]
type = "dependency"
action = "remove"
relationship = "requires"
name = "obsolete-dep"
```

## Build considerations

Some customizations change the rendered spec cleanly but have build-time prerequisites:

- **`remove-subpackage`** only removes the spec *sections*. If `%install` still installs the sub-package's files, RPM fails the build with *"Installed (but unpackaged) file(s) found"*. Removal is safe when the files are not produced by `%install` — for example `asio-doc`, whose `%files` is `%doc doc/*` (source-tree docs copied at packaging time). For sub-packages that own installed paths, also remove those files (e.g., with a `file-search-replace`/`spec-search-replace` overlay or by disabling the build option that produces them).
- **`build-system`** requires the target build system's `%buildsystem_<name>_*` macros to be present at **SRPM/parse time** (before `BuildRequires` install). These ship in the system's `*-srpm-macros` package. `pyproject`, `tree-sitter`, and `java` are pulled into the Azure Linux buildroot by `azurelinux-rpm-config`; `cmake` is not by default, so onboarding to `cmake` additionally requires `cmake-srpm-macros` in the buildroot (for example via the mock `chroot_setup_cmd`). A `BuildSystem:` that the buildroot does not know fails with *"Unknown buildsystem: `<name>`"*.

## Validation

Customization configurations are validated when the config file is loaded. Validation checks:

- The `type` is one of the known customization types
- Required fields are present for each type (e.g., `option`+`enabled` for `build-option`; `system` for `build-system`; `relationship`+`name`+`action` for `dependency`)
- `dependency` `action` is one of `add`, `remove`, `set-constraint`, and `set-constraint` provides an `op` and/or `version`

> **Tip:** Always provide a `description` so the per-customization log line (`[applied] ...`) is easy to identify.

## Tool resolution

The `rpm-spec-customize` binary is located via the `AZLDEV_RPM_SPEC_CUSTOMIZE` environment variable, falling back to `rpm-spec-customize` on `PATH`.

## Related Resources

- [Components](components.md) — customizations are defined within component configuration
- [Overlays](overlays.md) — structural spec/file edits, applied before customizations
- [Config File Structure](config-file.md) — top-level config file layout
- [JSON Schema](../../../../schemas/azldev.schema.json) — use with editors that support JSON Schema for TOML to get validation and auto-completion
