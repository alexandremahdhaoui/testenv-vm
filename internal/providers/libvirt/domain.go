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

func (p *Provider) VMCreate(req *providerv1.VMCreateRequest) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.vms[req.Name]; exists {
		return providerv1.ErrorResult(providerv1.NewAlreadyExistsError("vm", req.Name))
	}

	if existingDom, err := p.conn.DomainLookupByName(req.Name); err == nil {
		_ = p.conn.DomainDestroy(existingDom)
		_ = p.conn.DomainUndefine(existingDom)
	}

	networkNames, specErr := requestedNetworkNames(&req.Spec)
	if specErr != nil {
		return providerv1.ErrorResult(specErr)
	}
	for _, netName := range networkNames {
		if _, exists := p.networks[netName]; !exists {
			return providerv1.ErrorResult(providerv1.NewNotFoundError("network", netName))
		}
	}

	if req.Spec.Cdrom != "" {
		if _, err := os.Stat(req.Spec.Cdrom); err != nil {
			return providerv1.ErrorResult(providerv1.NewInvalidSpecError(fmt.Sprintf("cdrom ISO %s: %v", req.Spec.Cdrom, err)))
		}
	}

	ipTimeout, timeoutErr := ipResolutionBudget(req.Spec.Readiness)
	if timeoutErr != nil {
		return providerv1.ErrorResult(timeoutErr)
	}

	var diskPath, isoPath string
	var cleanupFuncs []func()
	defer func() {
		for i := len(cleanupFuncs) - 1; i >= 0; i-- {
			cleanupFuncs[i]()
		}
	}()

	diskPath = filepath.Join(p.config.StateDir, "disks", req.Name+".qcow2")
	diskSize := req.Spec.Disk.Size
	if diskSize == "" {
		diskSize = "20G"
	}
	if err := createDisk(req.Spec.Disk.BaseImage, diskPath, diskSize, p.config.QemuImgPath); err != nil {
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to create disk: "+err.Error(), false))
	}
	cleanupFuncs = append(cleanupFuncs, func() { _ = os.Remove(diskPath) })

	ciConfig := cloudInitConfigFromVMSpec(req.Name, &req.Spec, p.keys)
	if req.Spec.CloudInit != nil {
		isoPath = filepath.Join(p.config.StateDir, "cloudinit", req.Name+".iso")
		if err := generateCloudInitISO(ciConfig, isoPath, p.config.ISOTool); err != nil {
			return providerv1.ErrorResult(providerv1.NewProviderError("failed to generate cloud-init ISO: "+err.Error(), false))
		}
		cleanupFuncs = append(cleanupFuncs, func() { _ = os.Remove(isoPath) })
	}

	memoryMB := 2048
	vcpu := 2
	if req.Spec.Memory > 0 {
		memoryMB = req.Spec.Memory
	}
	if req.Spec.VCPUs > 0 {
		vcpu = req.Spec.VCPUs
	}

	domainXML, err := generateDomainXML(DomainConfig{
		Name:         req.Name,
		MemoryMB:     memoryMB,
		VCPU:         vcpu,
		DiskPath:     diskPath,
		CloudInitISO: isoPath,
		CdromPath:    req.Spec.Cdrom,
		Networks:     networkInterfaces(networkNames, req.Spec.Boot.Order),
		BootOrder:    req.Spec.Boot.Order,
		Firmware:     req.Spec.Boot.Firmware,
	})
	if err != nil {
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to generate domain XML: "+err.Error(), false))
	}

	dom, err := p.conn.DomainCreateXML(domainXML, 0)
	if err != nil {
		return providerv1.ErrorResult(providerv1.NewProviderError("failed to create domain: "+err.Error(), true))
	}
	cleanupFuncs = nil

	xmlDesc, err := p.conn.DomainGetXMLDesc(dom, 0)
	if err != nil {
		xmlDesc = ""
	}
	allMACs := extractAllMACsFromDomainXML(xmlDesc)
	macsByNet := make(map[string]string, len(networkNames))
	for i, netName := range networkNames {
		if i < len(allMACs) {
			macsByNet[netName] = allMACs[i]
		}
	}
	mac := ""
	if len(allMACs) > 0 {
		mac = allMACs[0]
	}

	strictReadiness := hasStrictReadiness(req.Spec.Readiness)

	bootTimeout := 60 * time.Second
	if bootTimeout > ipTimeout {
		bootTimeout = ipTimeout / 2
	}
	bootStart := time.Now()
	if err := waitForVMBoot(p.conn, dom, bootTimeout); err != nil && strictReadiness {
		return providerv1.ErrorResult(providerv1.NewProviderError(
			fmt.Sprintf("VM %s failed boot check: %s", req.Name, err.Error()), true))
	}

	remaining := ipTimeout - time.Since(bootStart)
	if remaining < 30*time.Second {
		remaining = 30 * time.Second
	}
	ip, err := p.resolvePrimaryIP(req, dom, networkNames[0], mac, remaining)

	if strictReadiness {
		if err != nil || ip == "" {
			return providerv1.ErrorResult(providerv1.NewTimeoutError(
				fmt.Sprintf("ip resolution for vm %s on network %s", req.Name, networkNames[0])))
		}
		if req.Spec.Readiness.SSH != nil {
			if err := validateIPReachability(ip, 22, 10*time.Second); err != nil {
				return providerv1.ErrorResult(providerv1.NewProviderError(
					fmt.Sprintf("VM %s IP %s not reachable: %s", req.Name, ip, err.Error()), true))
			}
		}
		if opErr := waitForReadiness(req.Spec.Readiness, ip); opErr != nil {
			return providerv1.ErrorResult(opErr)
		}
	} else if err != nil {
		ip = ""
	}

	ipsByNet := make(map[string]string, len(networkNames))
	if ip != "" {
		ipsByNet[networkNames[0]] = ip
	}
	for i := 1; i < len(networkNames); i++ {
		netName := networkNames[i]
		nicMAC := macsByNet[netName]
		if nicMAC == "" {
			continue
		}
		nicIP, nicErr := resolveIP(p.conn, netName, nicMAC, 5*time.Second)
		if nicErr == nil && nicIP != "" {
			ipsByNet[netName] = nicIP
		}
	}

	sshCommand := ""
	username := "ubuntu"
	if len(ciConfig.Users) > 0 {
		username = ciConfig.Users[0].Name
	}
	if ip != "" && username != "" && len(ciConfig.MatchedKeyNames) > 0 {
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
		IPs:        ipsByNet,
		MACs:       macsByNet,
		UUID:       formatUUID(dom.UUID),
		SSHCommand: sshCommand,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		ProviderState: map[string]any{
			"diskPath":     diskPath,
			"cloudInitISO": isoPath,
			"networks":     networkNames,
			"keys":         ciConfig.MatchedKeyNames,
		},
	}

	p.vms[req.Name] = state
	return providerv1.SuccessResult(state)
}

func requestedNetworkNames(spec *providerv1.VMSpec) ([]string, *providerv1.OperationError) {
	if len(spec.Networks) > 0 {
		return spec.Networks, nil
	}
	if spec.Network != "" {
		return []string{spec.Network}, nil
	}
	return nil, providerv1.NewInvalidSpecError("VM requires at least one network (set network or networks)")
}

func networkInterfaces(networkNames []string, bootOrder []string) []NetworkInterface {
	hasNetworkBoot := false
	for _, dev := range bootOrder {
		if dev == "network" {
			hasNetworkBoot = true
			break
		}
	}
	nics := make([]NetworkInterface, len(networkNames))
	for i, netName := range networkNames {
		nics[i] = NetworkInterface{
			Name:           netName,
			HasNetworkBoot: hasNetworkBoot && i == 0,
		}
	}
	return nics
}

func hasStrictReadiness(readiness *providerv1.ReadinessSpec) bool {
	return readiness != nil && (readiness.SSH != nil || readiness.TCP != nil)
}

func declaredTCPAddress(readiness *providerv1.ReadinessSpec) string {
	if readiness == nil || readiness.TCP == nil {
		return ""
	}
	return readiness.TCP.Address
}

func (p *Provider) resolvePrimaryIP(req *providerv1.VMCreateRequest, dom domainHandle, networkName, mac string, budget time.Duration) (string, error) {
	if ip := declaredTCPAddress(req.Spec.Readiness); ip != "" {
		return ip, nil
	}
	if ip := extractStaticIP(req.Spec.CloudInit); ip != "" {
		return ip, nil
	}
	ip, err := resolveIP(p.conn, networkName, mac, budget)
	if err == nil && ip != "" {
		return ip, nil
	}
	if arpIP := resolveIPFromARP(p.conn, dom); arpIP != "" {
		return arpIP, nil
	}
	return "", err
}

func (p *Provider) VMGet(name string) *providerv1.OperationResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	vm, exists := p.vms[name]
	if !exists {
		return providerv1.ErrorResult(providerv1.NewNotFoundError("vm", name))
	}

	return providerv1.SuccessResult(vm)
}

func (p *Provider) VMList(filter map[string]any) *providerv1.OperationResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	vms := make([]*providerv1.VMState, 0, len(p.vms))
	for _, vm := range p.vms {
		vms = append(vms, vm)
	}

	return providerv1.SuccessResult(vms)
}

func (p *Provider) VMDelete(name string) *providerv1.OperationResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	vm := p.vms[name]

	dom, err := p.conn.DomainLookupByName(name)
	if err == nil {
		_ = p.conn.DomainDestroy(dom)
		_ = p.conn.DomainUndefine(dom)
	}

	if vm != nil {
		if diskPath, ok := vm.ProviderState["diskPath"].(string); ok {
			_ = os.Remove(diskPath)
		}
		if isoPath, ok := vm.ProviderState["cloudInitISO"].(string); ok {
			_ = os.Remove(isoPath)
		}
	}

	if vm == nil {
		_ = os.Remove(filepath.Join(p.config.StateDir, "disks", name+".qcow2"))
		_ = os.Remove(filepath.Join(p.config.StateDir, "cloudinit", name+".iso"))
	}

	delete(p.vms, name)

	return providerv1.SuccessResult(nil)
}
