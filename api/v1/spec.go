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
	StateDir string `json:"stateDir,omitempty" yaml:"stateDir,omitempty"`
	// ArtifactDir is the directory for storing artifacts (keys, logs, etc.).
	ArtifactDir string `json:"artifactDir,omitempty" yaml:"artifactDir,omitempty"`
	// CleanupOnFailure determines whether to clean up resources on failure.
	// If nil, defaults to true.
	CleanupOnFailure *bool `json:"cleanupOnFailure,omitempty" yaml:"cleanupOnFailure,omitempty"`
	// ImageCacheDir is the directory for caching downloaded VM base images.
	// If empty, defaults to TESTENV_VM_IMAGE_CACHE_DIR env var or /tmp/testenv-vm/images/.
	ImageCacheDir string `json:"imageCacheDir,omitempty" yaml:"imageCacheDir,omitempty"`
	// DefaultBaseImage is the default base image to use for VMs.
	// Can be a well-known reference (e.g., "ubuntu:24.04") or an HTTPS URL.
	// Accessible via {{ .DefaultBaseImage }} template variable.
	DefaultBaseImage string `json:"defaultBaseImage,omitempty" yaml:"defaultBaseImage,omitempty"`
	// Images defines VM base images to download and cache.
	// Images are downloaded before any other resources (Phase 0).
	Images []ImageResource `json:"images,omitempty" yaml:"images,omitempty"`
	// Providers defines the available providers for resource provisioning.
	Providers []ProviderConfig `json:"providers" yaml:"providers"`
	// DefaultProvider is the name of the default provider to use when not specified.
	DefaultProvider string `json:"defaultProvider,omitempty" yaml:"defaultProvider,omitempty"`
	// Keys defines SSH key pair resources to create.
	Keys []KeyResource `json:"keys,omitempty" yaml:"keys,omitempty"`
	// Networks defines network infrastructure resources to create.
	Networks []NetworkResource `json:"networks,omitempty" yaml:"networks,omitempty"`
	// VMs defines virtual machine resources to create.
	VMs []VMResource `json:"vms,omitempty" yaml:"vms,omitempty"`
}

// ProviderConfig defines a provider configuration.
// Providers are MCP servers that implement resource provisioning.
type ProviderConfig struct {
	// Name is the unique identifier for this provider.
	Name string `json:"name" yaml:"name"`
	// Engine is the path to the provider binary or Go package.
	// Examples: "go://cmd/providers/testenv-vm-provider-stub", "./build/bin/provider"
	Engine string `json:"engine" yaml:"engine"`
	// Default marks this provider as the default for resources without explicit provider.
	Default bool `json:"default,omitempty" yaml:"default,omitempty"`
	// Spec contains provider-specific configuration passed during initialization.
	Spec map[string]any `json:"spec,omitempty" yaml:"spec,omitempty"`
}

// ImageResource defines a VM base image resource.
// Images are downloaded and cached by the orchestrator (not by providers).
type ImageResource struct {
	// Name is the unique identifier for this image.
	Name string `json:"name" yaml:"name"`
	// Spec contains image-specific configuration.
	Spec ImageSpec `json:"spec" yaml:"spec"`
}

// ImageSpec contains image-specific configuration.
type ImageSpec struct {
	// Source is the image source: either a well-known reference (e.g., "ubuntu:24.04")
	// or an HTTPS URL to a cloud image file.
	Source string `json:"source" yaml:"source"`
	// SHA256 is the expected SHA256 checksum of the image file.
	// Required for custom HTTPS URLs; optional for well-known images.
	SHA256 string `json:"sha256,omitempty" yaml:"sha256,omitempty"`
	// Alias is an optional alternative name for template references.
	// If set, the image can be accessed via {{ .Images.<alias>.Path }}.
	Alias string `json:"alias,omitempty" yaml:"alias,omitempty"`
}

// KeyResource defines an SSH key resource.
type KeyResource struct {
	// Name is the unique identifier for this key.
	Name string `json:"name" yaml:"name"`
	// Provider is the name of the provider to use. If empty, uses default provider.
	Provider string `json:"provider,omitempty" yaml:"provider,omitempty"`
	// Spec contains key-specific configuration.
	Spec KeySpec `json:"spec" yaml:"spec"`
	// ProviderSpec contains provider-specific configuration.
	ProviderSpec map[string]any `json:"providerSpec,omitempty" yaml:"providerSpec,omitempty"`
}

// KeySpec contains key-specific configuration.
type KeySpec struct {
	// Type is the key type: "rsa", "ed25519", "ecdsa".
	Type string `json:"type" yaml:"type"`
	// Bits is the key size in bits (for RSA: 2048, 4096; for ECDSA: 256, 384, 521).
	Bits int `json:"bits,omitempty" yaml:"bits,omitempty"`
	// Comment is an optional comment for the public key.
	Comment string `json:"comment,omitempty" yaml:"comment,omitempty"`
	// OutputDir is the directory to write key files to.
	OutputDir string `json:"outputDir,omitempty" yaml:"outputDir,omitempty"`
}

// NetworkResource defines a network resource.
type NetworkResource struct {
	// Name is the unique identifier for this network.
	Name string `json:"name" yaml:"name"`
	// Kind is the network type: "bridge", "libvirt", "dnsmasq", "vpc", "subnet", "security-group".
	Kind string `json:"kind" yaml:"kind"`
	// Provider is the name of the provider to use. If empty, uses default provider.
	Provider string `json:"provider,omitempty" yaml:"provider,omitempty"`
	// Spec contains network-specific configuration.
	Spec NetworkSpec `json:"spec" yaml:"spec"`
	// ProviderSpec contains provider-specific configuration.
	ProviderSpec map[string]any `json:"providerSpec,omitempty" yaml:"providerSpec,omitempty"`
}

// NetworkSpec contains network-specific configuration.
type NetworkSpec struct {
	// CIDR is the network CIDR (e.g., "192.168.100.1/24").
	CIDR string `json:"cidr,omitempty" yaml:"cidr,omitempty"`
	// Gateway is the gateway IP address.
	Gateway string `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	// AttachTo references another network resource (for layered networks).
	AttachTo string `json:"attachTo,omitempty" yaml:"attachTo,omitempty"`
	// MTU is the maximum transmission unit size.
	MTU int `json:"mtu,omitempty" yaml:"mtu,omitempty"`
	// DHCP configures the DHCP server.
	DHCP *DHCPSpec `json:"dhcp,omitempty" yaml:"dhcp,omitempty"`
	// DNS configures DNS forwarding.
	DNS *DNSSpec `json:"dns,omitempty" yaml:"dns,omitempty"`
	// TFTP configures TFTP server for PXE boot.
	TFTP *TFTPSpec `json:"tftp,omitempty" yaml:"tftp,omitempty"`
}

// DHCPSpec configures DHCP server.
type DHCPSpec struct {
	// Enabled enables DHCP.
	Enabled bool `json:"enabled" yaml:"enabled"`
	// RangeStart is the first IP in DHCP range.
	RangeStart string `json:"rangeStart" yaml:"rangeStart"`
	// RangeEnd is the last IP in DHCP range.
	RangeEnd string `json:"rangeEnd" yaml:"rangeEnd"`
	// LeaseTime duration (e.g., "12h").
	LeaseTime string `json:"leaseTime,omitempty" yaml:"leaseTime,omitempty"`
}

// DNSSpec configures DNS forwarding.
type DNSSpec struct {
	// Enabled enables DNS forwarding.
	Enabled bool `json:"enabled" yaml:"enabled"`
	// Servers to forward to.
	Servers []string `json:"servers,omitempty" yaml:"servers,omitempty"`
}

// TFTPSpec configures TFTP server for PXE boot.
type TFTPSpec struct {
	// Enabled enables TFTP server.
	Enabled bool `json:"enabled" yaml:"enabled"`
	// Root is the directory for TFTP files.
	Root string `json:"root" yaml:"root"`
	// BootFile is the default boot file (e.g., "undionly.kpxe").
	BootFile string `json:"bootFile" yaml:"bootFile"`
}

// VMResource defines a VM resource.
type VMResource struct {
	// Name is the unique identifier for this VM.
	Name string `json:"name" yaml:"name"`
	// Provider is the name of the provider to use. If empty, uses default provider.
	Provider string `json:"provider,omitempty" yaml:"provider,omitempty"`
	// Spec contains VM-specific configuration.
	Spec VMSpec `json:"spec" yaml:"spec"`
	// ProviderSpec contains provider-specific configuration.
	ProviderSpec map[string]any `json:"providerSpec,omitempty" yaml:"providerSpec,omitempty"`
}

// VMSpec contains VM-specific configuration.
type VMSpec struct {
	// Memory in MB.
	Memory int `json:"memory" yaml:"memory"`
	// VCPUs is the number of virtual CPUs.
	VCPUs int `json:"vcpus" yaml:"vcpus"`
	// Disk configures VM disk.
	Disk DiskSpec `json:"disk" yaml:"disk"`
	// Network is the name of the network resource to attach.
	Network string `json:"network" yaml:"network"`
	// CloudInit configures cloud-init.
	CloudInit *CloudInitSpec `json:"cloudInit,omitempty" yaml:"cloudInit,omitempty"`
	// Boot configures boot options.
	Boot BootSpec `json:"boot" yaml:"boot"`
	// Readiness configures readiness checks.
	Readiness *ReadinessSpec `json:"readiness,omitempty" yaml:"readiness,omitempty"`
}

// DiskSpec configures VM disk.
type DiskSpec struct {
	// BaseImage is the path/URL to base image (QCOW2, AMI, etc.).
	BaseImage string `json:"baseImage,omitempty" yaml:"baseImage,omitempty"`
	// Size is the disk size (e.g., "20G").
	Size string `json:"size" yaml:"size"`
}

// CloudInitSpec configures cloud-init.
type CloudInitSpec struct {
	// Hostname for the VM.
	Hostname string `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	// Users to create.
	Users []UserSpec `json:"users,omitempty" yaml:"users,omitempty"`
	// Packages to install.
	Packages []string `json:"packages,omitempty" yaml:"packages,omitempty"`
	// WriteFiles configures files to write.
	WriteFiles []WriteFileSpec `json:"writeFiles,omitempty" yaml:"writeFiles,omitempty"`
	// Runcmd configures commands to run.
	Runcmd []string `json:"runcmd,omitempty" yaml:"runcmd,omitempty"`
	// NetworkConfig configures the network via cloud-init's network-config.
	// If nil, uses DHCP on all ethernet interfaces.
	NetworkConfig *CloudInitNetworkConfig `json:"networkConfig,omitempty" yaml:"networkConfig,omitempty"`
}

// CloudInitNetworkConfig configures cloud-init network settings.
// Uses netplan version 2 format.
type CloudInitNetworkConfig struct {
	// Ethernets configures ethernet interfaces.
	Ethernets []CloudInitEthernetConfig `json:"ethernets,omitempty" yaml:"ethernets,omitempty"`
}

// CloudInitEthernetConfig configures a single ethernet interface.
type CloudInitEthernetConfig struct {
	// Name is the interface name pattern (e.g., "ens2", "en*", "eth*").
	// Supports wildcards for matching.
	Name string `json:"name" yaml:"name"`
	// DHCP4 enables DHCP for IPv4 if true. Defaults to false if Addresses is set.
	DHCP4 *bool `json:"dhcp4,omitempty" yaml:"dhcp4,omitempty"`
	// Addresses is a list of static IP addresses in CIDR notation (e.g., "192.168.100.2/24").
	Addresses []string `json:"addresses,omitempty" yaml:"addresses,omitempty"`
	// Gateway4 is the IPv4 gateway address.
	Gateway4 string `json:"gateway4,omitempty" yaml:"gateway4,omitempty"`
	// Nameservers configures DNS servers.
	Nameservers *CloudInitNameservers `json:"nameservers,omitempty" yaml:"nameservers,omitempty"`
}

// CloudInitNameservers configures DNS servers.
type CloudInitNameservers struct {
	// Addresses is a list of DNS server IP addresses.
	Addresses []string `json:"addresses,omitempty" yaml:"addresses,omitempty"`
}

// WriteFileSpec configures a file to write via cloud-init.
type WriteFileSpec struct {
	// Path is the file path.
	Path string `json:"path" yaml:"path"`
	// Content is the file content.
	Content string `json:"content" yaml:"content"`
	// Permissions is the file permissions (e.g., "0644").
	Permissions string `json:"permissions,omitempty" yaml:"permissions,omitempty"`
}

// UserSpec defines a user to create via cloud-init.
type UserSpec struct {
	// Name is the username.
	Name string `json:"name" yaml:"name"`
	// Sudo rules (e.g., "ALL=(ALL) NOPASSWD:ALL").
	Sudo string `json:"sudo,omitempty" yaml:"sudo,omitempty"`
	// SSHAuthorizedKeys are public keys to add to authorized_keys.
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitempty" yaml:"sshAuthorizedKeys,omitempty"`
}

// BootSpec configures boot options.
type BootSpec struct {
	// Order is the boot device order: "network", "hd", "cdrom".
	Order []string `json:"order" yaml:"order"`
	// Firmware is "bios" or "uefi".
	Firmware string `json:"firmware,omitempty" yaml:"firmware,omitempty"`
}

// ReadinessSpec configures readiness checks.
type ReadinessSpec struct {
	// SSH configures SSH readiness check.
	SSH *SSHReadinessSpec `json:"ssh,omitempty" yaml:"ssh,omitempty"`
}

// SSHReadinessSpec configures SSH readiness check.
type SSHReadinessSpec struct {
	// Enabled enables SSH readiness check.
	Enabled bool `json:"enabled" yaml:"enabled"`
	// Timeout for SSH to become available (e.g., "5m").
	Timeout string `json:"timeout" yaml:"timeout"`
	// User for SSH connection.
	User string `json:"user,omitempty" yaml:"user,omitempty"`
	// PrivateKey path (can use template like "{{ .Keys.vm-ssh.PrivateKeyPath }}").
	PrivateKey string `json:"privateKey,omitempty" yaml:"privateKey,omitempty"`
}

// ResourceRef uniquely identifies a resource.
type ResourceRef struct {
	// Kind is the resource type: "vm", "network", "key".
	Kind string `json:"kind" yaml:"kind"`
	// Name is the user-defined identifier.
	Name string `json:"name" yaml:"name"`
	// Provider is the provider that manages this resource.
	Provider string `json:"provider,omitempty" yaml:"provider,omitempty"`
}
