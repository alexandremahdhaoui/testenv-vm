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

// Package v1 provides the API types for testenv-vm configuration.
package v1

// TestenvSpec is the top-level specification for a test environment.
// It defines the providers, keys, networks, and VMs to be provisioned.
type TestenvSpec struct {
	// StateDir is the directory for persisting environment state.
	StateDir string `json:"stateDir,omitempty"`
	// ArtifactDir is the directory for storing artifacts (keys, logs, etc.).
	ArtifactDir string `json:"artifactDir,omitempty"`
	// CleanupOnFailure determines whether to clean up resources on failure.
	// If nil, defaults to true.
	CleanupOnFailure *bool `json:"cleanupOnFailure,omitempty"`
	// Providers defines the available providers for resource provisioning.
	Providers []ProviderConfig `json:"providers"`
	// DefaultProvider is the name of the default provider to use when not specified.
	DefaultProvider string `json:"defaultProvider,omitempty"`
	// Keys defines SSH key pair resources to create.
	Keys []KeyResource `json:"keys,omitempty"`
	// Networks defines network infrastructure resources to create.
	Networks []NetworkResource `json:"networks,omitempty"`
	// VMs defines virtual machine resources to create.
	VMs []VMResource `json:"vms,omitempty"`
}

// ProviderConfig defines a provider configuration.
// Providers are MCP servers that implement resource provisioning.
type ProviderConfig struct {
	// Name is the unique identifier for this provider.
	Name string `json:"name"`
	// Engine is the path to the provider binary or Go package.
	// Examples: "go://cmd/providers/testenv-vm-provider-stub", "./build/bin/provider"
	Engine string `json:"engine"`
	// Default marks this provider as the default for resources without explicit provider.
	Default bool `json:"default,omitempty"`
	// Spec contains provider-specific configuration passed during initialization.
	Spec map[string]any `json:"spec,omitempty"`
}

// KeyResource defines an SSH key resource.
type KeyResource struct {
	// Name is the unique identifier for this key.
	Name string `json:"name"`
	// Provider is the name of the provider to use. If empty, uses default provider.
	Provider string `json:"provider,omitempty"`
	// Spec contains key-specific configuration.
	Spec KeySpec `json:"spec"`
	// ProviderSpec contains provider-specific configuration.
	ProviderSpec map[string]any `json:"providerSpec,omitempty"`
}

// KeySpec contains key-specific configuration.
type KeySpec struct {
	// Type is the key type: "rsa", "ed25519", "ecdsa".
	Type string `json:"type"`
	// Bits is the key size in bits (for RSA: 2048, 4096; for ECDSA: 256, 384, 521).
	Bits int `json:"bits,omitempty"`
	// Comment is an optional comment for the public key.
	Comment string `json:"comment,omitempty"`
	// OutputDir is the directory to write key files to.
	OutputDir string `json:"outputDir,omitempty"`
}

// NetworkResource defines a network resource.
type NetworkResource struct {
	// Name is the unique identifier for this network.
	Name string `json:"name"`
	// Kind is the network type: "bridge", "libvirt", "dnsmasq", "vpc", "subnet", "security-group".
	Kind string `json:"kind"`
	// Provider is the name of the provider to use. If empty, uses default provider.
	Provider string `json:"provider,omitempty"`
	// Spec contains network-specific configuration.
	Spec NetworkSpec `json:"spec"`
	// ProviderSpec contains provider-specific configuration.
	ProviderSpec map[string]any `json:"providerSpec,omitempty"`
}

// NetworkSpec contains network-specific configuration.
type NetworkSpec struct {
	// CIDR is the network CIDR (e.g., "192.168.100.1/24").
	CIDR string `json:"cidr,omitempty"`
	// Gateway is the gateway IP address.
	Gateway string `json:"gateway,omitempty"`
	// AttachTo references another network resource (for layered networks).
	AttachTo string `json:"attachTo,omitempty"`
	// MTU is the maximum transmission unit size.
	MTU int `json:"mtu,omitempty"`
	// DHCP configures the DHCP server.
	DHCP *DHCPSpec `json:"dhcp,omitempty"`
	// DNS configures DNS forwarding.
	DNS *DNSSpec `json:"dns,omitempty"`
	// TFTP configures TFTP server for PXE boot.
	TFTP *TFTPSpec `json:"tftp,omitempty"`
}

// DHCPSpec configures DHCP server.
type DHCPSpec struct {
	// Enabled enables DHCP.
	Enabled bool `json:"enabled"`
	// RangeStart is the first IP in DHCP range.
	RangeStart string `json:"rangeStart"`
	// RangeEnd is the last IP in DHCP range.
	RangeEnd string `json:"rangeEnd"`
	// LeaseTime duration (e.g., "12h").
	LeaseTime string `json:"leaseTime,omitempty"`
}

// DNSSpec configures DNS forwarding.
type DNSSpec struct {
	// Enabled enables DNS forwarding.
	Enabled bool `json:"enabled"`
	// Servers to forward to.
	Servers []string `json:"servers,omitempty"`
}

// TFTPSpec configures TFTP server for PXE boot.
type TFTPSpec struct {
	// Enabled enables TFTP server.
	Enabled bool `json:"enabled"`
	// Root is the directory for TFTP files.
	Root string `json:"root"`
	// BootFile is the default boot file (e.g., "undionly.kpxe").
	BootFile string `json:"bootFile"`
}

// VMResource defines a VM resource.
type VMResource struct {
	// Name is the unique identifier for this VM.
	Name string `json:"name"`
	// Provider is the name of the provider to use. If empty, uses default provider.
	Provider string `json:"provider,omitempty"`
	// Spec contains VM-specific configuration.
	Spec VMSpec `json:"spec"`
	// ProviderSpec contains provider-specific configuration.
	ProviderSpec map[string]any `json:"providerSpec,omitempty"`
}

// VMSpec contains VM-specific configuration.
type VMSpec struct {
	// Memory in MB.
	Memory int `json:"memory"`
	// VCPUs is the number of virtual CPUs.
	VCPUs int `json:"vcpus"`
	// Disk configures VM disk.
	Disk DiskSpec `json:"disk"`
	// Network is the name of the network resource to attach.
	Network string `json:"network"`
	// CloudInit configures cloud-init.
	CloudInit *CloudInitSpec `json:"cloudInit,omitempty"`
	// Boot configures boot options.
	Boot BootSpec `json:"boot"`
	// Readiness configures readiness checks.
	Readiness *ReadinessSpec `json:"readiness,omitempty"`
}

// DiskSpec configures VM disk.
type DiskSpec struct {
	// BaseImage is the path/URL to base image (QCOW2, AMI, etc.).
	BaseImage string `json:"baseImage,omitempty"`
	// Size is the disk size (e.g., "20G").
	Size string `json:"size"`
}

// CloudInitSpec configures cloud-init.
type CloudInitSpec struct {
	// Hostname for the VM.
	Hostname string `json:"hostname,omitempty"`
	// Users to create.
	Users []UserSpec `json:"users,omitempty"`
	// Packages to install.
	Packages []string `json:"packages,omitempty"`
}

// UserSpec defines a user to create via cloud-init.
type UserSpec struct {
	// Name is the username.
	Name string `json:"name"`
	// Sudo rules (e.g., "ALL=(ALL) NOPASSWD:ALL").
	Sudo string `json:"sudo,omitempty"`
	// SSHAuthorizedKeys are public keys to add to authorized_keys.
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitempty"`
}

// BootSpec configures boot options.
type BootSpec struct {
	// Order is the boot device order: "network", "hd", "cdrom".
	Order []string `json:"order"`
	// Firmware is "bios" or "uefi".
	Firmware string `json:"firmware,omitempty"`
}

// ReadinessSpec configures readiness checks.
type ReadinessSpec struct {
	// SSH configures SSH readiness check.
	SSH *SSHReadinessSpec `json:"ssh,omitempty"`
}

// SSHReadinessSpec configures SSH readiness check.
type SSHReadinessSpec struct {
	// Enabled enables SSH readiness check.
	Enabled bool `json:"enabled"`
	// Timeout for SSH to become available (e.g., "5m").
	Timeout string `json:"timeout"`
	// User for SSH connection.
	User string `json:"user,omitempty"`
	// PrivateKey path (can use template like "{{ .Keys.vm-ssh.PrivateKeyPath }}").
	PrivateKey string `json:"privateKey,omitempty"`
}

// ResourceRef uniquely identifies a resource.
type ResourceRef struct {
	// Kind is the resource type: "vm", "network", "key".
	Kind string `json:"kind"`
	// Name is the user-defined identifier.
	Name string `json:"name"`
	// Provider is the provider that manages this resource.
	Provider string `json:"provider,omitempty"`
}
