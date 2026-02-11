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
	"net"
	"time"

	"github.com/digitalocean/go-libvirt"
)

// waitForVMBoot verifies the VM is making boot progress by checking that CPU time
// advances beyond the initial sample. This detects VMs stuck in BIOS/SeaBIOS.
func waitForVMBoot(conn *libvirt.Libvirt, domain libvirt.Domain, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	// Sample initial CPU time so we detect advancement from that baseline.
	_, _, _, _, initialCPUTime, err := conn.DomainGetInfo(domain)
	if err != nil {
		return fmt.Errorf("failed to query initial domain info: %w", err)
	}

	// Declare lastCPUTime outside the loop so it is accessible in the post-loop error message.
	var lastCPUTime uint64

	// Sleep before first comparison to give the VM time to advance.
	time.Sleep(5 * time.Second)

	for time.Now().Before(deadline) {
		var pollErr error
		_, _, _, _, lastCPUTime, pollErr = conn.DomainGetInfo(domain)
		if pollErr != nil {
			return fmt.Errorf("failed to query domain info: %w", pollErr)
		}

		// 1 second of CPU time advancement since first sample means the VM is booting.
		if lastCPUTime > initialCPUTime+1_000_000_000 {
			return nil
		}

		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("VM not making boot progress: CPU time stuck at %d ns (initial: %d ns) after %v", lastCPUTime, initialCPUTime, timeout)
}

// validateIPReachability performs a TCP probe to verify that the given IP and port are reachable.
func validateIPReachability(ip string, port int, timeout time.Duration) error {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), timeout)
	if err != nil {
		return fmt.Errorf("IP %s port %d not reachable: %w", ip, port, err)
	}
	_ = c.Close()
	return nil
}
