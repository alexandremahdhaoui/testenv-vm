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

package libvirt

import (
	"fmt"
	"strings"
	"time"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
	"github.com/digitalocean/go-libvirt"
)

// resolveIP attempts to resolve the IP address for a VM by polling DHCP leases.
// It returns an error if the IP cannot be resolved within the timeout.
func resolveIP(conn *libvirt.Libvirt, networkName, macAddress string, timeout time.Duration) (string, error) {
	// Look up the network
	net, err := conn.NetworkLookupByName(networkName)
	if err != nil {
		return "", fmt.Errorf("network not found: %s", networkName)
	}

	// Normalize MAC address for comparison (lowercase)
	macAddress = strings.ToLower(macAddress)

	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		// Strategy 1: Check DHCP leases (works when network has DHCP enabled)
		leases, _, err := conn.NetworkGetDhcpLeases(net, libvirt.OptString{}, 0, 0)
		if err == nil {
			for _, lease := range leases {
				leaseMAC := ""
				if len(lease.Mac) > 0 {
					leaseMAC = strings.ToLower(lease.Mac[0])
				}
				if leaseMAC == macAddress && lease.Ipaddr != "" {
					return lease.Ipaddr, nil
				}
			}
		}

		time.Sleep(pollInterval)
	}

	// Timeout reached, return error
	return "", fmt.Errorf("DHCP lease not found for MAC %s on network %s within %v", macAddress, networkName, timeout)
}

// resolveIPFromARP queries the host ARP table for the domain's IP address.
// This resolves IPs for VMs with static network configurations where DHCP is
// not available. Returns empty string if no IP is found.
func resolveIPFromARP(conn *libvirt.Libvirt, dom libvirt.Domain) string {
	ifaces, err := conn.DomainInterfaceAddresses(dom, uint32(libvirt.DomainInterfaceAddressesSrcArp), 0)
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		for _, addr := range iface.Addrs {
			if addr.Addr != "" {
				return addr.Addr
			}
		}
	}
	return ""
}

// extractStaticIP extracts the first static IP address from CloudInit network
// configuration. This is used as a last-resort fallback when neither DHCP leases
// nor ARP resolution can provide the IP (e.g., the VM hasn't generated any
// network traffic yet but has a known static IP).
func extractStaticIP(ci *providerv1.CloudInitSpec) string {
	if ci == nil || ci.NetworkConfig == nil {
		return ""
	}
	for _, eth := range ci.NetworkConfig.Ethernets {
		for _, addr := range eth.Addresses {
			// Addresses are in CIDR notation (e.g., "192.168.100.10/24")
			ip, _, _ := strings.Cut(addr, "/")
			if ip != "" {
				return ip
			}
		}
	}
	return ""
}

// extractMACFromDomainXML extracts the MAC address from a domain's XML.
// This is a simple extraction that looks for the first MAC address in a virtio interface.
func extractMACFromDomainXML(xml string) string {
	// Simple extraction - look for mac address="XX:XX:XX:XX:XX:XX" pattern
	// This is a basic implementation; a more robust one would use XML parsing
	macStart := strings.Index(xml, "<mac address='")
	if macStart == -1 {
		return ""
	}
	macStart += len("<mac address='")
	macEnd := strings.Index(xml[macStart:], "'")
	if macEnd == -1 {
		return ""
	}
	return xml[macStart : macStart+macEnd]
}
