# Libvirt Provider: Full-Featured Local VM Virtualization

**Local KVM/QEMU Virtualization Enables Real Infrastructure Testing Without Cloud Dependencies**

The libvirt provider for testenv-vm brings production-grade virtualization to your local development environment. It creates real virtual machines using libvirt/KVM, manages isolated networks with DHCP, and handles SSH key distribution - all through a simple declarative configuration.

"I needed to test my iPXE boot server against real VMs that boot from network, not containers. testenv-vm with libvirt lets me spin up isolated test environments in seconds, with proper networking and cloud-init configuration. The provider handles all the libvirt XML complexity for me."

The libvirt provider solves the local testing gap for systems requiring actual virtual machines: PXE/iPXE boot servers, network services needing isolated topologies, infrastructure controllers, and any application that needs to run in a VM rather than a container.

## Table of Contents

- [What is the libvirt provider?](#what-is-the-libvirt-provider)
- [What are the system requirements?](#what-are-the-system-requirements)
- [How do I install dependencies?](#how-do-i-install-dependencies)
- [How do I configure the provider?](#how-do-i-configure-the-provider)
- [What network types are supported?](#what-network-types-are-supported)
- [How do I create SSH keys?](#how-do-i-create-ssh-keys)
- [How do I configure VMs with cloud-init?](#how-do-i-configure-vms-with-cloud-init)
- [How do I connect to VMs via SSH?](#how-do-i-connect-to-vms-via-ssh)
- [What environment variables are available?](#what-environment-variables-are-available)
- [How do I troubleshoot permission issues?](#how-do-i-troubleshoot-permission-issues)
- [What base images are supported?](#what-base-images-are-supported)
- [How does the provider connect to libvirt?](#how-does-the-provider-connect-to-libvirt)
- [How are disk images created?](#how-are-disk-images-created)
- [How is IP resolution handled?](#how-is-ip-resolution-handled)
- [What state is persisted?](#what-state-is-persisted)
- [Configuration Reference](#configuration-reference)
- [Quick Start](#quick-start)

---

## What is the libvirt provider?

The libvirt provider is a testenv-vm provider that manages virtual machines, networks, and SSH keys through libvirt. It uses:

- **KVM/QEMU** for hardware-accelerated virtualization
- **libvirt networks** with built-in DHCP for automatic IP assignment
- **cloud-init** for VM configuration (users, SSH keys, packages)
- **qemu-img** for copy-on-write disk images from base images

The provider supports three resource types:
- **keys**: SSH key pairs (ed25519 or RSA)
- **networks**: Virtual networks (NAT, isolated, or bridge mode)
- **vms**: Virtual machines with cloud-init configuration

## What are the system requirements?

| Requirement | Minimum Version | Notes |
|-------------|-----------------|-------|
| Linux | Any recent | Windows/macOS not supported |
| libvirt | 6.0+ | Must have libvirtd running |
| QEMU/KVM | 6.0+ | KVM for hardware acceleration |
| qemu-img | Any | For disk image creation |
| genisoimage/mkisofs/xorriso | Any | For cloud-init ISO |

**Hardware requirements:**
- CPU with virtualization extensions (Intel VT-x or AMD-V)
- Sufficient RAM for VMs (each VM requires its configured memory)
- Disk space for base images and VM disks

## How do I install dependencies?

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install -y \
    qemu-kvm \
    libvirt-daemon-system \
    libvirt-clients \
    qemu-utils \
    genisoimage

# Add user to libvirt group
sudo usermod -aG libvirt $USER
sudo usermod -aG kvm $USER

# Re-login or run: newgrp libvirt
```

**Fedora/RHEL:**
```bash
sudo dnf install -y \
    qemu-kvm \
    libvirt \
    libvirt-client \
    qemu-img \
    genisoimage

sudo systemctl enable --now libvirtd
sudo usermod -aG libvirt $USER
```

**Arch Linux:**
```bash
sudo pacman -S \
    qemu-full \
    libvirt \
    virt-manager \
    cdrtools

sudo systemctl enable --now libvirtd
sudo usermod -aG libvirt $USER
```

## How do I configure the provider?

Add the libvirt provider to your testenv spec:

```yaml
providers:
  - name: libvirt
    engine: go://github.com/alexandremahdhaoui/testenv-vm/cmd/providers/testenv-vm-provider-libvirt
    default: true
    spec: {}
```

For local development, use a local binary path:

```yaml
providers:
  - name: libvirt
    engine: ./build/bin/testenv-vm-provider-libvirt
    default: true
    spec: {}
```

## What network types are supported?

The libvirt provider supports three network types:

### NAT Network (default)
VMs can access external networks through NAT. Best for general testing.

```yaml
networks:
  - name: test-net
    kind: nat
    provider: libvirt
    spec:
      cidr: "192.168.100.0/24"
```

### Isolated Network
VMs can only communicate with each other. No external access.

```yaml
networks:
  - name: isolated-net
    kind: isolated
    provider: libvirt
    spec:
      cidr: "10.0.0.0/24"
```

### Bridge Network
VMs connect to an existing bridge interface on the host.

```yaml
networks:
  - name: bridge-net
    kind: bridge
    provider: libvirt
    spec:
      cidr: "192.168.1.0/24"
```

**CIDR handling:**
- Gateway is automatically set to `.1` address (e.g., `192.168.100.1`)
- DHCP range starts at `.2` and ends at the last usable address
- Netmask is derived from CIDR prefix

## How do I create SSH keys?

The provider generates SSH key pairs and stores them in the state directory:

```yaml
keys:
  - name: vm-ssh-key
    provider: libvirt
    spec:
      type: ed25519  # or "rsa"
```

For RSA keys, you can specify the bit size:

```yaml
keys:
  - name: vm-ssh-key
    provider: libvirt
    spec:
      type: rsa
      bits: 4096  # default: 4096
```

**Supported key types:**
- `ed25519` (default, recommended): Modern, secure, fast
- `rsa`: Traditional RSA keys, configurable bit size

Keys are stored at:
- Private key: `{stateDir}/keys/{keyName}`
- Public key: `{stateDir}/keys/{keyName}.pub`

## How do I configure VMs with cloud-init?

VMs are configured using cloud-init. The provider generates a cloud-init ISO that is attached to the VM:

```yaml
vms:
  - name: my-vm
    provider: libvirt
    spec:
      memory: 2048        # MB
      vcpus: 2
      network: test-net   # Reference to network resource
      disk:
        baseImage: "/path/to/ubuntu-24.04-cloudimg.qcow2"
        size: "20G"
      cloudInit:
        hostname: my-vm
        users:
          - name: testuser
            sudo: "ALL=(ALL) NOPASSWD:ALL"
            sshAuthorizedKeys:
              - "{{ .Keys.vm-ssh-key.PublicKey }}"
        packages:
          - curl
          - vim
        runCommands:
          - "echo 'Hello from cloud-init' > /tmp/hello.txt"
```

**Template references:**
Use Go template syntax to reference other resources:
- `{{ .Keys.{keyName}.PublicKey }}` - SSH public key content
- `{{ .Networks.{networkName}.Name }}` - Network name
- `{{ .Env.VARIABLE_NAME }}` - Environment variables

## How do I connect to VMs via SSH?

After creation, the VM state includes an SSH command:

```yaml
# In the returned artifact:
env:
  TESTENV_VM_MY_VM_SSH: "ssh -i /tmp/testenv-vm/keys/vm-ssh-key -o StrictHostKeyChecking=no testuser@192.168.100.5"
```

You can also use the testenv-vm client library:

```go
import (
    "github.com/alexandremahdhaoui/testenv-vm/pkg/client"
    "github.com/alexandremahdhaoui/testenv-vm/pkg/client/provider"
)

// Create provider with libvirt connection
prov := provider.NewLibvirtProvider(
    provider.WithUser("testuser"),
    provider.WithKeyPath("/path/to/private-key"),
    provider.WithConnectionURI("qemu:///session"),
)

// Create client for specific VM
c, err := client.NewClient(prov, "my-vm")
if err != nil {
    log.Fatal(err)
}
defer c.Close()

// Wait for VM to be ready
ctx := context.Background()
err = c.WaitReady(ctx, 2*time.Minute)

// Run commands
stdout, stderr, err := c.Run(ctx, "hostname")
```

## What environment variables are available?

| Variable | Default | Description |
|----------|---------|-------------|
| `TESTENV_VM_LIBVIRT_URI` | `qemu:///session` (user) or `qemu:///system` (root) | Libvirt connection URI |
| `TESTENV_VM_STATE_DIR` | `/tmp/testenv-vm-{uid}` (session) or `/var/lib/testenv-vm` (system) | Directory for keys, disks, ISOs |
| `TESTENV_VM_IMAGE_CACHE_DIR` | `/tmp/testenv-vm-images` | Base image cache directory |

**Session vs System mode:**
- **Session mode** (`qemu:///session`): VMs run as your user, no root required
- **System mode** (`qemu:///system`): VMs run as libvirt-qemu user, requires polkit or root

## How do I troubleshoot permission issues?

### "Permission denied" accessing disk files

The libvirt daemon runs as a different user (libvirt-qemu) and needs access to disk files:

```bash
# Option 1: Use /tmp-based state directory (default for session mode)
export TESTENV_VM_STATE_DIR=/tmp/testenv-vm-$(id -u)

# Option 2: Set ACLs on your directory
setfacl -m g:libvirt-qemu:rwx /path/to/state
setfacl -d -m g:libvirt-qemu:rwx /path/to/state

# Option 3: Add yourself to the libvirt groups
sudo usermod -aG libvirt,libvirt-qemu,kvm $USER
```

### "Cannot access storage file" error

The base image path must be accessible to libvirt:

```bash
# Check the image is readable
ls -la /path/to/base-image.qcow2

# For home directory images, use ACLs or copy to /tmp
cp ~/images/ubuntu.qcow2 /tmp/testenv-vm-images/
```

### "Network 'xxx' not found" error

Ensure the network is created before VMs that reference it. The testenv-vm orchestrator handles dependency ordering automatically.

### Cannot connect to libvirt

```bash
# Check libvirtd is running
systemctl status libvirtd

# Test connection
virsh --connect qemu:///session version

# For system mode
virsh --connect qemu:///system version
```

## What base images are supported?

Any QCOW2 image with cloud-init support:

**Ubuntu Cloud Images:**
```bash
wget https://cloud-images.ubuntu.com/releases/noble/release/ubuntu-24.04-server-cloudimg-amd64.img
```

**Debian Cloud Images:**
```bash
wget https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.qcow2
```

**Fedora Cloud Images:**
```bash
wget https://download.fedoraproject.org/pub/fedora/linux/releases/40/Cloud/x86_64/images/Fedora-Cloud-Base-40-1.14.x86_64.qcow2
```

**Requirements:**
- QCOW2 format (or convertible via qemu-img)
- cloud-init installed and enabled
- Network configuration via cloud-init (DHCP)

---

## How does the provider connect to libvirt?

The provider uses the `go-libvirt` library to connect to the libvirt daemon over a Unix socket:

1. Parses the URI (e.g., `qemu:///session`)
2. Connects via `libvirt.ConnectToURI()`
3. Maintains the connection for the provider lifetime
4. Calls libvirt APIs for network/domain operations

The connection URI determines:
- **Session mode**: VMs run as your user, state in user-accessible location
- **System mode**: VMs run as libvirt-qemu, requires elevated permissions

## How are disk images created?

Disk creation uses `qemu-img` with copy-on-write:

```bash
qemu-img create -f qcow2 -b /path/to/base.qcow2 -F qcow2 /path/to/vm-disk.qcow2 20G
```

This creates a thin-provisioned disk that:
- References the base image (no copying)
- Only stores differences from the base
- Can be larger than the base image
- Is automatically cleaned up on VM deletion

## How is IP resolution handled?

The provider uses multiple methods to resolve VM IP addresses:

1. **domifaddr**: Query the VM's network interfaces via QEMU guest agent
2. **net-dhcp-leases**: Check libvirt's DHCP lease database
3. **Polling**: Retry for up to 60 seconds during VM creation

The resolved IP is stored in the VM state and used to generate the SSH command.

## What state is persisted?

The provider maintains state in the state directory:

```
{stateDir}/
├── keys/
│   ├── {keyName}         # Private key (mode 0600)
│   └── {keyName}.pub     # Public key (mode 0644)
├── disks/
│   └── {vmName}.qcow2    # VM disk image
└── cloudinit/
    └── {vmName}.iso      # Cloud-init configuration ISO
```

State is managed in-memory during provider lifetime and cleaned up on resource deletion.

---

## Configuration Reference

### Provider Configuration

```yaml
providers:
  - name: libvirt
    engine: go://github.com/alexandremahdhaoui/testenv-vm/cmd/providers/testenv-vm-provider-libvirt
    default: true          # Use as default provider
    spec: {}               # No provider-level configuration currently
```

### Key Configuration

```yaml
keys:
  - name: string           # Unique key name
    provider: libvirt      # Optional if libvirt is default
    spec:
      type: ed25519|rsa    # Key algorithm (default: ed25519)
      bits: 4096           # RSA key size (ignored for ed25519)
```

### Network Configuration

```yaml
networks:
  - name: string           # Unique network name
    kind: nat|isolated|bridge  # Network type (default: nat)
    provider: libvirt      # Optional if libvirt is default
    spec:
      cidr: "192.168.100.0/24"  # Network CIDR (default: 192.168.100.0/24)
```

### VM Configuration

```yaml
vms:
  - name: string           # Unique VM name
    provider: libvirt      # Optional if libvirt is default
    spec:
      memory: 2048         # Memory in MB (default: 2048)
      vcpus: 2             # Virtual CPUs (default: 2)
      network: string      # Network resource name (required)
      disk:
        baseImage: string  # Path to base QCOW2 image (required)
        size: "20G"        # Disk size (default: 20G)
      cloudInit:
        hostname: string   # VM hostname
        users:
          - name: string   # Username
            sudo: string   # Sudo configuration
            sshAuthorizedKeys:
              - string     # SSH public keys (supports templates)
        packages:
          - string         # Packages to install
        runCommands:
          - string         # Commands to run on first boot
```

---

## Quick Start

Complete example to create a VM with SSH access:

```yaml
# testenv-spec.yaml
providers:
  - name: libvirt
    engine: ./build/bin/testenv-vm-provider-libvirt
    default: true
    spec: {}

keys:
  - name: test-key
    provider: libvirt
    spec:
      type: ed25519

networks:
  - name: test-network
    kind: nat
    provider: libvirt
    spec:
      cidr: "192.168.200.0/24"

vms:
  - name: test-vm
    provider: libvirt
    spec:
      memory: 512
      vcpus: 1
      network: test-network
      disk:
        baseImage: "{{ .Env.TESTENV_VM_BASE_IMAGE }}"
        size: 5G
      cloudInit:
        hostname: test-vm
        users:
          - name: testuser
            sudo: "ALL=(ALL) NOPASSWD:ALL"
            sshAuthorizedKeys:
              - "{{ .Keys.test-key.PublicKey }}"
```

**Run with forge:**

```bash
# Set base image path
export TESTENV_VM_BASE_IMAGE=/tmp/testenv-vm-images/ubuntu-24.04-server-cloudimg-amd64.img

# Create test environment
forge test-create e2e

# SSH into the VM (command from artifact output)
ssh -i /tmp/testenv-vm-1000/keys/test-key -o StrictHostKeyChecking=no testuser@192.168.200.X

# Clean up
forge test-delete e2e
```

---

## License

Apache 2.0
