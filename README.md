# testenv-vm

**A Forge testenv engine that provisions virtual machines, networks, and SSH keys for infrastructure testing.**

> "I test PXE boot servers against real VMs that boot from network, not containers.
> testenv-vm lets me declare VMs, networks with DHCP/TFTP, and SSH keys in forge.yaml.
> The engine handles dependency ordering, parallel execution, and cleanup automatically."
> -- Infrastructure Engineer

## What problem does testenv-vm solve?

Infrastructure testing (PXE boot, bare-metal provisioning, network services) requires real VMs, not containers.
Each project builds custom VM lifecycle management: domain XML, QCOW2 overlays, cloud-init ISOs, bridge networking, DHCP/TFTP, SSH key distribution, cleanup.
How do you get declarative, provider-agnostic VM test environments with automatic dependency resolution?
testenv-vm handles it through `forge.yaml` configuration.
It manages 3 resource types (keys, networks, VMs), supports 3 network kinds (bridge, libvirt, dnsmasq), 3 key types (rsa, ed25519, ecdsa), and exposes 13 Model Context Protocol (MCP) tools per provider.

## Quick Start

```bash
# Install forge
go install github.com/alexandremahdhaoui/forge/cmd/forge@latest
```

```yaml
# forge.yaml
engines:
  - alias: e2e-testenv
    type: testenv
    testenv:
      - engine: go://github.com/alexandremahdhaoui/testenv-vm/cmd/testenv-vm
        spec:
          providers:
            - name: libvirt
              engine: go://github.com/alexandremahdhaoui/testenv-vm/cmd/providers/testenv-vm-provider-libvirt
          keys:
            - name: vm-ssh
              spec: { type: ed25519 }
          networks:
            - name: test-net
              kind: bridge
              spec: { cidr: "192.168.100.1/24" }
          vms:
            - name: test-vm
              spec:
                memory: 2048
                vcpus: 2
                network: "{{ .Networks.test-net.Name }}"
                cloudInit:
                  users:
                    - name: ubuntu
                      sshAuthorizedKeys: ["{{ .Keys.vm-ssh.PublicKey }}"]
```

```bash
# Run tests with VM infrastructure
forge test e2e
```

## How does it work?

```
+-------------------------------------------------------------------+
|                       Forge Test Runner                            |
+-------------------------------------------------------------------+
                              |
                              | MCP: "create" / "delete"
                              v
+-------------------------------------------------------------------+
|                   testenv-vm MCP Server                            |
|                                                                    |
|  +----------------+  +-------------+  +------------------------+   |
|  | Spec Parser    |  | DAG Builder |  | Executor               |   |
|  | - Validate     |  | - Deps graph|  | - Parallel phases      |   |
|  | - Template     |  | - Topo sort |  | - Rollback on failure  |   |
|  +----------------+  +-------------+  +------------------------+   |
|                                                                    |
|  +--------------------------------------------------------------+  |
|  |                    Provider Manager                           |  |
|  |  - Start/stop provider MCP servers                            |  |
|  |  - Route operations to providers                              |  |
|  +--------------------------------------------------------------+  |
+-------------------------------------------------------------------+
          |                                    |
          | MCP                                | MCP
          v                                    v
+------------------+                +------------------+
| Libvirt Provider |                |  Stub Provider   |
| - vm, network    |                | - In-memory mock |
| - key            |                | - E2E testing    |
+------------------+                +------------------+
```

Forge calls testenv-vm's `create` MCP tool with a spec. The orchestrator parses the spec, builds a Directed Acyclic Graph (DAG) from template references, and executes resources in parallel phases. Each provider runs as a separate MCP server process, enabling process isolation and independent versioning. See [DESIGN.md](./DESIGN.md) for the full technical design.

## Table of Contents

- [How do I configure?](#how-do-i-configure)
- [How do I build and test?](#how-do-i-build-and-test)
- [How does dependency resolution work?](#how-does-dependency-resolution-work)
- [How do I create VMs at runtime?](#how-do-i-create-vms-at-runtime)
- [How do I test PXE boot scenarios?](#how-do-i-test-pxe-boot-scenarios)
- [FAQ](#faq)
- [Documentation](#documentation)
- [License](#license)

## How do I configure?

Configuration lives in `forge.yaml` under the `testenv` section.

**Resource types:**
- **Keys** -- 3 types: rsa, ed25519, ecdsa. Generate SSH key pairs for VM access.
- **Networks** -- 3 kinds: bridge (Linux bridge), libvirt (virsh net-define), dnsmasq (DHCP/TFTP).
- **VMs** -- Define memory, vCPUs, disk, cloud-init, boot order, and readiness checks.

Template references (e.g., `{{ .Keys.vm-ssh.PublicKey }}`) link resources and drive dependency resolution.

See [Libvirt Provider](./docs/libvirt-provider.md) for provider-specific configuration options.

## How do I build and test?

4 build targets: `testenv-vm`, `testenv-vm-provider-stub`, `testenv-vm-provider-libvirt`, `generate-testenv-vm`.

8 test stages: 3 lint (`lint-tags`, `lint-licenses`, `lint`), 1 unit, 1 integration, 3 e2e (`e2e`, `e2e_libvirt`, `e2e_libvirt_delete`).

```bash
forge build                    # Build all artifacts
forge test-all                 # Run all test stages
forge test run unit            # Run a specific stage
forge test run e2e             # Run stub-based E2E tests
forge test run e2e_libvirt     # Run libvirt E2E tests (requires libvirt)
```

## How does dependency resolution work?

Template references (e.g., `{{ .Keys.vm-ssh.PublicKey }}`) define edges in a DAG. Topological sort produces execution phases. Resources within a phase execute in parallel. Phases execute sequentially.

Templates render just-in-time: immediately before resource creation, using values from already-completed resources. This prevents forward references and ensures deterministic execution order.

## How do I create VMs at runtime?

Use `client.NewClient` with `WithProvisioner` to create VMs during test execution:

```go
result, _ := orchestrator.Create(ctx, input)

c, _ := client.NewClient(result.Provisioner, "control-plane",
    client.WithProvisioner(result.Provisioner))

worker, _ := c.CreateVM(ctx, "worker-1", v1.VMSpec{
    Memory: 2048, VCPUs: 2, Network: "test-net",
})
worker.WaitReady(ctx, 2*time.Minute)
```

Runtime VMs integrate with existing resources via templates and are cleaned up automatically. See [Runtime VM Creation](./docs/runtime-vm-creation.md) for details.

## How do I test PXE boot scenarios?

Configure a dnsmasq network with DHCP/TFTP:

```yaml
networks:
  - name: pxe-services
    kind: dnsmasq
    spec:
      dhcp:
        enabled: true
        rangeStart: "192.168.200.50"
        rangeEnd: "192.168.200.100"
      tftp:
        enabled: true
        root: "./tftp"
        bootFile: "undionly.kpxe"
vms:
  - name: pxe-client
    spec:
      boot: { order: [network, hd], firmware: bios }
```

## FAQ

**What providers are available?**
Libvirt (full-featured local virtualization) and stub (in-memory mock for E2E testing). Each provider exposes 13 MCP tools covering 3 resource types.

**What readiness checks are supported?**
SSH (wait for SSH server), TCP (wait for port), and CloudInit (wait for cloud-init completion via SSH). SSH confirms both network connectivity and guest OS readiness.

**What happens if VM creation fails?**
When `cleanupOnFailure` is `true` (default), testenv-vm destroys created resources in reverse dependency order. Best-effort deletion continues through individual failures.

**How is state managed?**
JSON files in `stateDir` (default `.forge/testenv-vm`). State persistence enables reliable cleanup across process restarts.

**Can I use multiple providers?**
Yes. Each resource specifies its provider. Different resources in the same environment can use different providers.

**What are the system requirements?**
Linux, libvirt 6.0+, QEMU/KVM, sudo access for bridge creation. The stub provider has no system requirements.

**Why MCP for provider communication?**
Forge uses MCP for engine communication. Using the same protocol for providers maintains consistency, enables process isolation, and allows independent versioning.

## Documentation

**User guides:**
- [Libvirt Provider](./docs/libvirt-provider.md) -- configuration, network types, cloud-init, troubleshooting
- [Runtime VM Creation](./docs/runtime-vm-creation.md) -- dynamic VM provisioning during tests

**Design:**
- [DESIGN.md](./DESIGN.md) -- architecture, data model, protocol details

**API:**
- [Provider Interface](./api/provider/v1/) -- provider MCP tool specifications

## Contributing

Contributions welcome. Open an issue or pull request.

## License

Apache 2.0
