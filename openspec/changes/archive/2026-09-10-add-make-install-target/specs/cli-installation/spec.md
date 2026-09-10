## Purpose

Provide a repeatable Makefile command for installing the jardec CLI into the
destination selected by Go, including a configured GOBIN directory.

## ADDED Requirements

### Requirement: Makefile CLI installation target
The project SHALL provide a phony `install` Make target that installs the
`jardec` command through `go install ./cmd/jardec`.

#### Scenario: Install with GOBIN configured
- **WHEN** a developer runs `make install` with `GOBIN` configured
- **THEN** the target invokes Go installation for `./cmd/jardec`, allowing Go to place the executable in `GOBIN`

#### Scenario: Install without GOBIN configured
- **WHEN** a developer runs `make install` without `GOBIN` configured
- **THEN** the target invokes Go installation for `./cmd/jardec` using Go's default installation destination
