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
	"os"
	"path/filepath"
	"time"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
)

// VMCreate creates a VM via libvirt.
// This function is idempotent: if a VM with the same name already exists
// in libvirt (e.g., from a previous failed run), it will be cleaned up first.
func (p *Provider) VMCreate(req *providerv1.VMCreateRequest) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.vms[req.Name]; exists {
		return providerv1.ErrorResult(providerv1.NewAlreadyExistsError("vm", req.Name))
	}

	// Check if domain already exists in libvirt (orphaned from previous run)
	// If so, clean it up to ensure idempotent behavior
	if existingDom, err := p.conn.DomainLookupByName(req.Name); err == nil {
		// Domain exists in libvirt but not in our state - clean it up
		_ = p.conn.DomainDestroy(existingDom)
		_ = p.conn.DomainUndefine(existingDom)
	}

	// Get network name from spec
	networkName := req.Spec.Network
	if networkName == "" {
		return providerv1.ErrorResult(providerv1.NewInvalidSpecError("VM requires a network"))
	}

	// Verify network exists
	if _, exists := p.networks[networkName]; !exists {
		return providerv1.ErrorResult(providerv1.NewNotFoundError("network", networkName))
	}

	// Track created resources for rollback
	var diskPath, isoPath string
	var cleanupFuncs []func()
	defer func() {
		// Execute cleanup in reverse order if we exit with an error
		for i := len(cleanupFuncs) - 1; i >= 0; i-- {
			cleanupFuncs[i]()
		}
	}()

	// Create disk image
	diskPath = filepath.Join(p.config.StateDir, "disks", req.Name+".qcow2")
	baseImage := req.Spec.Disk.BaseImage
	diskSize := req.Spec.Disk.Size
	if diskSize == "" {
		diskSize = "20G"
	}

	if err := createDisk(baseImage, diskPath, diskSize, p.config.QemuImgPath); err != nil {
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to create disk: "+err.Error(), false))
	}
	cleanupFuncs = append(cleanupFuncs, func() { _ = os.Remove(diskPath) })

	// Generate cloud-init ISO
	isoPath = filepath.Join(p.config.StateDir, "cloudinit", req.Name+".iso")
	ciConfig := cloudInitConfigFromVMSpec(req.Name, &req.Spec, p.keys)
	if err := generateCloudInitISO(ciConfig, isoPath, p.config.ISOTool); err != nil {
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to generate cloud-init ISO: "+err.Error(), false))
	}
	cleanupFuncs = append(cleanupFuncs, func() { _ = os.Remove(isoPath) })

	// Build domain config
	memoryMB := 2048
	vcpu := 2
	if req.Spec.Memory > 0 {
		memoryMB = req.Spec.Memory
	}
	if req.Spec.VCPUs > 0 {
		vcpu = req.Spec.VCPUs
	}

	// Check if network boot is enabled
	hasNetworkBoot := false
	for _, dev := range req.Spec.Boot.Order {
		if dev == "network" {
			hasNetworkBoot = true
			break
		}
	}

	domainConfig := DomainConfig{
		Name:           req.Name,
		MemoryMB:       memoryMB,
		VCPU:           vcpu,
		DiskPath:       diskPath,
		CloudInitISO:   isoPath,
		NetworkName:    networkName,
		BootOrder:      req.Spec.Boot.Order,
		Firmware:       req.Spec.Boot.Firmware,
		HasNetworkBoot: hasNetworkBoot,
	}

	// Generate domain XML
	domainXML, err := generateDomainXML(domainConfig)
	if err != nil {
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to generate domain XML: "+err.Error(), false))
	}

	// Create and start the domain
	dom, err := p.conn.DomainCreateXML(domainXML, 0)
	if err != nil {
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to create domain: "+err.Error(), true))
	}

	// Domain created successfully, clear cleanup funcs
	cleanupFuncs = nil

	// Get domain XML to extract MAC address
	xmlDesc, err := p.conn.DomainGetXMLDesc(dom, 0)
	if err != nil {
		// Non-fatal: continue without MAC
		xmlDesc = ""
	}
	mac := extractMACFromDomainXML(xmlDesc)

	// Determine whether strict readiness checks are required
	sshReadiness := req.Spec.Readiness != nil && req.Spec.Readiness.SSH != nil
	ipTimeout := 60 * time.Second // best-effort default
	if sshReadiness {
		ipTimeout = 3 * time.Minute // strict default
		if req.Spec.Readiness.SSH.Timeout != "" {
			if d, err := time.ParseDuration(req.Spec.Readiness.SSH.Timeout); err == nil {
				ipTimeout = d
			}
		}
	}

	// Wait for VM boot (60s fixed budget, capped to half of total)
	bootTimeout := 60 * time.Second
	if bootTimeout > ipTimeout {
		bootTimeout = ipTimeout / 2
	}
	bootStart := time.Now()
	if err := waitForVMBoot(p.conn, dom, bootTimeout); err != nil {
		if sshReadiness {
			return providerv1.ErrorResult(providerv1.NewProviderError(
				fmt.Sprintf("VM %s failed boot check: %s", req.Name, err.Error()), true))
		}
		// Best-effort: log and continue without boot verification
	}

	// Resolve IP with remaining budget (total minus time already spent on boot check)
	remaining := ipTimeout - time.Since(bootStart)
	if remaining < 30*time.Second {
		remaining = 30 * time.Second // minimum 30s for DHCP
	}
	ip, err := resolveIP(p.conn, networkName, mac, remaining)
	if sshReadiness {
		if err != nil {
			return providerv1.ErrorResult(providerv1.NewTimeoutError("ip-resolution"))
		}
		if ip == "" {
			return providerv1.ErrorResult(providerv1.NewProviderError(
				fmt.Sprintf("VM %s: resolved empty IP without error", req.Name), true))
		}
		// Validate IP reachability via TCP probe to SSH port
		if err := validateIPReachability(ip, 22, 10*time.Second); err != nil {
			return providerv1.ErrorResult(providerv1.NewProviderError(
				fmt.Sprintf("VM %s IP %s not reachable: %s", req.Name, ip, err.Error()), true))
		}
	} else {
		// Best-effort: use resolved IP if available, empty string otherwise
		if err != nil {
			ip = ""
		}
	}

	// Generate SSH command if we have an IP and matched keys
	sshCommand := ""
	username := "ubuntu" // default username
	if len(ciConfig.Users) > 0 {
		username = ciConfig.Users[0].Name
	}
	if ip != "" && username != "" && len(ciConfig.MatchedKeyNames) > 0 {
		// Use the first matched key for the SSH command
		firstKeyName := ciConfig.MatchedKeyNames[0]
		if key, exists := p.keys[firstKeyName]; exists {
			sshCommand = fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no %s@%s",
				key.PrivateKeyPath, username, ip)
		}
	}

	state := &providerv1.VMState{
		Name:       req.Name,
		Status:     "running",
		IP:         ip,
		MAC:        mac,
		UUID:       formatUUID(dom.UUID),
		SSHCommand: sshCommand,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		ProviderState: map[string]any{
			"diskPath":     diskPath,
			"cloudInitISO": isoPath,
			"network":      networkName,
			"keys":         ciConfig.MatchedKeyNames,
		},
	}

	p.vms[req.Name] = state
	return providerv1.SuccessResult(state)
}

// VMGet retrieves a VM by name.
func (p *Provider) VMGet(name string) *providerv1.OperationResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	vm, exists := p.vms[name]
	if !exists {
		return providerv1.ErrorResult(providerv1.NewNotFoundError("vm", name))
	}

	return providerv1.SuccessResult(vm)
}

// VMList lists all VMs.
func (p *Provider) VMList(filter map[string]any) *providerv1.OperationResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	vms := make([]*providerv1.VMState, 0, len(p.vms))
	for _, vm := range p.vms {
		vms = append(vms, vm)
	}

	return providerv1.SuccessResult(vms)
}

// VMDelete deletes a VM by name.
// This function is idempotent: it will attempt to delete from libvirt even if
// the VM is not in in-memory state (e.g., from a previous crashed run).
func (p *Provider) VMDelete(name string) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm := p.vms[name]
	deleted := false

	// Always try to get domain from libvirt directly
	// This handles cases where VM exists in libvirt but not in our state
	// (e.g., provider restarted, previous run crashed)
	dom, err := p.conn.DomainLookupByName(name)
	if err == nil {
		// Stop the domain (ignore error if already stopped)
		_ = p.conn.DomainDestroy(dom)

		// Undefine the domain (for persistent domains)
		// Note: DomainCreateXML creates transient domains, so this may fail
		_ = p.conn.DomainUndefine(dom)
		deleted = true
	}

	// Clean up disk file if we have state
	if vm != nil {
		if diskPath, ok := vm.ProviderState["diskPath"].(string); ok {
			_ = os.Remove(diskPath)
		}

		// Clean up cloud-init ISO
		if isoPath, ok := vm.ProviderState["cloudInitISO"].(string); ok {
			_ = os.Remove(isoPath)
		}
	}

	// Also try to clean up files by convention if no state exists
	// This handles cases where state was lost but files remain
	if vm == nil {
		diskPath := filepath.Join(p.config.StateDir, "disks", name+".qcow2")
		_ = os.Remove(diskPath)

		isoPath := filepath.Join(p.config.StateDir, "cloudinit", name+".iso")
		_ = os.Remove(isoPath)
	}

	delete(p.vms, name)

	// Return success if we deleted anything or if nothing existed
	// This makes delete idempotent
	if deleted || vm != nil || err != nil {
		return providerv1.SuccessResult(nil)
	}

	// If nothing was found anywhere, still return success (idempotent)
	return providerv1.SuccessResult(nil)
}
