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

// Package orchestrator provides resource orchestration and execution.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/provider"
	specpkg "github.com/alexandremahdhaoui/testenv-vm/pkg/spec"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/state"
)

// Executor executes resource operations using providers.
type Executor struct {
	manager *provider.Manager
	store   *state.Store
	mu      sync.Mutex // Protects state modifications during parallel execution
}

// ExecutionResult contains the result of an execution operation.
type ExecutionResult struct {
	// Success indicates if all operations completed successfully.
	Success bool
	// Errors contains any errors that occurred during execution.
	Errors []error
	// State is the updated environment state.
	State *v1.EnvironmentState
}

// NewExecutor creates a new Executor with the given provider manager and state store.
func NewExecutor(manager *provider.Manager, store *state.Store) *Executor {
	return &Executor{
		manager: manager,
		store:   store,
	}
}

// ExecuteCreate executes the creation of resources according to the execution plan.
// Phases are executed sequentially, while resources within each phase are executed in parallel.
// Templates are rendered just-in-time before each resource creation.
func (e *Executor) ExecuteCreate(
	ctx context.Context,
	spec *v1.TestenvSpec,
	plan [][]v1.ResourceRef,
	templateCtx *specpkg.TemplateContext,
	envState *v1.EnvironmentState,
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

	// Execute phases sequentially
	for phaseIdx, phase := range plan {
		if len(phase) == 0 {
			continue
		}

		phaseErrors := e.executePhase(ctx, phase, spec, templateCtx, envState)
		if len(phaseErrors) > 0 {
			result.Errors = append(result.Errors, phaseErrors...)
			result.Success = false

			// Record errors in state
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

			// Update status to failed
			envState.Status = v1.StatusFailed
			envState.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if saveErr := e.store.Save(envState); saveErr != nil {
				result.Errors = append(result.Errors, fmt.Errorf("failed to save state after phase %d error: %w", phaseIdx, saveErr))
			}

			// Return on first phase with errors
			return result, nil
		}
	}

	return result, nil
}

// ExecuteDelete executes the deletion of resources in reverse order.
// Phases are reversed and processed sequentially, with resources in each phase deleted in parallel.
// Best-effort: continues on individual failures, collecting all errors.
// Note: Status management is handled by the orchestrator, not the executor.
func (e *Executor) ExecuteDelete(ctx context.Context, envState *v1.EnvironmentState) error {
	if envState == nil {
		return fmt.Errorf("state cannot be nil")
	}

	// Get the execution plan and reverse the phases
	var phases [][]v1.ResourceRef
	if envState.ExecutionPlan != nil {
		for _, phase := range envState.ExecutionPlan.Phases {
			phases = append(phases, phase.Resources)
		}
	}

	// Reverse the phases for deletion
	for i, j := 0, len(phases)-1; i < j; i, j = i+1, j-1 {
		phases[i], phases[j] = phases[j], phases[i]
	}

	var allErrors []error

	// Execute deletion phases sequentially
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

				if err := e.deleteResource(ctx, r, envState); err != nil {
					mu.Lock()
					phaseErrors = append(phaseErrors, fmt.Errorf("failed to delete %s/%s: %w", r.Kind, r.Name, err))
					mu.Unlock()
				}
			}(ref)
		}

		wg.Wait()

		// Collect errors but continue with next phase (best-effort)
		allErrors = append(allErrors, phaseErrors...)
	}

	if len(allErrors) > 0 {
		return fmt.Errorf("delete completed with %d errors: %v", len(allErrors), allErrors)
	}

	return nil
}

// executePhase executes all resources in a phase in parallel.
// Returns errors for any failed resources.
func (e *Executor) executePhase(
	ctx context.Context,
	phase []v1.ResourceRef,
	spec *v1.TestenvSpec,
	templateCtx *specpkg.TemplateContext,
	envState *v1.EnvironmentState,
) []error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []error

	for _, ref := range phase {
		wg.Add(1)
		go func(r v1.ResourceRef) {
			defer wg.Done()

			if err := e.createResource(ctx, r, spec, templateCtx, envState); err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("failed to create %s/%s: %w", r.Kind, r.Name, err))
				mu.Unlock()
			}
		}(ref)
	}

	wg.Wait()
	return errors
}

// createResource creates a single resource using the appropriate provider.
func (e *Executor) createResource(
	ctx context.Context,
	ref v1.ResourceRef,
	spec *v1.TestenvSpec,
	templateCtx *specpkg.TemplateContext,
	envState *v1.EnvironmentState,
) error {
	// Determine provider name
	providerName := ref.Provider
	if providerName == "" {
		providerName = spec.DefaultProvider
	}

	// Get the appropriate tool name and request based on resource kind
	var tool string
	var request interface{}

	switch ref.Kind {
	case "key":
		tool = "key_create"
		keySpec, err := e.findKeySpec(spec, ref.Name)
		if err != nil {
			return err
		}
		// Deep copy and render templates
		renderedSpec, err := e.renderKeySpec(keySpec, templateCtx)
		if err != nil {
			return fmt.Errorf("failed to render key spec: %w", err)
		}
		request = &providerv1.KeyCreateRequest{
			Name: ref.Name,
			Spec: providerv1.KeySpec{
				Type:      renderedSpec.Spec.Type,
				Bits:      renderedSpec.Spec.Bits,
				Comment:   renderedSpec.Spec.Comment,
				OutputDir: renderedSpec.Spec.OutputDir,
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
		// Deep copy and render templates
		renderedSpec, err := e.renderNetworkSpec(networkSpec, templateCtx)
		if err != nil {
			return fmt.Errorf("failed to render network spec: %w", err)
		}
		request = &providerv1.NetworkCreateRequest{
			Name: ref.Name,
			Kind: renderedSpec.Kind,
			Spec: e.convertNetworkSpec(renderedSpec.Spec),
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
		// Deep copy and render templates
		renderedSpec, err := e.renderVMSpec(vmSpec, templateCtx)
		if err != nil {
			return fmt.Errorf("failed to render vm spec: %w", err)
		}
		request = &providerv1.VMCreateRequest{
			Name: ref.Name,
			Spec: e.convertVMSpec(renderedSpec.Spec),
			ProviderSpec: renderedSpec.ProviderSpec,
		}
		if providerName == "" {
			providerName = renderedSpec.Provider
		}

	default:
		return fmt.Errorf("unknown resource kind: %s", ref.Kind)
	}

	// Use default provider if still not set
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

	// Call the provider
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

	// Update state with the result
	resourceState, err := e.convertResourceToMap(result.Resource)
	if err != nil {
		return fmt.Errorf("failed to convert resource state: %w", err)
	}

	// Lock to protect state modifications during parallel execution
	e.mu.Lock()
	e.updateResourceState(envState, ref, providerName, v1.StatusReady, resourceState, "")

	// Update template context with the new resource data
	e.updateTemplateContext(templateCtx, ref, resourceState)

	// Persist state
	envState.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	err = e.store.Save(envState)
	e.mu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to save state after creating %s/%s: %w", ref.Kind, ref.Name, err)
	}

	return nil
}

// deleteResource deletes a single resource using the appropriate provider.
func (e *Executor) deleteResource(
	ctx context.Context,
	ref v1.ResourceRef,
	envState *v1.EnvironmentState,
) error {
	// Lock to protect state reads during parallel execution
	e.mu.Lock()
	resourceState := e.getResourceState(envState, ref)
	e.mu.Unlock()

	if resourceState == nil {
		// Resource doesn't exist in state, nothing to delete
		return nil
	}

	providerName := resourceState.Provider
	if providerName == "" {
		return fmt.Errorf("no provider found for resource %s/%s", ref.Kind, ref.Name)
	}

	// Determine the delete tool based on kind
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

	// Create delete request
	request := &providerv1.DeleteRequest{
		Name: ref.Name,
	}

	// Call the provider
	result, err := e.manager.Call(providerName, tool, request)
	if err != nil {
		return fmt.Errorf("provider call failed: %w", err)
	}

	if !result.Success {
		// Check if it's a not found error - that's OK for delete
		if result.Error != nil && result.Error.Code == providerv1.ErrCodeNotFound {
			// Resource already doesn't exist
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

	// Update state with lock protection
	e.mu.Lock()
	e.updateResourceState(envState, ref, providerName, v1.StatusDestroyed, nil, "")
	e.mu.Unlock()

	return nil
}

// findKeySpec finds a key resource by name in the spec.
func (e *Executor) findKeySpec(spec *v1.TestenvSpec, name string) (*v1.KeyResource, error) {
	for i := range spec.Keys {
		if spec.Keys[i].Name == name {
			return &spec.Keys[i], nil
		}
	}
	return nil, fmt.Errorf("key resource %q not found in spec", name)
}

// findNetworkSpec finds a network resource by name in the spec.
func (e *Executor) findNetworkSpec(spec *v1.TestenvSpec, name string) (*v1.NetworkResource, error) {
	for i := range spec.Networks {
		if spec.Networks[i].Name == name {
			return &spec.Networks[i], nil
		}
	}
	return nil, fmt.Errorf("network resource %q not found in spec", name)
}

// findVMSpec finds a VM resource by name in the spec.
func (e *Executor) findVMSpec(spec *v1.TestenvSpec, name string) (*v1.VMResource, error) {
	for i := range spec.VMs {
		if spec.VMs[i].Name == name {
			return &spec.VMs[i], nil
		}
	}
	return nil, fmt.Errorf("vm resource %q not found in spec", name)
}

// renderKeySpec creates a deep copy and renders templates in a key spec.
func (e *Executor) renderKeySpec(original *v1.KeyResource, templateCtx *specpkg.TemplateContext) (*v1.KeyResource, error) {
	// Deep copy via JSON marshaling
	data, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	var copy v1.KeyResource
	if err := json.Unmarshal(data, &copy); err != nil {
		return nil, err
	}

	// Render templates
	if err := specpkg.RenderSpec(&copy, templateCtx); err != nil {
		return nil, err
	}

	return &copy, nil
}

// renderNetworkSpec creates a deep copy and renders templates in a network spec.
func (e *Executor) renderNetworkSpec(original *v1.NetworkResource, templateCtx *specpkg.TemplateContext) (*v1.NetworkResource, error) {
	// Deep copy via JSON marshaling
	data, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	var copy v1.NetworkResource
	if err := json.Unmarshal(data, &copy); err != nil {
		return nil, err
	}

	// Render templates
	if err := specpkg.RenderSpec(&copy, templateCtx); err != nil {
		return nil, err
	}

	return &copy, nil
}

// renderVMSpec creates a deep copy and renders templates in a VM spec.
func (e *Executor) renderVMSpec(original *v1.VMResource, templateCtx *specpkg.TemplateContext) (*v1.VMResource, error) {
	// Deep copy via JSON marshaling
	data, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	var copy v1.VMResource
	if err := json.Unmarshal(data, &copy); err != nil {
		return nil, err
	}

	// Render templates
	if err := specpkg.RenderSpec(&copy, templateCtx); err != nil {
		return nil, err
	}

	return &copy, nil
}

// convertNetworkSpec converts v1.NetworkSpec to providerv1.NetworkSpec.
func (e *Executor) convertNetworkSpec(spec v1.NetworkSpec) providerv1.NetworkSpec {
	result := providerv1.NetworkSpec{
		CIDR:     spec.CIDR,
		Gateway:  spec.Gateway,
		AttachTo: spec.AttachTo,
		MTU:      spec.MTU,
	}

	if spec.DHCP != nil {
		result.DHCP = &providerv1.DHCPSpec{
			Enabled:    spec.DHCP.Enabled,
			RangeStart: spec.DHCP.RangeStart,
			RangeEnd:   spec.DHCP.RangeEnd,
			LeaseTime:  spec.DHCP.LeaseTime,
		}
	}

	if spec.DNS != nil {
		result.DNS = &providerv1.DNSSpec{
			Enabled: spec.DNS.Enabled,
			Servers: spec.DNS.Servers,
		}
	}

	if spec.TFTP != nil {
		result.TFTP = &providerv1.TFTPSpec{
			Enabled:  spec.TFTP.Enabled,
			Root:     spec.TFTP.Root,
			BootFile: spec.TFTP.BootFile,
		}
	}

	return result
}

// convertVMSpec converts v1.VMSpec to providerv1.VMSpec.
func (e *Executor) convertVMSpec(spec v1.VMSpec) providerv1.VMSpec {
	result := providerv1.VMSpec{
		Memory:  spec.Memory,
		VCPUs:   spec.VCPUs,
		Network: spec.Network,
		Disk: providerv1.DiskSpec{
			BaseImage: spec.Disk.BaseImage,
			Size:      spec.Disk.Size,
		},
		Boot: providerv1.BootSpec{
			Order:    spec.Boot.Order,
			Firmware: spec.Boot.Firmware,
		},
	}

	if spec.CloudInit != nil {
		result.CloudInit = &providerv1.CloudInitSpec{
			Hostname: spec.CloudInit.Hostname,
			Packages: spec.CloudInit.Packages,
		}
		for _, u := range spec.CloudInit.Users {
			result.CloudInit.Users = append(result.CloudInit.Users, providerv1.UserSpec{
				Name:              u.Name,
				Sudo:              u.Sudo,
				SSHAuthorizedKeys: u.SSHAuthorizedKeys,
			})
		}
	}

	if spec.Readiness != nil && spec.Readiness.SSH != nil {
		result.Readiness = &providerv1.ReadinessSpec{
			SSH: &providerv1.SSHReadinessSpec{
				Enabled:    spec.Readiness.SSH.Enabled,
				Timeout:    spec.Readiness.SSH.Timeout,
				User:       spec.Readiness.SSH.User,
				PrivateKey: spec.Readiness.SSH.PrivateKey,
			},
		}
	}

	return result
}

// convertResourceToMap converts a resource to a map[string]any.
func (e *Executor) convertResourceToMap(resource any) (map[string]any, error) {
	if resource == nil {
		return nil, nil
	}

	// If it's already a map, return it
	if m, ok := resource.(map[string]any); ok {
		return m, nil
	}

	// Otherwise, convert via JSON
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

// updateResourceState updates the state for a specific resource.
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

	// Initialize maps if needed
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

// getResourceState retrieves the state for a specific resource.
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

// updateTemplateContext updates the template context with data from a created resource.
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

// getString safely extracts a string from a map.
func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
