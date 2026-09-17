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

package providerv1

type VMCreateRequest struct {
	Name         string         `json:"name"`
	Spec         VMSpec         `json:"spec"`
	ProviderSpec map[string]any `json:"providerSpec,omitempty"`
}

type VMSpec struct {
	Memory        int                `json:"memory"`
	VCPUs         int                `json:"vcpus"`
	Architecture  string             `json:"architecture,omitempty"`
	MachineType   string             `json:"machineType,omitempty"`
	CPU           *CPUSpec           `json:"cpu,omitempty"`
	Disk          DiskSpec           `json:"disk"`
	Network       string             `json:"network,omitempty"`
	Networks      []string           `json:"networks,omitempty"`
	CloudInit     *CloudInitSpec     `json:"cloudInit,omitempty"`
	Cdrom         string             `json:"cdrom,omitempty"`
	Boot          BootSpec           `json:"boot"`
	Console       *ConsoleSpec       `json:"console,omitempty"`
	MemoryBacking *MemoryBackingSpec `json:"memoryBacking,omitempty"`
	VirtioFS      []VirtioFSSpec     `json:"virtioFS,omitempty"`
	GuestAgent    bool               `json:"guestAgent,omitempty"`
	Readiness     *ReadinessSpec     `json:"readiness,omitempty"`
}

type CPUSpec struct {
	Mode    string `json:"mode,omitempty"`
	Model   string `json:"model,omitempty"`
	Cores   int    `json:"cores,omitempty"`
	Sockets int    `json:"sockets,omitempty"`
}

type DiskSpec struct {
	BaseImage string `json:"baseImage,omitempty"`
	Size      string `json:"size"`
	Bus       string `json:"bus,omitempty"`
	WWN       string `json:"wwn,omitempty"`
	Cache     string `json:"cache,omitempty"`
}

type CloudInitSpec struct {
	Hostname      string                  `json:"hostname,omitempty"`
	Users         []UserSpec              `json:"users,omitempty"`
	Packages      []string                `json:"packages,omitempty"`
	WriteFiles    []WriteFileSpec         `json:"writeFiles,omitempty"`
	Runcmd        []string                `json:"runcmd,omitempty"`
	NetworkConfig *CloudInitNetworkConfig `json:"networkConfig,omitempty"`
}

type CloudInitNetworkConfig struct {
	Ethernets []CloudInitEthernetConfig `json:"ethernets,omitempty"`
}

type CloudInitEthernetConfig struct {
	Name        string                `json:"name"`
	DHCP4       *bool                 `json:"dhcp4,omitempty"`
	Addresses   []string              `json:"addresses,omitempty"`
	Gateway4    string                `json:"gateway4,omitempty"`
	Nameservers *CloudInitNameservers `json:"nameservers,omitempty"`
}

type CloudInitNameservers struct {
	Addresses []string `json:"addresses,omitempty"`
}

type UserSpec struct {
	Name              string       `json:"name"`
	Sudo              string       `json:"sudo,omitempty"`
	Shell             string       `json:"shell,omitempty"`
	HomeDir           string       `json:"homeDir,omitempty"`
	SSHAuthorizedKeys []string     `json:"sshAuthorizedKeys,omitempty"`
	SSHKeys           *SSHKeysSpec `json:"sshKeys,omitempty"`
}

type SSHKeysSpec struct {
	RSAPrivate string `json:"rsaPrivate,omitempty"`
	RSAPublic  string `json:"rsaPublic,omitempty"`
}

type WriteFileSpec struct {
	Path        string `json:"path"`
	Content     string `json:"content"`
	Permissions string `json:"permissions,omitempty"`
}

type BootSpec struct {
	Order         []string `json:"order"`
	Firmware      string   `json:"firmware,omitempty"`
	SecureBoot    bool     `json:"secureBoot,omitempty"`
	OVMFPath      string   `json:"ovmfPath,omitempty"`
	NVRAMTemplate string   `json:"nvramTemplate,omitempty"`
}

type ConsoleSpec struct {
	Serial      bool   `json:"serial"`
	VNC         bool   `json:"vnc,omitempty"`
	VNCPort     int    `json:"vncPort,omitempty"`
	VNCPassword string `json:"vncPassword,omitempty"`
	Spice       bool   `json:"spice,omitempty"`
}

type MemoryBackingSpec struct {
	Source string `json:"source,omitempty"`
	Access string `json:"access,omitempty"`
}

type VirtioFSSpec struct {
	Tag      string `json:"tag"`
	HostPath string `json:"hostPath"`
	Queue    int    `json:"queue,omitempty"`
}

type ReadinessSpec struct {
	SSH       *SSHReadinessSpec       `json:"ssh,omitempty"`
	TCP       *TCPReadinessSpec       `json:"tcp,omitempty"`
	CloudInit *CloudInitReadinessSpec `json:"cloudInit,omitempty"`
}

type SSHReadinessSpec struct {
	Enabled    bool   `json:"enabled"`
	Timeout    string `json:"timeout"`
	User       string `json:"user,omitempty"`
	PrivateKey string `json:"privateKey,omitempty"`
}

type TCPReadinessSpec struct {
	Port    int    `json:"port"`
	Address string `json:"address,omitempty"`
	Timeout string `json:"timeout"`
}

type CloudInitReadinessSpec struct {
	Enabled bool   `json:"enabled"`
	Timeout string `json:"timeout"`
}

type VMState struct {
	Name          string            `json:"name"`
	Status        string            `json:"status"`
	IP            string            `json:"ip,omitempty"`
	MAC           string            `json:"mac,omitempty"`
	IPs           map[string]string `json:"ips,omitempty"`
	MACs          map[string]string `json:"macs,omitempty"`
	UUID          string            `json:"uuid,omitempty"`
	ConsoleOutput string            `json:"consoleOutput,omitempty"`
	SSHCommand    string            `json:"sshCommand,omitempty"`
	VNCAddress    string            `json:"vncAddress,omitempty"`
	SerialDevice  string            `json:"serialDevice,omitempty"`
	DomainXML     string            `json:"domainXML,omitempty"`
	QMPSocket     string            `json:"qmpSocket,omitempty"`
	CreatedAt     string            `json:"createdAt,omitempty"`
	ProviderState map[string]any    `json:"providerState,omitempty"`
}

type NetworkCreateRequest struct {
	Name         string         `json:"name"`
	Kind         string         `json:"kind"`
	Spec         NetworkSpec    `json:"spec"`
	ProviderSpec map[string]any `json:"providerSpec,omitempty"`
}

type NetworkSpec struct {
	CIDR     string    `json:"cidr,omitempty"`
	Gateway  string    `json:"gateway,omitempty"`
	AttachTo string    `json:"attachTo,omitempty"`
	MTU      int       `json:"mtu,omitempty"`
	DHCP     *DHCPSpec `json:"dhcp,omitempty"`
	DNS      *DNSSpec  `json:"dns,omitempty"`
	TFTP     *TFTPSpec `json:"tftp,omitempty"`
	IPv6     *IPv6Spec `json:"ipv6,omitempty"`
}

type DHCPSpec struct {
	Enabled      bool          `json:"enabled"`
	RangeStart   string        `json:"rangeStart"`
	RangeEnd     string        `json:"rangeEnd"`
	LeaseTime    string        `json:"leaseTime,omitempty"`
	Router       string        `json:"router,omitempty"`
	DNSServers   []string      `json:"dnsServers,omitempty"`
	Domain       string        `json:"domain,omitempty"`
	NextServer   string        `json:"nextServer,omitempty"`
	StaticLeases []StaticLease `json:"staticLeases,omitempty"`
}

type StaticLease struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname,omitempty"`
}

type DNSSpec struct {
	Enabled bool      `json:"enabled"`
	Servers []string  `json:"servers,omitempty"`
	Hosts   []DNSHost `json:"hosts,omitempty"`
	Domain  string    `json:"domain,omitempty"`
}

type DNSHost struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
}

type TFTPSpec struct {
	Enabled         bool              `json:"enabled"`
	Root            string            `json:"root"`
	BootFile        string            `json:"bootFile"`
	BootFileEFI     string            `json:"bootFileEfi,omitempty"`
	DHCPBootOptions map[string]string `json:"dhcpBootOptions,omitempty"`
}

type IPv6Spec struct {
	CIDR    string `json:"cidr,omitempty"`
	Gateway string `json:"gateway,omitempty"`
	DHCP6   bool   `json:"dhcp6,omitempty"`
	SLAAC   bool   `json:"slaac,omitempty"`
}

type NetworkState struct {
	Name          string         `json:"name"`
	Kind          string         `json:"kind"`
	Status        string         `json:"status"`
	IP            string         `json:"ip,omitempty"`
	CIDR          string         `json:"cidr,omitempty"`
	InterfaceName string         `json:"interfaceName,omitempty"`
	UUID          string         `json:"uuid,omitempty"`
	PID           int            `json:"pid,omitempty"`
	ProviderState map[string]any `json:"providerState,omitempty"`
}

type KeyCreateRequest struct {
	Name         string         `json:"name"`
	Spec         KeySpec        `json:"spec"`
	ProviderSpec map[string]any `json:"providerSpec,omitempty"`
}

type KeySpec struct {
	Type      string `json:"type"`
	Bits      int    `json:"bits,omitempty"`
	Comment   string `json:"comment,omitempty"`
	OutputDir string `json:"outputDir,omitempty"`
}

type KeyState struct {
	Name           string         `json:"name"`
	Type           string         `json:"type"`
	PublicKey      string         `json:"publicKey"`
	PublicKeyPath  string         `json:"publicKeyPath"`
	PrivateKeyPath string         `json:"privateKeyPath"`
	Fingerprint    string         `json:"fingerprint"`
	AWSKeyPairID   string         `json:"awsKeyPairId,omitempty"`
	CreatedAt      string         `json:"createdAt,omitempty"`
	ProviderState  map[string]any `json:"providerState,omitempty"`
}
