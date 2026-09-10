## Context

The Makefile currently supports testing, repository-local builds, help output,
and cleanup. It has no command that delegates installation to the Go toolchain.
The existing README development section lists the available Make targets.

## Goals / Non-Goals

**Goals:**

- Add one explicit Make entry point for installing the CLI.
- Preserve Go's standard `GOBIN` and default destination selection.
- Verify the Make target's observable command invocation without writing to a real installation directory.

**Non-Goals:**

- Do not add custom copying, destination overrides, version stamping, or uninstall behavior.
- Do not alter the CLI build package or runtime behavior.

## Decisions

- Define `install` as a phony Make target that runs `go install ./cmd/jardec`. This delegates package resolution and output placement to the Go toolchain, so a configured `GOBIN` is honored. A custom `cp`-based target was rejected because it would duplicate Go's destination and executable handling.
- Add `install` to the Makefile's phony target declaration. This prevents a same-named local file from suppressing the installation command.
- Exercise the target with a test-local fake `go` executable placed first on `PATH`, then assert the captured argument sequence. This verifies actual Make expansion and command execution while avoiding writes to `GOBIN`. A source-text assertion was rejected because it would not demonstrate target behavior.

## Risks / Trade-offs

- [A user expects installation even when Go cannot resolve dependencies] → Go's error is intentionally propagated so the target does not hide build failures.
- [A user has no `GOBIN` configured] → Go's default installation destination remains supported and documented as the fallback behavior.

## Migration Plan

No migration is required. The new target is additive and can be used immediately after update.
