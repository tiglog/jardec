## 1. Installation target coverage

- [x] 1.1 Add a root-level Makefile behavior test that runs `make install` with a fake `go` command and a configured `GOBIN`, then verify it initially fails because the target is absent.

## 2. Makefile installation support

- [x] 2.1 Add the phony `install` target that delegates to `go install ./cmd/jardec`, then verify the new Makefile behavior test passes.
- [x] 2.2 Run `go test ./...` to verify the complete Go test suite passes with the new Makefile test.
