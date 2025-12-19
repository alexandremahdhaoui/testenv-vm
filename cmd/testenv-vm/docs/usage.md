# testenv-vm

**Provider-based VM test environment subengine for Forge.**

> "I need to spin up VMs with networking and SSH access for integration tests, but managing libvirt, cloud-init, and cleanup is painful. testenv-vm handles all of that through a simple YAML spec."

## What problem does testenv-vm solve?

testenv-vm orchestrates VM test environments through pluggable provider MCP servers. It manages SSH keys, networks, and VMs with automatic dependency resolution and cleanup.

## Table of Contents

- [Quick Start](#quick-start)
- [Configuration Reference](#configuration-reference)
- [Resource Types](#resource-types)
- [Template Syntax](#template-syntax)
- [Image Management](#image-management)
- [Environment Variables](#environment-variables)

## Quick Start

```yaml
# forge.yaml
testenv:
  engine: testenv-vm
  spec:
    providers:
      - name: libvirt
        engine: go://github.com/alexandremahdhaoui/testenv-vm-provider-libvirt
        default: true
    keys:
      - name: vm-ssh
        spec:
          type: ed25519
    networks:
      - name: test-net
        kind: bridge
        spec:
          cidr: 192.168.100.1/24
          dhcp:
            enabled: true
            rangeStart: 192.168.100.10
            rangeEnd: 192.168.100.100
    vms:
      - name: worker
        spec:
          memory: 2048
          vcpus: 2
          network: test-net
          disk:
            baseImage: "{{ .DefaultBaseImage }}"
            size: 20G
          boot:
            order: [hd]
          cloudInit:
            hostname: worker
            users:
              - name: testuser
                sudo: "ALL=(ALL) NOPASSWD:ALL"
                sshAuthorizedKeys:
                  - "{{ .Keys.vm-ssh.PublicKey }}"
```

## Configuration Reference

| Field | Type | Description |
|-------|------|-------------|
| `stateDir` | string | Directory for persisting environment state |
| `artifactDir` | string | Directory for storing artifacts (keys, logs) |
| `cleanupOnFailure` | bool | Clean up resources on failure (default: true) |
| `imageCacheDir` | string | Directory for caching VM base images |
| `defaultBaseImage` | string | Default base image for VMs |
| `providers` | array | Provider configurations (required) |
| `defaultProvider` | string | Name of default provider |
| `keys` | array | SSH key resources |
| `networks` | array | Network resources |
| `vms` | array | VM resources |

## Resource Types

### Keys

SSH key pairs for VM authentication.

```yaml
keys:
  - name: my-key
    spec:
      type: ed25519  # or rsa, ecdsa
      bits: 4096     # for rsa
      comment: "test key"
```

### Networks

Network infrastructure with optional DHCP/DNS.

```yaml
networks:
  - name: my-net
    kind: bridge  # or libvirt, dnsmasq
    spec:
      cidr: 192.168.100.1/24
      gateway: 192.168.100.1
      dhcp:
        enabled: true
        rangeStart: 192.168.100.10
        rangeEnd: 192.168.100.100
```

### VMs

Virtual machines with cloud-init configuration.

```yaml
vms:
  - name: my-vm
    spec:
      memory: 2048
      vcpus: 2
      network: my-net
      disk:
        baseImage: "{{ .Images.ubuntu.Path }}"
        size: 20G
      boot:
        order: [hd]
      cloudInit:
        hostname: my-vm
        packages: [curl, jq]
        users:
          - name: testuser
            sshAuthorizedKeys:
              - "{{ .Keys.my-key.PublicKey }}"
```

## Template Syntax

Resources can reference each other using Go templates:

| Template | Description |
|----------|-------------|
| `{{ .Keys.<name>.PublicKey }}` | SSH public key content |
| `{{ .Keys.<name>.PrivateKeyPath }}` | Path to private key file |
| `{{ .Networks.<name>.CIDR }}` | Network CIDR |
| `{{ .Networks.<name>.Gateway }}` | Network gateway IP |
| `{{ .Images.<name>.Path }}` | Path to cached image |
| `{{ .DefaultBaseImage }}` | Default base image path |

## Image Management

Define images to download and cache:

```yaml
images:
  - name: ubuntu
    spec:
      source: ubuntu:24.04  # well-known reference
  - name: custom
    spec:
      source: https://example.com/image.qcow2
      sha256: abc123...
```

Well-known images: `ubuntu:24.04`, `ubuntu:22.04`, `debian:12`

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `TESTENV_VM_STATE_DIR` | State directory | `.forge/testenv-vm/state` |
| `TESTENV_VM_CLEANUP_ON_FAILURE` | Rollback on failure | `true` |
| `TESTENV_VM_IMAGE_CACHE_DIR` | Image cache directory | `/tmp/testenv-vm/images` |
| `TESTENV_VM_DEBUG` | Enable verbose logging | (unset) |
