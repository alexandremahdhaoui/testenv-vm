# Contributing to testenv-vm

**Contribute to declarative VM test environments -- clone, build, test, submit.**

## Quick start

```bash
git clone https://github.com/alexandremahdhaoui/testenv-vm.git
cd testenv-vm
direnv allow                  # Load environment variables
forge build                   # Build all 4 targets
forge test run unit           # Run unit tests (no external deps)
forge test run e2e            # Run e2e tests with stub provider (no external deps)
forge test-all                # Full suite -- 8 stages, stops on first failure
```

Prerequisites: Go 1.25+, [forge](https://github.com/alexandremahdhaoui/forge).
Optional (for libvirt tests): libvirt 6.0+, QEMU/KVM, `qemu-img`, `genisoimage`/`mkisofs`/`xorriso`.

## How do I structure commits?

Each commit uses an emoji prefix and a structured body.

| Emoji | Meaning     |
|-------|-------------|
| `✨`  | New feature |
| `🐛`  | Bug fix     |
| `📖`  | Documentation |
| `🌱`  | Misc (chore, test, refactor) |
| `⚠`   | Breaking change (maintainer approval required) |

```
✨ Short imperative summary (50 chars or less)

Why: Explain the motivation. What problem exists?

How: Describe the approach. What strategy did you choose?

What:

- pkg/foo/bar.go: description of change
- cmd/baz/main.go: description of change

How changes were verified:

- Unit tests for new logic (go test)
- forge test-all: all stages passed

Signed-off-by: Your Name <your.email@example.com>
```

Every commit requires `Signed-off-by`. Use `git commit -s` to add it automatically.

## How do I submit a pull request?

1. Create a feature branch from `main`.
2. Run `forge test-all` and confirm all 8 stages pass.
3. Open a PR with a title and description explaining what changed and why.
4. Address review feedback promptly.

## How do I run tests?

8 test stages run sequentially during `forge test-all`, stopping on first failure.

| Stage                | Build tag             | Requirements                     |
|----------------------|-----------------------|----------------------------------|
| `lint-tags`          | --                    | None                             |
| `lint-licenses`      | --                    | None                             |
| `lint`               | --                    | None                             |
| `unit`               | `unit`                | None                             |
| `integration`        | `integration`         | libvirt, qemu-img, genisoimage   |
| `e2e`                | `e2e`                 | None (uses stub provider)        |
| `e2e_libvirt`        | `e2e_libvirt`         | libvirt, QEMU/KVM, full stack    |
| `e2e_libvirt_delete` | `e2e_libvirt_delete`  | Same as `e2e_libvirt`            |

```bash
forge test run lint           # Run a single stage
forge test run unit           # Fast iteration -- no external deps
forge test-all                # Full suite -- run before submitting PRs
```

Contributors without libvirt can run `lint-tags`, `lint-licenses`, `lint`, `unit`, and `e2e` locally. CI runs the full suite.

## How is the project structured?

```
testenv-vm/
├── api/
│   ├── v1/                     # Spec types, artifacts, state (generated + manual)
│   └── provider/v1/            # Provider protocol: operations, resources, error codes
├── cmd/
│   ├── testenv-vm/             # Main engine binary (MCP server)
│   └── providers/
│       ├── testenv-vm-provider-stub/     # Stub provider (testing)
│       └── testenv-vm-provider-libvirt/  # Libvirt provider (production)
├── internal/providers/
│   ├── libvirt/                # Libvirt provider: VMs, networks, keys via go-libvirt
│   └── stub/                   # In-memory stub provider for e2e tests
├── pkg/                        # Public packages (see below)
├── test/
│   ├── e2e/                    # E2E tests (stub provider)
│   ├── e2e-libvirt/            # E2E tests (real libvirt)
│   └── e2e-libvirt-delete/     # Cleanup verification tests
├── docs/                       # User documentation
├── forge.yaml                  # Build and test configuration
└── DESIGN.md                   # Technical design document
```

## What does each CLI tool do?

testenv-vm exposes 3 engine-level MCP tools and 13 MCP tools per provider.

**Engine tools** (`testenv-vm` binary):

| Tool               | Description                                        |
|--------------------|----------------------------------------------------|
| `create`           | Create test environment (keys, networks, VMs)      |
| `delete`           | Delete test environment and clean up resources      |
| `config-validate`  | Validate testenv-vm spec against OpenAPI schema     |

**Provider tools** (per provider, 13 total):

| Category | Tools                                        | Description                        |
|----------|----------------------------------------------|------------------------------------|
| Key      | `key_create`, `key_get`, `key_list`, `key_delete` | SSH key lifecycle (ed25519, rsa, ecdsa) |
| Network  | `network_create`, `network_get`, `network_list`, `network_delete` | Virtual network lifecycle          |
| VM       | `vm_create`, `vm_get`, `vm_list`, `vm_delete` | Virtual machine lifecycle          |
| System   | `provider_capabilities`                      | Report supported resources/operations |

## What does each package do?

**`pkg/` (public):**

| Package        | Description                                                              |
|----------------|--------------------------------------------------------------------------|
| `orchestrator` | DAG-based resource orchestration: dependency resolution, parallel phases, rollback |
| `client`       | SSH client for VM operations: command execution, file transfer, provisioning |
| `provider`     | MCP client manager: starts provider processes, routes operations         |
| `spec`         | Spec parsing, 2-phase validation, Go template rendering                  |
| `image`        | Image cache with file-based locking, HTTP download, SHA256 verification  |
| `state`        | JSON-based persistent state for reliable cleanup across restarts         |

**`internal/providers/`:**

| Package   | Description                                                                |
|-----------|----------------------------------------------------------------------------|
| `libvirt` | Real provider: go-libvirt API, QCOW2 overlays, cloud-init ISOs, DHCP/TFTP |
| `stub`    | In-memory mock provider for e2e testing without infrastructure             |

## How do I create a new engine?

testenv-vm uses `go://forge-dev` for code generation. The workflow:

1. Define an OpenAPI spec (`cmd/testenv-vm/spec.openapi.yaml`).
2. Configure generation in `cmd/testenv-vm/forge-dev.yaml`.
3. Run `forge build generate-testenv-vm` to produce `zz_generated.*.go` files.
4. Implement `Create()` and `Delete()` functions in `create.go` and `delete.go`.

Generated files (never edit manually):

| File                       | Content                              |
|----------------------------|--------------------------------------|
| `zz_generated.main.go`    | Bootstrap and MCP server setup       |
| `zz_generated.mcp.go`     | MCP tool registration and wrappers   |
| `zz_generated.validate.go`| OpenAPI-based validation             |
| `zz_generated.spec.go`    | Spec type definitions (in `api/v1/`) |
| `zz_generated.docs.go`    | Generated documentation              |

Re-run code generation after modifying `spec.openapi.yaml` or `forge-dev.yaml`.

## What conventions must I follow?

**Build tags.** Every test file starts with the appropriate directive:

```go
//go:build unit

package foo
```

Valid tags: `unit`, `integration`, `e2e`, `e2e_libvirt`, `e2e_libvirt_delete`.

**License headers.** Every `.go` file requires this header (after the build tag, if present):

```go
// Copyright 2025 Alexandre Mahdhaoui
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
```

**Generated files.** Files matching `zz_generated.*.go` are produced by `forge build generate-testenv-vm`. Never edit them manually.

**Linting.** Run `forge test run lint` before committing. `lint-tags` validates build tags. `lint-licenses` validates license headers.

## License

testenv-vm is licensed under [Apache 2.0](LICENSE).
By contributing, you agree that your contributions are licensed under the same terms.
