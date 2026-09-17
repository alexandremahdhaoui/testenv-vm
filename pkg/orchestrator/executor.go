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

package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/image"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/provider"
	specpkg "github.com/alexandremahdhaoui/testenv-vm/pkg/spec"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/state"
)

type Executor struct {
	manager  *provider.Manager
	store    *state.Store
	imageMgr *image.CacheManager
	mu       sync.Mutex
}

type ExecutionResult struct {
	Success bool
	Errors  []error
	State   *v1.EnvironmentState
}

func NewExecutor(manager *provider.Manager, store *state.Store, imageMgr *image.CacheManager) *Executor {
	return &Executor{
		manager:  manager,
		store:    store,
		imageMgr: imageMgr,
	}
}

func (e *Executor) ExecuteCreate(
	ctx context.Context,
	spec *v1.Spec,
	plan [][]v1.ResourceRef,
	templateCtx *specpkg.TemplateContext,
	envState *v1.EnvironmentState,
	templatedFields *specpkg.TemplatedFields,
	isoConfig *IsolationConfig,
) (*ExecutionResult, error) {
	if spec == nil {
		return nil, fmt.Errorf("spec cannot be nil")
	}
	if envState == nil {
		return nil, fmt.Errorf("state cannot be nil")
	}

	result := &ExecutionResult{
		Success: true,
		Errors:  []error{},
		State:   envState,
	}

	for phaseIdx, phase := range plan {
		if len(phase) == 0 {
			continue
		}

		phaseErrors := e.executePhase(ctx, phase, spec, templateCtx, envState, templatedFields, isoConfig)
		if len(phaseErrors) > 0 {
			result.Errors = append(result.Errors, phaseErrors...)
			result.Success = false

			for i, err := range phaseErrors {
				if i < len(phase) {
					envState.Errors = append(envState.Errors, v1.ErrorRecord{
						Resource:  phase[i],
						Operation: "create",
						Error:     err.Error(),
						Timestamp: time.Now().UTC().Format(time.RFC3339),
					})
				}
			}

			envState.Status = v1.StatusFailed
			envState.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if saveErr := e.store.Save(envState); saveErr != nil {
				result.Errors = append(result.Errors, fmt.Errorf("failed to save state after phase %d error: %w", phaseIdx, saveErr))
			}

			return result, nil
		}
	}

	return result, nil
}

func (e *Executor) ExecuteDelete(ctx context.Context, envState *v1.EnvironmentState, isoConfig *IsolationConfig) error {
	if envState == nil {
		return fmt.Errorf("state cannot be nil")
	}

	var phases [][]v1.ResourceRef
	if envState.ExecutionPlan != nil {
		for _, phase := range envState.ExecutionPlan.Phases {
			phases = append(phases, phase.Resources)
		}
	}

	for i, j := 0, len(phases)-1; i < j; i, j = i+1, j-1 {
		phases[i], phases[j] = phases[j], phases[i]
	}

	var allErrors []error

	for _, phase := range phases {
		if len(phase) == 0 {
			continue
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		var phaseErrors []error

		for _, ref := range phase {
			wg.Add(1)
			go func(r v1.ResourceRef) {
				defer wg.Done()

				if err := e.deleteResource(ctx, r, envState, isoConfig); err != nil {
					mu.Lock()
					phaseErrors = append(phaseErrors, fmt.Errorf("failed to delete %s/%s: %w", r.Kind, r.Name, err))
					mu.Unlock()
				}
			}(ref)
		}

		wg.Wait()

		allErrors = append(allErrors, phaseErrors...)
	}

	if len(allErrors) > 0 {
		return fmt.Errorf("delete completed with %d errors: %v", len(allErrors), allErrors)
	}

	return nil
}

func (e *Executor) executePhase(
	ctx context.Context,
	phase []v1.ResourceRef,
	spec *v1.Spec,
	templateCtx *specpkg.TemplateContext,
	envState *v1.EnvironmentState,
	templatedFields *specpkg.TemplatedFields,
	isoConfig *IsolationConfig,
) []error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []error

	for _, ref := range phase {
		wg.Add(1)
		go func(r v1.ResourceRef) {
			defer wg.Done()

			if err := e.createResource(ctx, r, spec, templateCtx, envState, templatedFields, isoConfig); err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("failed to create %s/%s: %w", r.Kind, r.Name, err))
				mu.Unlock()
			}
		}(ref)
	}

	wg.Wait()
	return errors
}

func prefixedName(isoConfig *IsolationConfig, name string) string {
	if isoConfig == nil || isoConfig.NamePrefix == "" {
		return name
	}
	return isoConfig.NamePrefix + "-" + name
}

func (e *Executor) createResource(
	ctx context.Context,
	ref v1.ResourceRef,
	spec *v1.Spec,
	templateCtx *specpkg.TemplateContext,
	envState *v1.EnvironmentState,
	templatedFields *specpkg.TemplatedFields,
	isoConfig *IsolationConfig,
) error {
	providerName := ref.Provider
	if providerName == "" {
		providerName = spec.DefaultProvider
	}

	var tool string
	var request interface{}

	switch ref.Kind {
	case "key":
		tool = "key_create"
		keySpec, err := e.findKeySpec(spec, ref.Name)
		if err != nil {
			return err
		}
		renderedSpec, err := e.renderKeySpec(keySpec, templateCtx)
		if err != nil {
			return fmt.Errorf("failed to render key spec: %w", err)
		}
		outputDir := renderedSpec.Spec.OutputDir
		if outputDir == "" && spec.StateDir != "" {
			outputDir = filepath.Join(spec.StateDir, "keys")
		}
		request = &providerv1.KeyCreateRequest{
			Name: prefixedName(isoConfig, ref.Name),
			Spec: providerv1.KeySpec{
				Type:      renderedSpec.Spec.Type,
				Bits:      renderedSpec.Spec.Bits,
				Comment:   renderedSpec.Spec.Comment,
				OutputDir: outputDir,
			},
			ProviderSpec: renderedSpec.ProviderSpec,
		}
		if providerName == "" {
			providerName = renderedSpec.Provider
		}

	case "network":
		tool = "network_create"
		networkSpec, err := e.findNetworkSpec(spec, ref.Name)
		if err != nil {
			return err
		}
		renderedSpec, err := e.renderNetworkSpec(networkSpec, templateCtx)
		if err != nil {
			return fmt.Errorf("failed to render network spec: %w", err)
		}
		if err := specpkg.ValidateResourceRefsLate("network", ref.Name, renderedSpec, spec, templatedFields); err != nil {
			return fmt.Errorf("phase 2 validation failed: %w", err)
		}
		convertedSpec := e.convertNetworkSpec(renderedSpec.Spec)
		if isoConfig != nil && isoConfig.OriginalCIDRPrefix != isoConfig.NewCIDRPrefix {
			convertedSpec.CIDR = strings.ReplaceAll(convertedSpec.CIDR, isoConfig.OriginalCIDRPrefix, isoConfig.NewCIDRPrefix)
			convertedSpec.Gateway = strings.ReplaceAll(convertedSpec.Gateway, isoConfig.OriginalCIDRPrefix, isoConfig.NewCIDRPrefix)
			if convertedSpec.DHCP != nil {
				convertedSpec.DHCP.RangeStart = strings.ReplaceAll(convertedSpec.DHCP.RangeStart, isoConfig.OriginalCIDRPrefix, isoConfig.NewCIDRPrefix)
				convertedSpec.DHCP.RangeEnd = strings.ReplaceAll(convertedSpec.DHCP.RangeEnd, isoConfig.OriginalCIDRPrefix, isoConfig.NewCIDRPrefix)
			}
		}
		request = &providerv1.NetworkCreateRequest{
			Name:         prefixedName(isoConfig, ref.Name),
			Kind:         renderedSpec.Kind,
			Spec:         convertedSpec,
			ProviderSpec: renderedSpec.ProviderSpec,
		}
		if providerName == "" {
			providerName = renderedSpec.Provider
		}

	case "vm":
		tool = "vm_create"
		vmSpec, err := e.findVMSpec(spec, ref.Name)
		if err != nil {
			return err
		}
		renderedSpec, err := e.renderVMSpec(vmSpec, templateCtx)
		if err != nil {
			return fmt.Errorf("failed to render vm spec: %w", err)
		}
		if err := specpkg.ValidateResourceRefsLate("vm", ref.Name, renderedSpec, spec, templatedFields); err != nil {
			return fmt.Errorf("phase 2 validation failed: %w", err)
		}
		convertedVMSpec := e.convertVMSpec(renderedSpec.Spec)
		if isoConfig != nil && isoConfig.NamePrefix != "" {
			if len(convertedVMSpec.Networks) > 0 {
				for i, n := range convertedVMSpec.Networks {
					convertedVMSpec.Networks[i] = prefixedName(isoConfig, n)
				}
			}
			if convertedVMSpec.Network != "" {
				convertedVMSpec.Network = prefixedName(isoConfig, convertedVMSpec.Network)
			}
		}
		if isoConfig != nil && isoConfig.OriginalCIDRPrefix != isoConfig.NewCIDRPrefix {
			if convertedVMSpec.CloudInit != nil {
				for i, wf := range convertedVMSpec.CloudInit.WriteFiles {
					convertedVMSpec.CloudInit.WriteFiles[i].Content = strings.ReplaceAll(wf.Content, isoConfig.OriginalCIDRPrefix, isoConfig.NewCIDRPrefix)
				}
				for i, cmd := range convertedVMSpec.CloudInit.Runcmd {
					convertedVMSpec.CloudInit.Runcmd[i] = strings.ReplaceAll(cmd, isoConfig.OriginalCIDRPrefix, isoConfig.NewCIDRPrefix)
				}
				if convertedVMSpec.CloudInit.NetworkConfig != nil {
					for i, eth := range convertedVMSpec.CloudInit.NetworkConfig.Ethernets {
						for j, addr := range eth.Addresses {
							convertedVMSpec.CloudInit.NetworkConfig.Ethernets[i].Addresses[j] = strings.ReplaceAll(addr, isoConfig.OriginalCIDRPrefix, isoConfig.NewCIDRPrefix)
						}
						convertedVMSpec.CloudInit.NetworkConfig.Ethernets[i].Gateway4 = strings.ReplaceAll(eth.Gateway4, isoConfig.OriginalCIDRPrefix, isoConfig.NewCIDRPrefix)
					}
				}
			}
		}
		request = &providerv1.VMCreateRequest{
			Name:         prefixedName(isoConfig, ref.Name),
			Spec:         convertedVMSpec,
			ProviderSpec: renderedSpec.ProviderSpec,
		}
		if providerName == "" {
			providerName = renderedSpec.Provider
		}

	case "image":
		imageRes, err := e.findImageSpec(spec, ref.Name)
		if err != nil {
			return err
		}
		imgState, err := e.imageMgr.EnsureImage(ctx, ref.Name, imageRes.Spec)
		if err != nil {
			return fmt.Errorf("failed to ensure image %q: %w", ref.Name, err)
		}
		e.mu.Lock()
		if templateCtx.Images == nil {
			templateCtx.Images = make(map[string]specpkg.ImageTemplateData)
		}
		templateCtx.Images[ref.Name] = specpkg.ImageTemplateData{
			Path: imgState.LocalPath,
			Name: ref.Name,
		}
		if imageRes.Spec.Alias != "" {
			templateCtx.Images[imageRes.Spec.Alias] = specpkg.ImageTemplateData{
				Path: imgState.LocalPath,
				Name: ref.Name,
			}
		}
		e.mu.Unlock()
		return nil

	default:
		return fmt.Errorf("unknown resource kind: %s", ref.Kind)
	}

	if providerName == "" {
		for _, p := range spec.Providers {
			if p.Default {
				providerName = p.Name
				break
			}
		}
	}

	if providerName == "" {
		return fmt.Errorf("no provider specified for resource %s/%s and no default provider configured", ref.Kind, ref.Name)
	}

	result, err := e.manager.Call(providerName, tool, request)
	if err != nil {
		e.mu.Lock()
		e.updateResourceState(envState, ref, providerName, v1.StatusFailed, nil, err.Error())
		e.mu.Unlock()
		return fmt.Errorf("provider call failed: %w", err)
	}

	if !result.Success {
		errMsg := "unknown error"
		if result.Error != nil {
			errMsg = result.Error.Message
		}
		e.mu.Lock()
		e.updateResourceState(envState, ref, providerName, v1.StatusFailed, nil, errMsg)
		e.mu.Unlock()
		return fmt.Errorf("provider returned error: %s", errMsg)
	}

	resourceState, err := e.convertResourceToMap(result.Resource)
	if err != nil {
		return fmt.Errorf("failed to convert resource state: %w", err)
	}

	e.mu.Lock()
	e.updateResourceState(envState, ref, providerName, v1.StatusReady, resourceState, "")

	e.updateTemplateContext(templateCtx, ref, resourceState)

	envState.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	err = e.store.Save(envState)
	e.mu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to save state after creating %s/%s: %w", ref.Kind, ref.Name, err)
	}

	return nil
}

func (e *Executor) deleteResource(
	ctx context.Context,
	ref v1.ResourceRef,
	envState *v1.EnvironmentState,
	isoConfig *IsolationConfig,
) error {
	e.mu.Lock()
	resourceState := e.getResourceState(envState, ref)
	e.mu.Unlock()

	if resourceState == nil {
		return nil
	}

	providerName := resourceState.Provider
	if providerName == "" {
		return fmt.Errorf("no provider found for resource %s/%s", ref.Kind, ref.Name)
	}

	var tool string
	switch ref.Kind {
	case "key":
		tool = "key_delete"
	case "network":
		tool = "network_delete"
	case "vm":
		tool = "vm_delete"
	default:
		return fmt.Errorf("unknown resource kind: %s", ref.Kind)
	}

	request := &providerv1.DeleteRequest{
		Name: prefixedName(isoConfig, ref.Name),
	}

	result, err := e.manager.Call(providerName, tool, request)
	if err != nil {
		return fmt.Errorf("provider call failed: %w", err)
	}

	if !result.Success {
		if result.Error != nil && result.Error.Code == providerv1.ErrCodeNotFound {
			e.mu.Lock()
			e.updateResourceState(envState, ref, providerName, v1.StatusDestroyed, nil, "")
			e.mu.Unlock()
			return nil
		}

		errMsg := "unknown error"
		if result.Error != nil {
			errMsg = result.Error.Message
		}
		return fmt.Errorf("provider returned error: %s", errMsg)
	}

	e.mu.Lock()
	e.updateResourceState(envState, ref, providerName, v1.StatusDestroyed, nil, "")
	e.mu.Unlock()

	return nil
}

func (e *Executor) findKeySpec(spec *v1.Spec, name string) (*v1.KeyResource, error) {
	for i := range spec.Keys {
		if spec.Keys[i].Name == name {
			return &spec.Keys[i], nil
		}
	}
	return nil, fmt.Errorf("key resource %q not found in spec", name)
}

func (e *Executor) findNetworkSpec(spec *v1.Spec, name string) (*v1.NetworkResource, error) {
	for i := range spec.Networks {
		if spec.Networks[i].Name == name {
			return &spec.Networks[i], nil
		}
	}
	return nil, fmt.Errorf("network resource %q not found in spec", name)
}

func (e *Executor) findVMSpec(spec *v1.Spec, name string) (*v1.VMResource, error) {
	for i := range spec.Vms {
		if spec.Vms[i].Name == name {
			return &spec.Vms[i], nil
		}
	}
	return nil, fmt.Errorf("vm resource %q not found in spec", name)
}

func (e *Executor) findImageSpec(spec *v1.Spec, name string) (*v1.ImageResource, error) {
	for i := range spec.Images {
		if spec.Images[i].Name == name {
			return &spec.Images[i], nil
		}
	}
	return nil, fmt.Errorf("image resource %q not found in spec", name)
}

func (e *Executor) renderKeySpec(original *v1.KeyResource, templateCtx *specpkg.TemplateContext) (*v1.KeyResource, error) {
	data, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	var copy v1.KeyResource
	if err := json.Unmarshal(data, &copy); err != nil {
		return nil, err
	}

	if err := specpkg.RenderSpec(&copy, templateCtx); err != nil {
		return nil, err
	}

	return &copy, nil
}

func (e *Executor) renderNetworkSpec(original *v1.NetworkResource, templateCtx *specpkg.TemplateContext) (*v1.NetworkResource, error) {
	data, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	var copy v1.NetworkResource
	if err := json.Unmarshal(data, &copy); err != nil {
		return nil, err
	}

	if err := specpkg.RenderSpec(&copy, templateCtx); err != nil {
		return nil, err
	}

	return &copy, nil
}

func (e *Executor) renderVMSpec(original *v1.VMResource, templateCtx *specpkg.TemplateContext) (*v1.VMResource, error) {
	data, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	var copy v1.VMResource
	if err := json.Unmarshal(data, &copy); err != nil {
		return nil, err
	}

	if err := specpkg.RenderSpec(&copy, templateCtx); err != nil {
		return nil, err
	}

	return &copy, nil
}

func (e *Executor) convertNetworkSpec(spec v1.NetworkSpec) providerv1.NetworkSpec {
	result := providerv1.NetworkSpec{
		CIDR:     spec.Cidr,
		Gateway:  spec.Gateway,
		AttachTo: spec.AttachTo,
		MTU:      spec.Mtu,
	}

	if spec.Dhcp != nil {
		result.DHCP = &providerv1.DHCPSpec{
			Enabled:    spec.Dhcp.Enabled,
			RangeStart: spec.Dhcp.RangeStart,
			RangeEnd:   spec.Dhcp.RangeEnd,
			LeaseTime:  spec.Dhcp.LeaseTime,
			Router:     spec.Dhcp.Router,
			DNSServers: spec.Dhcp.DnsServers,
		}
	}

	if spec.Dns != nil {
		result.DNS = &providerv1.DNSSpec{
			Enabled: spec.Dns.Enabled,
			Servers: spec.Dns.Servers,
		}
	}

	if spec.Tftp != nil {
		result.TFTP = &providerv1.TFTPSpec{
			Enabled:  spec.Tftp.Enabled,
			Root:     spec.Tftp.Root,
			BootFile: spec.Tftp.BootFile,
		}
	}

	return result
}

func (e *Executor) convertVMSpec(spec v1.VMSpec) providerv1.VMSpec {
	bootOrder := spec.Boot.Order
	if bootOrder == nil {
		bootOrder = []string{}
	}

	var networks []string
	network := spec.Network
	if len(spec.Networks) > 0 {
		networks = spec.Networks
	} else if spec.Network != "" {
		networks = []string{spec.Network}
	}

	result := providerv1.VMSpec{
		Memory:   spec.Memory,
		VCPUs:    spec.Vcpus,
		Network:  network,
		Networks: networks,
		Cdrom:    spec.Cdrom,
		Disk: providerv1.DiskSpec{
			BaseImage: spec.Disk.BaseImage,
			Size:      spec.Disk.Size,
			Bus:       spec.Disk.Bus,
			WWN:       spec.Disk.Wwn,
		},
		Boot: providerv1.BootSpec{
			Order:    bootOrder,
			Firmware: spec.Boot.Firmware,
		},
	}

	if spec.CloudInit.Hostname != "" || len(spec.CloudInit.Users) > 0 || len(spec.CloudInit.Packages) > 0 ||
		len(spec.CloudInit.Runcmd) > 0 || len(spec.CloudInit.WriteFiles) > 0 || len(spec.CloudInit.NetworkConfig.Ethernets) > 0 {
		result.CloudInit = &providerv1.CloudInitSpec{
			Hostname: spec.CloudInit.Hostname,
			Packages: spec.CloudInit.Packages,
			Runcmd:   spec.CloudInit.Runcmd,
		}
		for _, u := range spec.CloudInit.Users {
			result.CloudInit.Users = append(result.CloudInit.Users, providerv1.UserSpec{
				Name:              u.Name,
				Sudo:              u.Sudo,
				SSHAuthorizedKeys: u.SshAuthorizedKeys,
			})
		}
		for _, wf := range spec.CloudInit.WriteFiles {
			result.CloudInit.WriteFiles = append(result.CloudInit.WriteFiles, providerv1.WriteFileSpec{
				Path:        wf.Path,
				Content:     wf.Content,
				Permissions: wf.Permissions,
			})
		}
		if len(spec.CloudInit.NetworkConfig.Ethernets) > 0 {
			result.CloudInit.NetworkConfig = &providerv1.CloudInitNetworkConfig{}
			for _, eth := range spec.CloudInit.NetworkConfig.Ethernets {
				dhcp4 := eth.Dhcp4
				providerEth := providerv1.CloudInitEthernetConfig{
					Name:      eth.Name,
					DHCP4:     &dhcp4,
					Addresses: eth.Addresses,
					Gateway4:  eth.Gateway4,
				}
				if len(eth.Nameservers.Addresses) > 0 {
					providerEth.Nameservers = &providerv1.CloudInitNameservers{
						Addresses: eth.Nameservers.Addresses,
					}
				}
				result.CloudInit.NetworkConfig.Ethernets = append(result.CloudInit.NetworkConfig.Ethernets, providerEth)
			}
		}
	}

	if spec.Readiness.Ssh.Enabled || spec.Readiness.CloudInit.Enabled || spec.Readiness.Tcp.Port > 0 {
		result.Readiness = &providerv1.ReadinessSpec{}
	}

	if spec.Readiness.Ssh.Enabled {
		result.Readiness.SSH = &providerv1.SSHReadinessSpec{
			Enabled:    spec.Readiness.Ssh.Enabled,
			Timeout:    spec.Readiness.Ssh.Timeout,
			User:       spec.Readiness.Ssh.User,
			PrivateKey: spec.Readiness.Ssh.PrivateKey,
		}
	}

	if spec.Readiness.CloudInit.Enabled {
		result.Readiness.CloudInit = &providerv1.CloudInitReadinessSpec{
			Enabled: spec.Readiness.CloudInit.Enabled,
			Timeout: spec.Readiness.CloudInit.Timeout,
		}
	}

	if spec.Readiness.Tcp.Port > 0 {
		result.Readiness.TCP = &providerv1.TCPReadinessSpec{
			Port:    spec.Readiness.Tcp.Port,
			Address: spec.Readiness.Tcp.Address,
			Timeout: spec.Readiness.Tcp.Timeout,
		}
	}

	return result
}

func (e *Executor) convertResourceToMap(resource any) (map[string]any, error) {
	if resource == nil {
		return nil, nil
	}

	if m, ok := resource.(map[string]any); ok {
		return m, nil
	}

	data, err := json.Marshal(resource)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func (e *Executor) updateResourceState(
	envState *v1.EnvironmentState,
	ref v1.ResourceRef,
	providerName string,
	status string,
	resourceData map[string]any,
	errMsg string,
) {
	state := &v1.ResourceState{
		Provider:  providerName,
		Status:    status,
		State:     resourceData,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Error:     errMsg,
	}

	if status == v1.StatusReady {
		state.CreatedAt = state.UpdatedAt
	}

	if envState.Resources.Keys == nil {
		envState.Resources.Keys = make(map[string]*v1.ResourceState)
	}
	if envState.Resources.Networks == nil {
		envState.Resources.Networks = make(map[string]*v1.ResourceState)
	}
	if envState.Resources.VMs == nil {
		envState.Resources.VMs = make(map[string]*v1.ResourceState)
	}

	switch ref.Kind {
	case "key":
		envState.Resources.Keys[ref.Name] = state
	case "network":
		envState.Resources.Networks[ref.Name] = state
	case "vm":
		envState.Resources.VMs[ref.Name] = state
	}
}

func (e *Executor) getResourceState(envState *v1.EnvironmentState, ref v1.ResourceRef) *v1.ResourceState {
	switch ref.Kind {
	case "key":
		if envState.Resources.Keys != nil {
			return envState.Resources.Keys[ref.Name]
		}
	case "network":
		if envState.Resources.Networks != nil {
			return envState.Resources.Networks[ref.Name]
		}
	case "vm":
		if envState.Resources.VMs != nil {
			return envState.Resources.VMs[ref.Name]
		}
	}
	return nil
}

func (e *Executor) updateTemplateContext(templateCtx *specpkg.TemplateContext, ref v1.ResourceRef, resourceData map[string]any) {
	if templateCtx == nil || resourceData == nil {
		return
	}

	switch ref.Kind {
	case "key":
		if templateCtx.Keys == nil {
			templateCtx.Keys = make(map[string]specpkg.KeyTemplateData)
		}
		templateCtx.Keys[ref.Name] = specpkg.KeyTemplateData{
			PublicKey:      getString(resourceData, "publicKey"),
			PrivateKeyPath: getString(resourceData, "privateKeyPath"),
			PublicKeyPath:  getString(resourceData, "publicKeyPath"),
			Fingerprint:    getString(resourceData, "fingerprint"),
		}

	case "network":
		if templateCtx.Networks == nil {
			templateCtx.Networks = make(map[string]specpkg.NetworkTemplateData)
		}
		templateCtx.Networks[ref.Name] = specpkg.NetworkTemplateData{
			Name:          getString(resourceData, "name"),
			IP:            getString(resourceData, "ip"),
			CIDR:          getString(resourceData, "cidr"),
			InterfaceName: getString(resourceData, "interfaceName"),
			UUID:          getString(resourceData, "uuid"),
		}

	case "vm":
		if templateCtx.VMs == nil {
			templateCtx.VMs = make(map[string]specpkg.VMTemplateData)
		}
		templateCtx.VMs[ref.Name] = specpkg.VMTemplateData{
			Name:       getString(resourceData, "name"),
			IP:         getString(resourceData, "ip"),
			MAC:        getString(resourceData, "mac"),
			SSHCommand: getString(resourceData, "sshCommand"),
		}
	}
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
