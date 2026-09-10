## Why

Developers can build `jardec` into the repository-local `bin/` directory, but there is no Make target for installing the command for shell use. Providing one makes the standard Go installation workflow discoverable and repeatable.

## What Changes

- Add an `install` Make target that invokes Go's installation workflow for the `jardec` command.
- Add a Make-level behavioral test that verifies the target delegates to `go install` for the CLI package.

## Capabilities

### New Capabilities

- `cli-installation`: Installing the `jardec` CLI through the Makefile using Go's standard destination selection.

### Modified Capabilities

- None.

## Impact

- Affected files: `Makefile` and Makefile test coverage.
- No CLI flags, runtime behavior, or third-party dependencies change.
