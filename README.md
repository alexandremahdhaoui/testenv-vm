# testenv-vm: VM Test Environment Engine for Forge

**Provider-Based VM Test Environment Engine Enables Real Infrastructure Testing**

`testenv-vm` is a Forge testenv subengine that provisions virtual machines, networks, and SSH keys for end-to-end testing. The engine uses a provider-based architecture where testenv-vm orchestrates resource creation while delegating actual provisioning to pluggable provider MCP servers (libvirt, QEMU, AWS).

"I needed to test iPXE boot server against real VMs that boot from network, not containers",  "testenv-vm lets me define VMs, networks with DHCP/TFTP, and SSH keys in forge.yaml. The engine handles dependency ordering and cleanup automatically."

testenv-vm solves the infrastructure testing gap for systems requiring actual VMs: PXE/iPXE boot servers, bare-metal provisioning tools, network services needing isolated topologies, and infrastructure controllers requiring specific hardware characteristics.

## Table of Contents

- [What problem does testenv-vm solve?](#what-problem-does-testenv-vm-solve)
- [How do I add testenv-vm to my project?](#how-do-i-add-testenv-vm-to-my-project)
- [Which providers are supported?](#which-providers-are-supported)
- [How do I configure the libvirt provider?](#how-do-i-configure-the-libvirt-provider)
- [How does dependency resolution work?](#how-does-dependency-resolution-work)
- [What happens if VM creation fails?](#what-happens-if-vm-creation-fails)
- [How do I test PXE boot scenarios?](#how-do-i-test-pxe-boot-scenarios)
- [Can I use multiple providers in one environment?](#can-i-use-multiple-providers-in-one-environment)
- [Why a provider-based architecture instead of direct implementation?](#why-a-provider-based-architecture-instead-of-direct-implementation)
- [Why MCP for provider communication?](#why-mcp-for-provider-communication)
- [How does testenv-vm integrate with Forge's testenv interface?](#how-does-testenv-vm-integrate-with-forges-testenv-interface)
- [What are the system requirements?](#what-are-the-system-requirements)
- [How is state managed across create/delete calls?](#how-is-state-managed-across-createdelete-calls)
- [What readiness checks are supported?](#what-readiness-checks-are-supported)
- [Quick Start](#quick-start)
- [Documentation](#documentation)
- [License](#license)

## What problem does testenv-vm solve?

Many systems require testing against actual virtual machines rather than containers. Previously, users had to implement custom infrastructure management: VM lifecycle, network bridges, DHCP/TFTP servers, SSH key distribution, and cleanup. testenv-vm handles all of this through a declarative configuration.

## How do I add testenv-vm to my project?

Add a testenv configuration to your forge.yaml:

```yaml
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

## Which providers are supported?

The mid-term goal is to support these three providers:

- **libvirt**: Full-featured local virtualization with KVM/QEMU, Linux bridges, dnsmasq
- **QEMU**: Lightweight direct QEMU process management without libvirt
- **AWS**: EC2 instances, VPCs, subnets, and security groups

## How do I configure the libvirt provider?

See the [Libvirt Provider Documentation](./docs/libvirt-provider.md) for detailed configuration options, system requirements, network types, cloud-init setup, and troubleshooting.

## How does dependency resolution work?

testenv-vm analyzes template references (e.g., `{{ .Keys.vm-ssh.PublicKey }}`) to build a dependency graph. Resources are created in phases: independent resources run in parallel, dependent resources wait for their dependencies. Templates are rendered just-in-time with values from completed resources.

## What happens if VM creation fails?

If `cleanupOnFailure: true` (default), testenv-vm destroys created resources in reverse dependency order. If `false`, resources remain for debugging. Cleanup uses best-effort: failures are logged but don't block cleanup of other resources.

## How do I test PXE boot scenarios?

Configure a dnsmasq network with TFTP enabled:

```yaml
networks:
  - name: pxe-services
    kind: dnsmasq
    spec:
      dhcp: { enabled: true, rangeStart: "192.168.200.50", rangeEnd: "192.168.200.100" }
      tftp: { enabled: true, root: "./tftp", bootFile: "undionly.kpxe" }
vms:
  - name: pxe-client
    spec:
      boot: { order: [network, hd], firmware: bios }
```

## Can I use multiple providers in one environment?

Yes. Each resource specifies its provider. You might use libvirt for local VMs and AWS for cloud resources in the same test environment.

## Why a provider-based architecture instead of direct implementation?

A: Separation of concerns. The orchestrator handles spec parsing, dependency resolution, and parallel execution. Providers handle infrastructure-specific logic. This enables adding new providers (GCP, Azure, Firecracker) without modifying the core engine.

## Why MCP for provider communication?

A: Forge already uses MCP for engine communication. Using the same protocol for providers maintains consistency, enables process isolation, and allows providers to be developed and versioned independently.

## How does testenv-vm integrate with Forge's testenv interface?

testenv-vm implements two MCP tools: `create` and `delete`. Forge calls `create` with a `CreateInput` containing testID, stage, tmpDir, and spec. testenv-vm returns a `TestEnvArtifact` with files, metadata, and environment variables for subsequent test runners.

## What are the system requirements?

Linux is required for libvirt and QEMU providers. Specific requirements:

- Libvirt provider: libvirt 6.0+, QEMU/KVM, sudo access for bridge creation
- QEMU provider: QEMU 6.0+, KVM access (or TCG for software emulation)
- AWS provider: AWS CLI 2.x, configured credentials

## How is state managed across create/delete calls?

A: State is persisted to `stateDir` (default `.forge/testenv-vm`). The state includes spec, provider status, resource states, execution plan, and artifact paths. This enables reliable cleanup even if the orchestrator restarts.

## What readiness checks are supported?

A: Three check types: SSH (wait for SSH server), TCP (wait for port), and CloudInit (wait for cloud-init completion via SSH). SSH is recommended as it confirms both network connectivity and guest OS readiness.

---

## Quick Start

```bash
# Install forge (if not already installed)
go install github.com/alexandremahdhaoui/forge/cmd/forge@latest

# Run tests with VM infrastructure
forge test e2e
```

## Documentation

- [ARCHITECTURE.md](./ARCHITECTURE.md) - System design and component diagrams
- [Provider Interface](./api/provider/v1/) - Provider MCP tool specifications
- [Examples](./examples/) - Sample configurations for common scenarios

## License

Apache 2.0
