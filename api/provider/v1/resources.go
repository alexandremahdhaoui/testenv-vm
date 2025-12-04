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

// Package providerv1 defines resource types for provider communication.
// These types are used for communication between the testenv-vm orchestrator
// and provider MCP servers.
package providerv1

// VMCreateRequest is the input for vm_create tool.
// It contains all information needed to create a virtual machine.
type VMCreateRequest struct {
	// Name is the unique identifier for this VM.
	Name string `json:"name"`
	// Spec contains the common VM configuration.
	Spec VMSpec `json:"spec"`
	// ProviderSpec contains provider-specific configuration.
	ProviderSpec map[string]any `json:"providerSpec,omitempty"`
}

// VMSpec is the complete VM specification.
// It defines all aspects of VM configuration including compute, storage,
// networking, and boot settings.
type VMSpec struct {
	// Memory in MB.
	Memory int `json:"memory"`
	// VCPUs count.
	VCPUs int `json:"vcpus"`
	// Architecture (x86_64, aarch64) - defaults to x86_64.
	Architecture string `json:"architecture,omitempty"`
	// MachineType (pc-q35-8.0, i440fx, virt) - provider-specific default.
	MachineType string `json:"machineType,omitempty"`
	// CPU configuration.
	CPU *CPUSpec `json:"cpu,omitempty"`
	// Disk configuration.
	Disk DiskSpec `json:"disk"`
	// Network to attach (reference to network resource name).
	Network string `json:"network"`
	// CloudInit configuration.
	CloudInit *CloudInitSpec `json:"cloudInit,omitempty"`
	// Boot configuration.
	Boot BootSpec `json:"boot"`
	// Console access configuration.
	Console *ConsoleSpec `json:"console,omitempty"`
	// MemoryBacking configuration (required for VirtioFS).
	MemoryBacking *MemoryBackingSpec `json:"memoryBacking,omitempty"`
	// VirtioFS shared filesystem mounts.
	VirtioFS []VirtioFSSpec `json:"virtioFS,omitempty"`
	// GuestAgent enables QEMU guest agent.
	GuestAgent bool `json:"guestAgent,omitempty"`
	// Readiness checks.
	Readiness *ReadinessSpec `json:"readiness,omitempty"`
}

// CPUSpec defines CPU configuration for the VM.
type CPUSpec struct {
	// Mode: host-passthrough, host-model, custom.
	Mode string `json:"mode,omitempty"`
	// Model for custom mode (qemu64, Haswell, etc.).
	Model string `json:"model,omitempty"`
	// Cores per socket.
	Cores int `json:"cores,omitempty"`
	// Sockets is the number of CPU sockets.
	Sockets int `json:"sockets,omitempty"`
}

// DiskSpec defines disk configuration for the VM.
type DiskSpec struct {
	// BaseImage is the path/URL to base image (QCOW2, AMI, etc.).
	BaseImage string `json:"baseImage,omitempty"`
	// Size is the disk size (e.g., "20G").
	Size string `json:"size"`
	// Bus is the disk bus type (virtio, scsi, ide) - defaults to virtio.
	Bus string `json:"bus,omitempty"`
	// Cache mode (none, writeback, writethrough) - defaults to none.
	Cache string `json:"cache,omitempty"`
}

// CloudInitSpec defines cloud-init configuration for the VM.
type CloudInitSpec struct {
	// Hostname for the VM.
	Hostname string `json:"hostname,omitempty"`
	// Users to create.
	Users []UserSpec `json:"users,omitempty"`
	// Packages to install.
	Packages []string `json:"packages,omitempty"`
	// WriteFiles defines files to write.
	WriteFiles []WriteFileSpec `json:"writeFiles,omitempty"`
	// RunCommands defines commands to run.
	RunCommands []string `json:"runCommands,omitempty"`
}

// UserSpec defines a user to create via cloud-init.
type UserSpec struct {
	// Name is the username.
	Name string `json:"name"`
	// Sudo rules.
	Sudo string `json:"sudo,omitempty"`
	// Shell path.
	Shell string `json:"shell,omitempty"`
	// HomeDir override.
	HomeDir string `json:"homeDir,omitempty"`
	// SSHAuthorizedKeys are SSH authorized keys for this user.
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitempty"`
	// SSHKeys for user's own key pair (for outbound SSH).
	SSHKeys *SSHKeysSpec `json:"sshKeys,omitempty"`
}

// SSHKeysSpec defines SSH key pair for a user.
type SSHKeysSpec struct {
	// RSAPrivate is the private key content.
	RSAPrivate string `json:"rsaPrivate,omitempty"`
	// RSAPublic is the public key content.
	RSAPublic string `json:"rsaPublic,omitempty"`
}

// WriteFileSpec defines a file to write via cloud-init.
type WriteFileSpec struct {
	// Path is the file path.
	Path string `json:"path"`
	// Content is the file content.
	Content string `json:"content"`
	// Permissions (e.g., "0644").
	Permissions string `json:"permissions,omitempty"`
}

// BootSpec defines boot configuration for the VM.
type BootSpec struct {
	// Order is the boot device order: "network", "hd", "cdrom".
	Order []string `json:"order"`
	// Firmware: "bios" or "uefi".
	Firmware string `json:"firmware,omitempty"`
	// SecureBoot enables UEFI secure boot (requires firmware: uefi).
	SecureBoot bool `json:"secureBoot,omitempty"`
	// OVMFPath is the path to OVMF firmware (for UEFI).
	OVMFPath string `json:"ovmfPath,omitempty"`
	// NVRAMTemplate is the NVRAM template path (for UEFI).
	NVRAMTemplate string `json:"nvramTemplate,omitempty"`
}

// ConsoleSpec defines console access configuration for the VM.
type ConsoleSpec struct {
	// Serial enables serial console.
	Serial bool `json:"serial"`
	// VNC enables VNC access.
	VNC bool `json:"vnc,omitempty"`
	// VNCPort is the VNC port (0 for auto-assign).
	VNCPort int `json:"vncPort,omitempty"`
	// VNCPassword for VNC authentication (optional).
	VNCPassword string `json:"vncPassword,omitempty"`
	// Spice enables SPICE access.
	Spice bool `json:"spice,omitempty"`
}

// MemoryBackingSpec defines memory backing configuration.
// Required for VirtioFS shared filesystems.
type MemoryBackingSpec struct {
	// Source: memfd, file, anonymous.
	Source string `json:"source,omitempty"`
	// Access: shared, private.
	Access string `json:"access,omitempty"`
}

// VirtioFSSpec defines a VirtioFS shared filesystem mount.
type VirtioFSSpec struct {
	// Tag is the guest mount tag.
	Tag string `json:"tag"`
	// HostPath is the host directory to share.
	HostPath string `json:"hostPath"`
	// Queue size (default 1024).
	Queue int `json:"queue,omitempty"`
}

// ReadinessSpec defines readiness check configuration.
type ReadinessSpec struct {
	// SSH readiness check.
	SSH *SSHReadinessSpec `json:"ssh,omitempty"`
	// TCP port readiness check.
	TCP *TCPReadinessSpec `json:"tcp,omitempty"`
	// CloudInit readiness check (wait for cloud-init to complete).
	CloudInit *CloudInitReadinessSpec `json:"cloudInit,omitempty"`
}

// SSHReadinessSpec defines SSH readiness check configuration.
type SSHReadinessSpec struct {
	// Enabled enables SSH readiness check.
	Enabled bool `json:"enabled"`
	// Timeout for SSH to become available.
	Timeout string `json:"timeout"`
	// User for SSH connection.
	User string `json:"user,omitempty"`
	// PrivateKey path (can use template).
	PrivateKey string `json:"privateKey,omitempty"`
}

// TCPReadinessSpec defines TCP port readiness check configuration.
type TCPReadinessSpec struct {
	// Port to check.
	Port int `json:"port"`
	// Timeout for port to become available.
	Timeout string `json:"timeout"`
}

// CloudInitReadinessSpec defines cloud-init completion readiness check.
type CloudInitReadinessSpec struct {
	// Enabled enables cloud-init completion check.
	Enabled bool `json:"enabled"`
	// Timeout for cloud-init to complete.
	Timeout string `json:"timeout"`
}

// VMState is the VM resource state returned by operations.
type VMState struct {
	// Name is the VM identifier.
	Name string `json:"name"`
	// Status: creating, running, stopped, failed, destroyed.
	Status string `json:"status"`
	// IP is the assigned IP address (if available).
	IP string `json:"ip,omitempty"`
	// MAC is the MAC address.
	MAC string `json:"mac,omitempty"`
	// UUID is the provider-specific unique ID.
	UUID string `json:"uuid,omitempty"`
	// ConsoleOutput path to console log file.
	ConsoleOutput string `json:"consoleOutput,omitempty"`
	// SSHCommand to connect to this VM.
	SSHCommand string `json:"sshCommand,omitempty"`
	// VNCAddress for VNC connection.
	VNCAddress string `json:"vncAddress,omitempty"`
	// SerialDevice path for serial console.
	SerialDevice string `json:"serialDevice,omitempty"`
	// DomainXML is the full libvirt domain XML (for debugging).
	DomainXML string `json:"domainXML,omitempty"`
	// QMPSocket path (for QEMU provider direct control).
	QMPSocket string `json:"qmpSocket,omitempty"`
	// CreatedAt timestamp.
	CreatedAt string `json:"createdAt,omitempty"`
	// ProviderState contains provider-specific state.
	ProviderState map[string]any `json:"providerState,omitempty"`
}

// NetworkCreateRequest is the input for network_create tool.
type NetworkCreateRequest struct {
	// Name is the unique identifier for this network.
	Name string `json:"name"`
	// Kind is the network type (provider-specific).
	// Examples: "bridge", "libvirt", "dnsmasq", "vpc", "subnet", "security-group".
	Kind string `json:"kind"`
	// Spec contains the common network configuration.
	Spec NetworkSpec `json:"spec"`
	// ProviderSpec contains provider-specific configuration.
	ProviderSpec map[string]any `json:"providerSpec,omitempty"`
}

// NetworkSpec is the network specification.
type NetworkSpec struct {
	// CIDR is the network CIDR (e.g., "192.168.100.1/24").
	CIDR string `json:"cidr,omitempty"`
	// Gateway IP address.
	Gateway string `json:"gateway,omitempty"`
	// AttachTo references another network resource (for layered networks).
	AttachTo string `json:"attachTo,omitempty"`
	// MTU size (default 1500).
	MTU int `json:"mtu,omitempty"`
	// DHCP configuration.
	DHCP *DHCPSpec `json:"dhcp,omitempty"`
	// DNS configuration.
	DNS *DNSSpec `json:"dns,omitempty"`
	// TFTP configuration (for PXE boot).
	TFTP *TFTPSpec `json:"tftp,omitempty"`
	// IPv6 configuration.
	IPv6 *IPv6Spec `json:"ipv6,omitempty"`
}

// DHCPSpec defines DHCP configuration for a network.
type DHCPSpec struct {
	// Enabled enables DHCP.
	Enabled bool `json:"enabled"`
	// RangeStart is the first IP in DHCP range.
	RangeStart string `json:"rangeStart"`
	// RangeEnd is the last IP in DHCP range.
	RangeEnd string `json:"rangeEnd"`
	// LeaseTime duration (e.g., "12h").
	LeaseTime string `json:"leaseTime,omitempty"`
	// Router IP to advertise (defaults to gateway).
	Router string `json:"router,omitempty"`
	// DNSServers to advertise.
	DNSServers []string `json:"dnsServers,omitempty"`
	// Domain to advertise.
	Domain string `json:"domain,omitempty"`
	// NextServer for PXE (TFTP server address).
	NextServer string `json:"nextServer,omitempty"`
	// StaticLeases for specific MAC addresses.
	StaticLeases []StaticLease `json:"staticLeases,omitempty"`
}

// StaticLease defines a static DHCP lease.
type StaticLease struct {
	// MAC address.
	MAC string `json:"mac"`
	// IP address.
	IP string `json:"ip"`
	// Hostname for the lease.
	Hostname string `json:"hostname,omitempty"`
}

// DNSSpec defines DNS configuration for a network.
type DNSSpec struct {
	// Enabled enables DNS forwarding.
	Enabled bool `json:"enabled"`
	// Servers to forward to.
	Servers []string `json:"servers,omitempty"`
	// Hosts for local DNS entries.
	Hosts []DNSHost `json:"hosts,omitempty"`
	// Domain for local DNS.
	Domain string `json:"domain,omitempty"`
}

// DNSHost defines a local DNS entry.
type DNSHost struct {
	// Hostname for the DNS entry.
	Hostname string `json:"hostname"`
	// IP address for the DNS entry.
	IP string `json:"ip"`
}

// TFTPSpec defines TFTP configuration for PXE boot.
type TFTPSpec struct {
	// Enabled enables TFTP server.
	Enabled bool `json:"enabled"`
	// Root directory for TFTP files.
	Root string `json:"root"`
	// BootFile is the default boot file for BIOS (e.g., "undionly.kpxe").
	BootFile string `json:"bootFile"`
	// BootFileEFI for UEFI clients (e.g., "ipxe.efi").
	BootFileEFI string `json:"bootFileEfi,omitempty"`
	// DHCPBootOptions for vendor-specific options (option 43, etc.).
	DHCPBootOptions map[string]string `json:"dhcpBootOptions,omitempty"`
}

// IPv6Spec defines IPv6 configuration for a network.
type IPv6Spec struct {
	// CIDR is the IPv6 network CIDR.
	CIDR string `json:"cidr,omitempty"`
	// Gateway is the IPv6 gateway.
	Gateway string `json:"gateway,omitempty"`
	// DHCP6 enables DHCPv6.
	DHCP6 bool `json:"dhcp6,omitempty"`
	// SLAAC enables stateless address autoconfiguration.
	SLAAC bool `json:"slaac,omitempty"`
}

// NetworkState is the network resource state.
type NetworkState struct {
	// Name is the network identifier.
	Name string `json:"name"`
	// Kind is the network type.
	Kind string `json:"kind"`
	// Status: creating, ready, failed, destroyed.
	Status string `json:"status"`
	// IP is the gateway/interface IP.
	IP string `json:"ip,omitempty"`
	// CIDR is the network range.
	CIDR string `json:"cidr,omitempty"`
	// InterfaceName is the OS-level interface (for bridges).
	InterfaceName string `json:"interfaceName,omitempty"`
	// UUID is the libvirt network UUID.
	UUID string `json:"uuid,omitempty"`
	// PID for dnsmasq process.
	PID int `json:"pid,omitempty"`
	// ProviderState contains provider-specific state.
	ProviderState map[string]any `json:"providerState,omitempty"`
}

// KeyCreateRequest is the input for key_create tool.
type KeyCreateRequest struct {
	// Name is the unique identifier for this key pair.
	Name string `json:"name"`
	// Spec contains the key configuration.
	Spec KeySpec `json:"spec"`
	// ProviderSpec contains provider-specific configuration.
	ProviderSpec map[string]any `json:"providerSpec,omitempty"`
}

// KeySpec is the key specification.
type KeySpec struct {
	// Type is the key algorithm: rsa, ed25519, ecdsa.
	Type string `json:"type"`
	// Bits is the key size (for RSA: 2048, 4096; ignored for ed25519).
	Bits int `json:"bits,omitempty"`
	// Comment is added to the public key.
	Comment string `json:"comment,omitempty"`
	// OutputDir for key file storage (defaults to artifact dir).
	OutputDir string `json:"outputDir,omitempty"`
}

// KeyState is the key resource state.
type KeyState struct {
	// Name is the key identifier.
	Name string `json:"name"`
	// Type is the key algorithm.
	Type string `json:"type"`
	// PublicKey is the public key content.
	PublicKey string `json:"publicKey"`
	// PublicKeyPath is the file path to public key.
	PublicKeyPath string `json:"publicKeyPath"`
	// PrivateKeyPath is the file path to private key.
	PrivateKeyPath string `json:"privateKeyPath"`
	// Fingerprint is the key fingerprint.
	Fingerprint string `json:"fingerprint"`
	// AWSKeyPairID for AWS-managed keys (if applicable).
	AWSKeyPairID string `json:"awsKeyPairId,omitempty"`
	// CreatedAt timestamp.
	CreatedAt string `json:"createdAt,omitempty"`
	// ProviderState contains provider-specific state.
	ProviderState map[string]any `json:"providerState,omitempty"`
}
