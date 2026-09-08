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
  engine: forge://github.com/alexandremahdhaoui/testenv-vm/cmd/testenv-vm
  spec:
    providers:
      - name: libvirt
        engine: go://github.com/alexandremahdhaoui/testenv-vm/cmd/providers/testenv-vm-provider-libvirt
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

The provider `engine` line keeps `go://` because `pkg/provider/manager.go` resolves that prefix itself and takes `go://` or a binary path only.

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

### Booting an ISO

`cdrom` attaches an ISO as the first cdrom device. `boot.order: [cdrom]` boots it. A VM with a cdrom and no `baseImage` gets an empty qcow2 disk of the declared size. `readiness.tcp` waits for a port on the address the DHCP lease reveals.

```yaml
images:
  - name: talos-metal-amd64
    spec:
      source: https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.14.0/metal-amd64.iso
      sha256: c70686a8ef7232e3c881523a686519411c73156f14e2911665b0da781c6d262d
networks:
  - name: talos-net
    kind: nat
    spec:
      cidr: "192.168.232.0/24"
vms:
  - name: talos
    spec:
      memory: 2048
      vcpus: 2
      network: talos-net
      disk:
        size: 10G
      cdrom: "{{ .Images.talos-metal-amd64.Path }}"
      boot:
        order: [cdrom]
      readiness:
        tcp:
          port: 50000
          timeout: 5m
```

### Booting a raw image on a declared gateway

A `.gz` source is verified over the compressed bytes and decompressed beside it. The base image format is detected, so a raw image works as a backing file. `gateway` places the host at that address instead of the first one in the CIDR. `readiness.tcp.address` names a static guest address no lease reveals, and the provider reports it as the VM address.

```yaml
images:
  - name: openwrt-x86-64
    spec:
      source: https://downloads.openwrt.org/releases/24.10.8/targets/x86/64/openwrt-24.10.8-x86-64-generic-ext4-combined.img.gz
      sha256: 23872c64fdb66d0765e0d17f71453a66e7a91e9ecb10606d5f738e6d166e14ae
networks:
  - name: openwrt-lan
    kind: nat
    spec:
      cidr: "192.168.1.0/24"
      gateway: "192.168.1.254"
      dhcp:
        enabled: false
vms:
  - name: openwrt
    spec:
      memory: 512
      vcpus: 1
      network: openwrt-lan
      disk:
        baseImage: "{{ .Images.openwrt-x86-64.Path }}"
        size: 1G
      boot:
        order: [hd]
      readiness:
        tcp:
          address: "192.168.1.1"
          port: 22
          timeout: 5m
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
