# testenv-vm Design

testenv-vm is a provider-based VM test environment engine that provisions infrastructure resources (keys, networks, VMs) via Model Context Protocol (MCP) connected provider processes.

## Problem Statement

Infrastructure testing -- PXE boot, bare-metal provisioning, network services -- requires real virtual machines. Containers cannot test BIOS/UEFI firmware, network booting, or hardware-level interactions. Each project that needs VM-based testing builds custom lifecycle management: libvirt domain XML generation, QCOW2 overlay creation, cloud-init ISO assembly, bridge networking, DHCP/TFTP configuration, SSH key distribution, and cleanup. This custom work duplicates effort, produces inconsistent cleanup behavior, and lacks dependency resolution between resources.

testenv-vm solves this by separating orchestration from provisioning. A central orchestrator handles spec parsing, Directed Acyclic Graph (DAG) based dependency resolution, template rendering, and parallel execution. Pluggable providers handle infrastructure-specific operations via MCP (JSON-RPC 2.0). Users declare desired state in `forge.yaml`. The engine resolves dependencies, creates resources in the correct order, and guarantees cleanup -- even after partial failures or process restarts.

## Tenets

Ordered by priority. When tenets conflict, the higher-ranked tenet wins.

1. **Correctness over speed.** Resources are created in dependency order. Parallel execution happens only within a phase where resources are independent of each other.
2. **Reliable cleanup over fast cleanup.** Best-effort deletion continues through individual failures. JSON state files enable cleanup across process restarts.
3. **Provider isolation.** Providers run as separate processes. The orchestrator communicates via MCP/JSON-RPC 2.0. Providers can be developed and versioned independently.
4. **Declarative over imperative.** Users declare desired state in `forge.yaml`. The engine resolves dependencies, renders templates, and executes operations.
5. **Fail fast, clean up thoroughly.** Two-phase validation catches errors before any resources are created. If creation fails mid-flight, cleanup runs in reverse dependency order.

## Requirements

From the user perspective:

- Define VM test environments in `forge.yaml` with keys, networks, and VMs.
- Automatic dependency resolution from template references between resources.
- Parallel resource creation within independent execution phases.
- Reliable cleanup even after partial failures or process restarts.
- Template-based cross-resource references (e.g., SSH public key in cloud-init).
- Runtime VM creation during test execution via the client library.
- Readiness checks: SSH, TCP, and CloudInit.
- PXE/iPXE boot scenarios with DHCP/TFTP configuration.
- Well-known image references (e.g., `ubuntu:24.04`) that resolve to cloud image URLs.
- Image caching with checksum verification to avoid repeated downloads.

## Out of Scope

- Container-based test environments (handled by other Forge engines).
- Cloud provider support (AWS, GCP, Azure) -- not implemented.
- QEMU direct provider -- not implemented.
- GUI or web interface.
- Multi-host orchestration.

## Success Criteria

- Create a 4-resource environment (1 key, 1 bridge network, 1 libvirt network, 1 VM) in under 120 seconds.
- Clean up all resources on failure with zero leaked resources.
- Support parallel execution of independent resources within a phase.
- 13 MCP tools per provider covering 3 resource types.
- 9 error codes for structured error handling.
- 6 resource status values tracking full lifecycle.

## Proposed Design

### System Architecture

```
+------------------------------------------------------------------+
|                         Forge Test Runner                         |
|                      (forge test <stage> run)                     |
+------------------------------------------------------------------+
                                |
                                | MCP: "create" / "delete"
                                v
+------------------------------------------------------------------+
|                     testenv-vm MCP Server                         |
|                                                                   |
|  +--------------------+  +---------------+  +-----------------+   |
|  | Spec Parser        |  | DAG Builder   |  | Executor        |   |
|  | - Two-phase        |  | - Deps graph  |  | - Parallel exec |   |
|  |   validation       |  | - Topo sort   |  | - Rollback      |   |
|  | - Template render  |  | (Kahn's alg.) |  | - State persist |   |
|  +--------------------+  +---------------+  +-----------------+   |
|                                                                   |
|  +------------------------------------------------------------+   |
|  |                    Provider Manager                         |   |
|  |  - Start/stop provider MCP servers                          |   |
|  |  - Route operations to providers                            |   |
|  |  - Handle "not implemented" responses                       |   |
|  +------------------------------------------------------------+   |
|                                                                   |
|  +---------------------------+  +-----------------------------+   |
|  | Image Cache Manager       |  | State Store                 |   |
|  | - Download + SHA256 verify|  | - JSON file persistence     |   |
|  | - Well-known registry     |  | - Atomic writes (rename)    |   |
|  | - File-based locking      |  | - Cross-restart recovery    |   |
|  +---------------------------+  +-----------------------------+   |
+------------------------------------------------------------------+
          |                                    |
          | MCP (JSON-RPC 2.0)                 | MCP (JSON-RPC 2.0)
          v                                    v
+------------------+                 +------------------+
| Libvirt Provider |                 |  Stub Provider   |
| - vm, network    |                 | - In-memory      |
| - key            |                 | - E2E testing    |
+------------------+                 +------------------+
```

### Create Flow

```
Orchestrator                          Provider MCP Server
     |                                       |
     |-- Parse spec (v1.SpecFromMap) ------->|
     |-- ValidateEarly (Phase 1) ----------->|
     |-- BuildDAG (template scanning) ------>|
     |-- TopologicalSort (Kahn's alg.) ----->|
     |                                       |
     |-- Start provider processes ---------->|
     |-------- provider_capabilities ------->|
     |<------- {resources, operations} ------|
     |                                       |
     |   For each phase (sequential):        |
     |     For each resource (parallel):     |
     |       |-- Render templates (JIT) ---->|
     |       |-- ValidateResourceRefsLate -->|
     |       |      (Phase 2)               |
     |       |-- <kind>_create {spec} ------>|
     |       |<- {success, resource state} --|
     |       |-- Update state + persist ---->|
     |       |-- Update template context --->|
     |                                       |
     |-- Set status = "ready" -------------->|
     |-- Return TestEnvArtifact ------------>|
```

### Delete Flow

```
[Load State] --> [Start Providers] --> [Reverse Phase Order]
                                              |
                 +----------------------------+
                 v
         +---------------+
         | For each phase | (reverse order, sequential)
         +-------+-------+
                 |
     +-----------+-----------+
     v                       v
[Delete Resource]      [Delete Resource]    (parallel within phase)
     |                       |
     v                       v
[Best Effort]          [Best Effort]
[Continue on Error]    [Continue on Error]
     |                       |
     +-----------+-----------+
                 v
[Delete State File] --> [Remove Artifact Dir] --> [Done]
```

### Dependency Resolution (DAG)

```
Example Environment:
  keys: [vm-ssh]
  networks: [bridge, libvirt-net (attachTo: bridge), dnsmasq (attachTo: bridge)]
  vms: [test-vm (network: libvirt-net, cloudInit.sshKeys: vm-ssh)]

Dependency Graph:
                    +----------+
                    |  vm-ssh  |
                    |  (key)   |
                    +----+-----+
                         |
    +--------------------+--------------------+
    |                                         |
    v                                         |
+----------+                                  |
|  bridge  |                                  |
| (network)|                                  |
+----+-----+                                  |
     |                                        |
     +------------------+                     |
     |                  |                     |
     v                  v                     |
+-----------+    +-----------+                |
|libvirt-net|    |  dnsmasq  |                |
| (network) |    | (network) |                |
+-----+-----+    +-----------+                |
      |                                       |
      +-------------------+-------------------+
                          |
                          v
                    +-----------+
                    |  test-vm  |
                    |   (vm)    |
                    +-----------+

Execution Phases:
  Phase 1: vm-ssh (key), bridge (network)   [parallel: 2 resources]
  Phase 2: libvirt-net, dnsmasq             [parallel: 2 resources]
  Phase 3: test-vm (vm)                     [parallel: 1 resource]
```

### Engine Resolution

Forge resolves `go://` engine URIs to provider binaries. External modules (e.g., `go://github.com/user/repo/cmd/tool@v1.0.0`) use `go run`. Internal packages (e.g., `go://cmd/providers/testenv-vm-provider-stub`) require `FORGE_RUN_LOCAL_ENABLED=true` and resolve to `go run ./<path> --mcp`.

### Testenv Chain Composition

testenv-vm implements Forge's testenv interface via `create` and `delete` MCP tools. Forge chains multiple testenv engines; each receives the previous engine's `Metadata`, `Env`, and `ManagedResources` through `CreateInput`. testenv-vm produces a `TestEnvArtifact` containing VM IPs, SSH key paths, and environment variables for downstream engines and test runners.

### Parallel Execution

DAG topological sort (Kahn's algorithm) produces execution phases. Resources within a phase have no mutual dependencies and execute in parallel using goroutines with `sync.WaitGroup`. Phases execute sequentially. A mutex protects shared state modifications during parallel resource creation.

## Technical Design

### Data Model

Core types from `api/v1/`:

```go
// Top-level specification from forge.yaml
type Spec struct {
    Providers        []ProviderConfig
    Keys             []KeyResource
    Networks         []NetworkResource
    Vms              []VMResource
    Images           []ImageResource
    DefaultProvider  string
    DefaultBaseImage string
    StateDir         string
    ArtifactDir      string
    ImageCacheDir    string
    CleanupOnFailure bool
}

// Forge-to-orchestrator input
type CreateInput struct {
    TestID   string
    Stage    string
    TmpDir   string
    Spec     map[string]any
    Env      map[string]string
    Metadata map[string]string
    RootDir  string
}

// Orchestrator-to-Forge output
type TestEnvArtifact struct {
    TestID           string
    Files            map[string]string
    Metadata         map[string]string
    ManagedResources []string
    Env              map[string]string
}

// Persisted state for cross-restart recovery
type EnvironmentState struct {
    ID            string
    Stage         string
    Status        string           // pending, creating, ready, failed, destroying, destroyed
    Spec          *Spec
    Resources     ResourceMap      // Keys, Networks, VMs
    ExecutionPlan *ExecutionPlan   // Phases of ResourceRefs
    Errors        []ErrorRecord
    ArtifactDir   string
}
```

Provider types from `api/provider/v1/`:

```go
// Standard provider response
type OperationResult struct {
    Success  bool
    Error    *OperationError
    Resource any              // VMState, NetworkState, or KeyState
}

type OperationError struct {
    Code      string
    Message   string
    Retryable bool
    Details   map[string]any
}

// Provider self-description
type CapabilitiesResponse struct {
    ProviderName string
    Version      string
    Resources    []ResourceCapability
}
```

### Protocol Details

Communication uses MCP over JSON-RPC 2.0 on stdio. Each provider process reads JSON-RPC requests from stdin and writes responses to stdout.

**13 MCP Tools per Provider:**

| Tool                 | Description                    |
|----------------------|--------------------------------|
| provider_capabilities| Return supported resources     |
| vm_create            | Create VM from spec            |
| vm_get               | Get VM state by name           |
| vm_list              | List all VMs                   |
| vm_delete            | Delete VM                      |
| network_create       | Create network resource        |
| network_get          | Get network state              |
| network_list         | List networks                  |
| network_delete       | Delete network                 |
| key_create           | Generate SSH key pair          |
| key_get              | Get key state                  |
| key_list             | List keys                      |
| key_delete           | Delete key pair                |

**9 Error Codes:**

| Code              | Retryable | Description                          |
|-------------------|-----------|--------------------------------------|
| NOT_IMPLEMENTED   | No        | Provider does not support operation  |
| NOT_FOUND         | No        | Resource does not exist              |
| ALREADY_EXISTS    | No        | Resource already exists              |
| INVALID_SPEC      | No        | Invalid specification                |
| PROVIDER_ERROR    | Varies    | Provider-specific error              |
| TIMEOUT           | Yes       | Operation timed out                  |
| PERMISSION_DENIED | No        | Insufficient permissions             |
| RESOURCE_BUSY     | Yes       | Resource is in use                   |
| DEPENDENCY_FAILED | No        | Dependency not satisfied             |

### Component Catalog

4 CLI binaries built from `cmd/`:

| Binary                         | Description                                           |
|--------------------------------|-------------------------------------------------------|
| `testenv-vm`                   | Main orchestrator MCP server                          |
| `testenv-vm-provider-libvirt`  | Libvirt provider MCP server                           |
| `testenv-vm-provider-stub`    | In-memory stub provider for E2E testing               |
| `generate-testenv-vm`          | Code generator for MCP server, validation, and docs   |

The `generate-testenv-vm` binary reads `spec.openapi.yaml` and produces `zz_generated.*.go` files in `cmd/testenv-vm/`:
- `zz_generated.main.go` -- MCP server bootstrap
- `zz_generated.mcp.go` -- MCP tool registration and routing
- `zz_generated.validate.go` -- Input validation
- `zz_generated.docs.go` -- Tool documentation

### Package Catalog

**Public packages (`pkg/`):**

| Package              | Contents                                                                        |
|----------------------|---------------------------------------------------------------------------------|
| `pkg/orchestrator/`  | `Orchestrator`, `DAG`, `Executor`, `Rollback`, `ResourcePrefix`, `SubnetOctet` |
| `pkg/provider/`      | `Manager` (lifecycle), `Client` (MCP/JSON-RPC 2.0), engine resolution          |
| `pkg/spec/`          | `TemplateContext`, `RenderSpec`, `ValidateEarly`, `ValidateResourceRefsLate`    |
| `pkg/state/`         | `Store` -- JSON file persistence with atomic writes                             |
| `pkg/image/`         | `CacheManager`, `Downloader`, well-known image registry, checksum verification |
| `pkg/client/`        | `Client` (SSH operations), `RuntimeProvisioner` (runtime VM create/delete)     |

**Internal packages (`internal/`):**

| Package                       | Contents                                          |
|-------------------------------|---------------------------------------------------|
| `internal/providers/libvirt/` | Libvirt provider: domain XML, QCOW2, cloud-init  |
| `internal/providers/stub/`    | In-memory stub provider for testing               |

**API packages (`api/`):**

| Package             | Contents                                                    |
|---------------------|-------------------------------------------------------------|
| `api/v1/`           | Core types: `Spec`, `CreateInput`, `TestEnvArtifact`, etc. |
| `api/provider/v1/`  | Provider interface: `OperationResult`, `VMState`, etc.     |

### Image Caching and Well-Known Registry

`pkg/image/` provides two capabilities:

**Well-known image registry.** Short references like `ubuntu:24.04` resolve to cloud image URLs. The built-in registry includes Ubuntu 24.04, Ubuntu 22.04, and Debian 12. Well-known images skip checksum enforcement because cloud providers periodically update images with security patches. Custom HTTPS URLs require a SHA256 checksum.

**Image cache manager.** `CacheManager` downloads images to a local directory, verifies SHA256 checksums, and stores metadata in `metadata.json`. File-based locking (`flock`) ensures cross-process safety when multiple test environments download images concurrently. Images are referenced in specs via `ImageResource` with source, alias, and optional SHA256 fields. Downloaded images become available as `{{ .Images.<name>.Path }}` in templates.

### Client Library

`pkg/client/` provides a high-level Go API for interacting with VMs during tests:

- `Client` -- SSH command execution, file copy, directory creation, readiness polling.
- `RuntimeProvisioner` -- Creates and deletes VMs at runtime during test execution. Implements the `ClientProvider` interface for VM info lookup. Updates `EnvironmentState` and template context so runtime VMs participate in cleanup.

### Resource Prefix Isolation

`pkg/orchestrator/prefix.go` prevents name collisions when multiple test environments run in parallel:

- `ResourcePrefix(testID)` returns `SHA256(testID)[:6]` -- a 6-character hex prefix.
- `SubnetOctet(testID)` returns `CRC32(testID) % 200 + 20` -- a subnet third octet in [20, 219].
- `PrefixName(prefix, name)` concatenates prefix and resource name with a hyphen separator.

Deterministic derivation from testID means the same test always produces the same prefix.

### Libvirt Provider

The libvirt provider (`internal/providers/libvirt/`) connects to `qemu:///system` and supports:

- **VMs:** Domain XML generation, QCOW2 overlay creation, cloud-init ISO assembly, IP polling, serial console logging.
- **Networks:** 3 kinds -- `bridge` (Linux bridge via `ip link`), `libvirt` (virsh net-define/net-start), `dnsmasq` (standalone dnsmasq process with DHCP/TFTP/DNS).
- **Keys:** SSH key pair generation (RSA, Ed25519, ECDSA) using Go's `crypto` packages.

### Stub Provider

The stub provider (`internal/providers/stub/`) stores resources in memory. It supports all 13 MCP tools and enables full E2E testing without libvirt dependencies. The stub provider returns deterministic IPs and MAC addresses for predictable test assertions.

### Template Resolution

```
Input Spec (with templates):
+------------------------------------------+
| cloudInit:                               |
|   users:                                 |
|     - sshAuthorizedKeys:                 |
|         - "{{ .Keys.vm-ssh.PublicKey }}" |
+------------------------------------------+
                    |
                    v
Template Context (built from completed resources):
+------------------------------------------+
| Keys:                                    |
|   vm-ssh:                               |
|     PublicKey: "ssh-ed25519 AAAA..."    |
|     PrivateKeyPath: "/path/to/key"      |
+------------------------------------------+
                    |
                    v
Rendered Output:
+------------------------------------------+
| cloudInit:                               |
|   users:                                 |
|     - sshAuthorizedKeys:                 |
|         - "ssh-ed25519 AAAA..."         |
+------------------------------------------+

Available Template Variables:
  {{ .Keys.<name>.PublicKey }}        {{ .Networks.<name>.IP }}
  {{ .Keys.<name>.PrivateKeyPath }}   {{ .Networks.<name>.InterfaceName }}
  {{ .Keys.<name>.PublicKeyPath }}    {{ .Networks.<name>.CIDR }}
  {{ .Keys.<name>.Fingerprint }}      {{ .Networks.<name>.UUID }}
  {{ .VMs.<name>.IP }}                {{ .Images.<name>.Path }}
  {{ .VMs.<name>.MAC }}               {{ .Env.<name> }}
  {{ .VMs.<name>.SSHCommand }}        {{ .DefaultBaseImage }}
```

### State Storage

```
{stateDir}/
    +-- state/
          +-- testenv-{testID}.json

{tmpDir}/{testID}/
    +-- vm-ssh.pub           <-- SSH public key
    +-- vm-ssh               <-- SSH private key
    +-- test-vm.console      <-- VM console output

State JSON Structure:
{
  "id": "testenv-abc123",
  "stage": "e2e",
  "status": "ready",
  "createdAt": "2025-01-01T00:00:00Z",
  "updatedAt": "2025-01-01T00:01:00Z",
  "spec": { ... },
  "resources": {
    "keys":     { "<name>": { "provider": "...", "status": "...", "state": {...} } },
    "networks": { "<name>": { "provider": "...", "status": "...", "state": {...} } },
    "vms":      { "<name>": { "provider": "...", "status": "...", "state": {...} } }
  },
  "executionPlan": {
    "phases": [
      { "resources": [{ "kind": "key", "name": "vm-ssh" }] },
      { "resources": [{ "kind": "network", "name": "bridge" }] },
      ...
    ]
  },
  "errors": []
}
```

## Design Patterns

**Provider pattern.** Each provider runs as a separate MCP server process communicating via JSON-RPC 2.0 on stdio. The orchestrator starts providers, fetches capabilities, routes tool calls, and stops providers on shutdown. Process isolation prevents provider crashes from taking down the orchestrator and enables independent versioning.

**DAG-based execution.** Template references and explicit fields (`attachTo`, `network`) define edges in a directed acyclic graph. Kahn's algorithm produces topologically sorted execution phases. Resources within a phase run in parallel via goroutines. Phases execute sequentially, guaranteeing dependency order.

**Two-phase validation.** Phase 1 (`ValidateEarly`) validates spec structure, key types, provider references, and template reference targets before any resources are created. Templated fields are deferred and tracked in `TemplatedFields`. Phase 2 (`ValidateResourceRefsLate`) validates resolved values after template rendering, immediately before resource creation. This catches errors early without blocking template-based flexibility.

**Just-in-time template rendering.** Templates are rendered immediately before each resource creation, using values from already-completed resources in the same environment. The `TemplateContext` is populated incrementally after each successful resource creation. This prevents forward references: a resource can only reference resources in earlier execution phases.

**Resource prefix isolation.** `SHA256(testID)[:6]` prefixes resource names and `CRC32(testID) % 200 + 20` derives subnet octets. This prevents name and network collisions when multiple test environments run in parallel. Deterministic derivation from testID ensures the same test always produces the same prefix.

**Singleton orchestrator.** `sync.Once` ensures a single `Orchestrator` instance across MCP calls within the same process. The generated `zz_generated.main.go` initializes the orchestrator once and reuses it for all `create`/`delete` tool invocations.

## Alternatives Considered

**Do nothing.** Each project implements custom VM management scripts. Rejected: duplicates effort across projects, produces inconsistent cleanup behavior, and lacks dependency resolution between resources.

**Direct implementation (no providers).** Embed libvirt calls directly in the orchestrator binary. Rejected: couples the orchestrator to one hypervisor, prevents adding new backends without modifying the core engine.

**Container-based testing.** Use containers instead of VMs. Rejected: cannot test PXE boot, BIOS/UEFI firmware, network booting, or bare-metal provisioning workflows that require real virtual hardware.

**Terraform/Pulumi integration.** Delegate infrastructure to existing Infrastructure as Code tools. Rejected: heavy dependencies, slow feedback loop for ephemeral test environments, and not designed for the create-test-destroy lifecycle of test environments.

## Risks and Mitigations

**Resource leaks.** Mitigation: JSON file-based state persistence (`pkg/state/Store`) enables cleanup across process restarts. Best-effort deletion continues through individual failures. `cleanupOnFailure` (default true) triggers automatic rollback on creation errors.

**Provider process crashes.** Mitigation: the orchestrator detects provider failure and triggers cleanup of resources created so far. State files record which resources were created and by which provider, enabling recovery.

**Parallel test collisions.** Mitigation: SHA256-based resource prefix isolation (`pkg/orchestrator/prefix.go`) prevents name conflicts. CRC32-based subnet octet derivation prevents network CIDR collisions. Collision probability at 5 concurrent environments is approximately 5%.

**Stale images.** Mitigation: checksum-based image caching validates integrity on download. Custom URLs require SHA256. Well-known images skip checksum enforcement to tolerate upstream security updates.

## Testing Strategy

8 test stages defined in `forge.yaml`:

| Stage                | Type        | Description                                          |
|----------------------|-------------|------------------------------------------------------|
| `lint-tags`          | Lint        | Validates Go build tags                              |
| `lint-licenses`      | Lint        | Validates license headers                            |
| `lint`               | Lint        | Go linter suite                                      |
| `unit`               | Unit        | Pure logic: spec parsing, DAG construction, template rendering, validation, prefix generation |
| `integration`        | Integration | Libvirt provider against real libvirt daemon (in `internal/providers/libvirt/`) |
| `e2e`                | E2E         | Full create/delete cycle using stub provider (no libvirt required) |
| `e2e_libvirt`        | E2E         | Full create cycle using libvirt provider with real VMs |
| `e2e_libvirt_delete` | E2E         | Verifies libvirt resources were properly cleaned up   |

The stub provider enables E2E testing on any machine without libvirt dependencies. Integration and libvirt E2E tests require a running libvirt daemon, `qemu-img`, and ISO generation tools.

## FAQ

**Why separate processes for providers instead of in-process plugins?**
Process isolation prevents provider crashes from taking down the orchestrator. MCP/JSON-RPC 2.0 provides a stable interface boundary. Providers can be developed in any language and versioned independently of the orchestrator.

**Why file-based state instead of a database?**
Simplicity. JSON files are human-readable, easy to debug, and sufficient for single-host test environments. Atomic writes (write to temp file, rename) prevent corruption. No additional dependencies are required.

**Why SHA256 prefix instead of UUID?**
Deterministic derivation from testID. The same testID always produces the same prefix, enabling idempotent operations and predictable resource naming. UUIDs would produce different names on each run, complicating debugging and cleanup.

**How does code generation work?**
`generate-testenv-vm` reads `spec.openapi.yaml` in `cmd/testenv-vm/` and produces `zz_generated.*.go` files for MCP server bootstrap, tool routing, input validation, and documentation. Regenerate with `forge build generate-testenv-vm`. The generated code is committed to the repository.

**How does the well-known image registry work?**
`pkg/image/registry.go` maps short references (e.g., `ubuntu:24.04`) to cloud image download URLs. The registry ships with Ubuntu 24.04, Ubuntu 22.04, and Debian 12. Users reference images in spec as `source: "ubuntu:24.04"`. The `CacheManager` resolves the reference, downloads the image, and caches it locally.

## Appendix

### forge.yaml Example

```yaml
name: testenv-vm
build:
  - name: generate-testenv-vm
    src: ./cmd/testenv-vm
    engine: go://forge-dev

  - name: testenv-vm
    src: ./cmd/testenv-vm
    dest: ./build/bin
    engine: go://go-build
    depends: [generate-testenv-vm]

  - name: testenv-vm-provider-stub
    src: ./cmd/providers/testenv-vm-provider-stub
    dest: ./build/bin
    engine: go://go-build

  - name: testenv-vm-provider-libvirt
    src: ./cmd/providers/testenv-vm-provider-libvirt
    dest: ./build/bin
    engine: go://go-build

test:
  - name: lint-tags
    runner: go://go-lint-tags
  - name: lint-licenses
    runner: go://go-lint-licenses
  - name: lint
    runner: go://go-lint
  - name: unit
    runner: go://go-test
  - name: integration
    runner: go://go-test
  - name: e2e
    runner: go://go-test
  - name: e2e_libvirt
    runner: go://go-test
  - name: e2e_libvirt_delete
    runner: go://go-test
```

### Testenv Configuration in forge.yaml

```yaml
testenv:
  - name: testenv-vm
    engine: go://cmd/testenv-vm
    spec:
      providers:
        - name: libvirt
          engine: go://cmd/providers/testenv-vm-provider-libvirt
          default: true

      images:
        - name: ubuntu
          spec:
            source: "ubuntu:24.04"
            alias: "base-image"

      keys:
        - name: vm-ssh
          spec:
            type: ed25519

      networks:
        - name: br0
          kind: bridge
          spec:
            cidr: 192.168.100.1/24
            gateway: 192.168.100.1
        - name: testnet
          kind: libvirt
          spec:
            attachTo: br0
            cidr: 192.168.100.1/24
            dhcp:
              enabled: true
              rangeStart: 192.168.100.10
              rangeEnd: 192.168.100.50

      vms:
        - name: test-vm
          spec:
            memory: 2048
            vcpus: 2
            network: testnet
            disk:
              baseImage: "{{ .Images.ubuntu.Path }}"
              size: 20G
            boot:
              order: [hd]
              firmware: bios
            cloudInit:
              hostname: test-vm
              users:
                - name: ubuntu
                  sudo: "ALL=(ALL) NOPASSWD:ALL"
                  sshAuthorizedKeys:
                    - "{{ .Keys.vm-ssh.PublicKey }}"
            readiness:
              ssh:
                enabled: true
                timeout: 3m
                user: ubuntu
                privateKey: "{{ .Keys.vm-ssh.PrivateKeyPath }}"
```

### Repository Structure

```
testenv-vm/
+-- api/
|   +-- v1/                              # Core API types (Spec, CreateInput, state)
|   +-- provider/v1/                     # Provider interface types (operations, resources, errors)
+-- cmd/
|   +-- testenv-vm/                      # Main orchestrator (create.go, delete.go, zz_generated.*.go)
|   +-- providers/
|       +-- testenv-vm-provider-libvirt/  # Libvirt provider binary
|       +-- testenv-vm-provider-stub/    # Stub provider binary
+-- pkg/
|   +-- orchestrator/                    # DAG, Executor, Rollback, prefix isolation
|   +-- provider/                        # Manager, Client (MCP/JSON-RPC 2.0), engine resolution
|   +-- spec/                            # Parser, two-phase validator, template renderer
|   +-- state/                           # JSON file-based state persistence
|   +-- image/                           # CacheManager, Downloader, well-known registry
|   +-- client/                          # SSH client, RuntimeProvisioner, file operations
+-- internal/
|   +-- providers/
|       +-- libvirt/                     # Libvirt provider implementation + integration tests
|       +-- stub/                        # Stub provider implementation
+-- test/
|   +-- e2e/                             # E2E tests (stub provider)
|   +-- e2e-libvirt/                     # E2E tests (libvirt provider, create)
|   +-- e2e-libvirt-delete/              # E2E tests (libvirt provider, delete verification)
+-- docs/
|   +-- libvirt-provider.md              # Libvirt provider user guide
|   +-- runtime-vm-creation.md           # Runtime VM creation guide
+-- forge.yaml                           # Build and test configuration
+-- DESIGN.md                            # This document
```
