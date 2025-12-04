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

	"github.com/digitalocean/go-libvirt"
)

// resolveIP attempts to resolve the IP address for a VM by polling DHCP leases.
// It returns an empty string (not an error) if the IP cannot be resolved within the timeout.
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
		// Get all DHCP leases from the network (pass empty OptString for no MAC filter)
		leases, _, err := conn.NetworkGetDhcpLeases(net, libvirt.OptString{}, 0, 0)
		if err == nil {
			for _, lease := range leases {
				// Extract MAC from OptString
				leaseMAC := ""
				if len(lease.Mac) > 0 {
					leaseMAC = strings.ToLower(lease.Mac[0])
				}
				if leaseMAC == macAddress && lease.Ipaddr != "" {
					return lease.Ipaddr, nil
				}
			}
		}

		// Wait before next poll
		time.Sleep(pollInterval)
	}

	// Timeout reached, return empty string (not an error per design doc)
	return "", nil
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
