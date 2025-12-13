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

// Package spec provides parsing, validation, and template rendering for
// testenv-vm specifications.
package spec

import (
	"fmt"
	"strings"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"github.com/alexandremahdhaoui/testenv-vm/pkg/image"
)

// ValidKeyTypes defines the allowed key types.
var ValidKeyTypes = map[string]bool{
	"rsa":     true,
	"ed25519": true,
	"ecdsa":   true,
}

// IsTemplated checks if a string contains Go template syntax.
// Returns true if the string contains "{{" delimiter.
func IsTemplated(s string) bool {
	return strings.Contains(s, "{{")
}

// TemplatedFields tracks which resource fields contained template syntax
// during Phase 1 validation. These fields require Phase 2 validation
// after template rendering.
type TemplatedFields struct {
	// NetworkAttachTo maps network names to whether their attachTo field was templated
	NetworkAttachTo map[string]bool
	// VMNetwork maps VM names to whether their network field was templated
	VMNetwork map[string]bool
}

// NewTemplatedFields creates a new TemplatedFields with initialized maps.
func NewTemplatedFields() *TemplatedFields {
	return &TemplatedFields{
		NetworkAttachTo: make(map[string]bool),
		VMNetwork:       make(map[string]bool),
	}
}

// ValidateEarly performs Phase 1 validation on a TestenvSpec.
// It validates structure, syntax, and verifies template references point to
// resources that exist in the spec. Templated fields are marked for Phase 2
// validation after template rendering.
// Returns TemplatedFields indicating which fields need Phase 2 validation.
func ValidateEarly(spec *v1.TestenvSpec) (*TemplatedFields, error) {
	if spec == nil {
		return nil, fmt.Errorf("spec cannot be nil")
	}

	templatedFields := NewTemplatedFields()

	// Validate providers first (other validations depend on provider names)
	if err := ValidateProviders(spec.Providers); err != nil {
		return nil, fmt.Errorf("providers validation failed: %w", err)
	}

	// Build provider name set for cross-reference validation
	providerNames := make(map[string]bool)
	for _, p := range spec.Providers {
		providerNames[p.Name] = true
	}

	// Validate default provider reference if explicitly set
	if spec.DefaultProvider != "" && !providerNames[spec.DefaultProvider] {
		return nil, fmt.Errorf("defaultProvider %q does not match any defined provider", spec.DefaultProvider)
	}

	// Check DefaultProvider/Default:true consistency
	if spec.DefaultProvider != "" {
		for _, p := range spec.Providers {
			if p.Default && p.Name != spec.DefaultProvider {
				return nil, fmt.Errorf("provider %q is marked as default, but defaultProvider is set to %q (these must match)", p.Name, spec.DefaultProvider)
			}
		}
	}

	// Validate keys
	if err := ValidateKeys(spec.Keys); err != nil {
		return nil, fmt.Errorf("keys validation failed: %w", err)
	}

	// Validate networks
	if err := ValidateNetworks(spec.Networks); err != nil {
		return nil, fmt.Errorf("networks validation failed: %w", err)
	}

	// Validate VMs
	if err := ValidateVMs(spec.VMs); err != nil {
		return nil, fmt.Errorf("vms validation failed: %w", err)
	}

	// Validate images
	if err := validateImages(spec); err != nil {
		return nil, fmt.Errorf("images validation failed: %w", err)
	}

	// Validate cross-references: provider references in resources
	if err := validateProviderRefs(spec, providerNames); err != nil {
		return nil, err
	}

	// Validate template references point to existing resources
	if err := validateTemplateRefsExist(spec); err != nil {
		return nil, err
	}

	// Validate cross-references: resource references (network.AttachTo, vm.Network)
	// Modified to skip templated fields and mark them for Phase 2 validation
	if err := validateResourceRefs(spec, templatedFields); err != nil {
		return nil, err
	}

	return templatedFields, nil
}

// Validate validates an entire TestenvSpec.
// This is a convenience wrapper around ValidateEarly that discards the
// TemplatedFields information. Use ValidateEarly directly if you need
// to know which fields require Phase 2 validation.
func Validate(spec *v1.TestenvSpec) error {
	_, err := ValidateEarly(spec)
	return err
}

// ValidateProviders validates provider configurations.
// It ensures:
// - At least one provider is defined
// - Provider names are unique
// - Each provider has name and engine fields
// - A default provider exists (either via DefaultProvider field or a provider marked default)
func ValidateProviders(providers []v1.ProviderConfig) error {
	if len(providers) == 0 {
		return fmt.Errorf("at least one provider must be defined")
	}

	seen := make(map[string]bool)
	defaultCount := 0

	for i, p := range providers {
		// Check required fields
		if p.Name == "" {
			return fmt.Errorf("provider at index %d: name is required", i)
		}
		if p.Engine == "" {
			return fmt.Errorf("provider %q: engine is required", p.Name)
		}

		// Check for duplicate names
		if seen[p.Name] {
			return fmt.Errorf("provider %q: duplicate provider name", p.Name)
		}
		seen[p.Name] = true

		// Count default providers
		if p.Default {
			defaultCount++
		}
	}

	// Validate default provider exists
	// A default is required if there's more than one provider
	// (with one provider, it's implicitly the default)
	if len(providers) > 1 && defaultCount == 0 {
		return fmt.Errorf("multiple providers defined but no default provider specified (set 'default: true' on one provider)")
	}
	if defaultCount > 1 {
		return fmt.Errorf("multiple providers marked as default (only one provider can be default)")
	}

	return nil
}

// ValidateKeys validates key resource configurations.
// It ensures:
// - Resource names are unique within keys
// - Each key has a name field
// - Key type is one of: rsa, ed25519, ecdsa
func ValidateKeys(keys []v1.KeyResource) error {
	seen := make(map[string]bool)

	for i, k := range keys {
		// Check required fields
		if k.Name == "" {
			return fmt.Errorf("key at index %d: name is required", i)
		}

		// Check for duplicate names
		if seen[k.Name] {
			return fmt.Errorf("key %q: duplicate key name", k.Name)
		}
		seen[k.Name] = true

		// Validate key type
		if k.Spec.Type == "" {
			return fmt.Errorf("key %q: spec.type is required", k.Name)
		}
		keyType := strings.ToLower(k.Spec.Type)
		if !ValidKeyTypes[keyType] {
			return fmt.Errorf("key %q: invalid key type %q (must be one of: rsa, ed25519, ecdsa)", k.Name, k.Spec.Type)
		}
	}

	return nil
}

// ValidateNetworks validates network resource configurations.
// It ensures:
// - Resource names are unique within networks
// - Each network has name and kind fields
// - CIDR is required for networks with DHCP enabled
func ValidateNetworks(networks []v1.NetworkResource) error {
	seen := make(map[string]bool)

	for i, n := range networks {
		// Check required fields
		if n.Name == "" {
			return fmt.Errorf("network at index %d: name is required", i)
		}
		if n.Kind == "" {
			return fmt.Errorf("network %q: kind is required", n.Name)
		}

		// Check for duplicate names
		if seen[n.Name] {
			return fmt.Errorf("network %q: duplicate network name", n.Name)
		}
		seen[n.Name] = true

		// If DHCP is enabled, CIDR is required
		if n.Spec.DHCP != nil && n.Spec.DHCP.Enabled && n.Spec.CIDR == "" {
			return fmt.Errorf("network %q: cidr is required when DHCP is enabled", n.Name)
		}
	}

	return nil
}

// ValidateVMs validates VM resource configurations.
// It ensures:
// - Resource names are unique within VMs
// - Each VM has a name field
// - Memory and VCPUs are positive values
func ValidateVMs(vms []v1.VMResource) error {
	seen := make(map[string]bool)

	for i, vm := range vms {
		// Check required fields
		if vm.Name == "" {
			return fmt.Errorf("vm at index %d: name is required", i)
		}

		// Check for duplicate names
		if seen[vm.Name] {
			return fmt.Errorf("vm %q: duplicate vm name", vm.Name)
		}
		seen[vm.Name] = true

		// Validate memory is positive
		if vm.Spec.Memory <= 0 {
			return fmt.Errorf("vm %q: memory must be a positive value (got %d)", vm.Name, vm.Spec.Memory)
		}

		// Validate VCPUs is positive
		if vm.Spec.VCPUs <= 0 {
			return fmt.Errorf("vm %q: vcpus must be a positive value (got %d)", vm.Name, vm.Spec.VCPUs)
		}
	}

	return nil
}

// validateProviderRefs validates that all provider references in resources
// refer to existing provider names.
func validateProviderRefs(spec *v1.TestenvSpec, providerNames map[string]bool) error {
	// Check keys
	for _, k := range spec.Keys {
		if k.Provider != "" && !providerNames[k.Provider] {
			return fmt.Errorf("key %q: provider %q not found", k.Name, k.Provider)
		}
	}

	// Check networks
	for _, n := range spec.Networks {
		if n.Provider != "" && !providerNames[n.Provider] {
			return fmt.Errorf("network %q: provider %q not found", n.Name, n.Provider)
		}
	}

	// Check VMs
	for _, vm := range spec.VMs {
		if vm.Provider != "" && !providerNames[vm.Provider] {
			return fmt.Errorf("vm %q: provider %q not found", vm.Name, vm.Provider)
		}
	}

	return nil
}

// validateResourceRefs validates cross-references between resources.
// It checks that network.AttachTo and vm.Network reference existing networks.
// Templated fields are skipped and marked in templatedFields for Phase 2 validation.
func validateResourceRefs(spec *v1.TestenvSpec, templatedFields *TemplatedFields) error {
	// Build network name set
	networkNames := make(map[string]bool)
	for _, n := range spec.Networks {
		networkNames[n.Name] = true
	}

	// Check network AttachTo references
	for _, n := range spec.Networks {
		if n.Spec.AttachTo != "" {
			if IsTemplated(n.Spec.AttachTo) {
				// Mark for Phase 2 validation
				templatedFields.NetworkAttachTo[n.Name] = true
				continue
			}
			// Literal value - validate now
			if n.Spec.AttachTo == n.Name {
				return fmt.Errorf("network %q: attachTo cannot reference itself", n.Name)
			}
			if !networkNames[n.Spec.AttachTo] {
				return fmt.Errorf("network %q: attachTo references non-existent network %q", n.Name, n.Spec.AttachTo)
			}
		}
	}

	// Check VM network references
	for _, vm := range spec.VMs {
		if vm.Spec.Network != "" {
			if IsTemplated(vm.Spec.Network) {
				// Mark for Phase 2 validation
				templatedFields.VMNetwork[vm.Name] = true
				continue
			}
			// Literal value - validate now
			if !networkNames[vm.Spec.Network] {
				return fmt.Errorf("vm %q: network %q not found", vm.Name, vm.Spec.Network)
			}
		}
	}

	return nil
}

// validateImages validates image resource configurations.
// It ensures:
// - Each image has a non-empty Name
// - Each image has a non-empty Source
// - Source is either a well-known reference or an HTTPS URL
// - Custom URLs (non-well-known) require SHA256 checksum
// - No duplicate image names
// - Aliases don't conflict with other names/aliases
func validateImages(spec *v1.TestenvSpec) error {
	seen := make(map[string]bool)      // tracks image names
	aliases := make(map[string]string) // maps alias -> image name that owns it

	for i, img := range spec.Images {
		// Check required name
		if img.Name == "" {
			return fmt.Errorf("image at index %d: name is required", i)
		}

		// Check for duplicate names
		if seen[img.Name] {
			return fmt.Errorf("duplicate image name: %q", img.Name)
		}
		seen[img.Name] = true

		// Check required source
		if img.Spec.Source == "" {
			return fmt.Errorf("image %q: source is required", img.Name)
		}

		// Validate source is either well-known or HTTPS URL
		if !image.IsWellKnown(img.Spec.Source) {
			// Not a well-known image - must be an HTTPS URL
			if !strings.HasPrefix(img.Spec.Source, "https://") {
				return fmt.Errorf("image %q: source %q must be well-known reference or HTTPS URL", img.Name, img.Spec.Source)
			}
			// Custom HTTPS URLs require SHA256 checksum
			if img.Spec.SHA256 == "" {
				return fmt.Errorf("image %q: custom URL requires sha256 checksum", img.Name)
			}
		}

		// Validate alias doesn't conflict with names or other aliases
		if img.Spec.Alias != "" {
			// Check if alias conflicts with an image name
			if seen[img.Spec.Alias] {
				return fmt.Errorf("image %q: alias %q conflicts with another image name", img.Name, img.Spec.Alias)
			}
			// Check if alias conflicts with another alias
			if owner, ok := aliases[img.Spec.Alias]; ok {
				return fmt.Errorf("image %q: alias %q conflicts with alias from image %q", img.Name, img.Spec.Alias, owner)
			}
			aliases[img.Spec.Alias] = img.Name
		}
	}

	// Second pass: check that no image name conflicts with an alias from a previous image
	// (alias declared before the name was processed)
	for _, img := range spec.Images {
		if owner, ok := aliases[img.Name]; ok && owner != img.Name {
			return fmt.Errorf("image %q: name conflicts with alias from image %q", img.Name, owner)
		}
	}

	// Validate ImageCacheDir if set (basic path validation)
	if spec.ImageCacheDir != "" {
		// Basic validation: must not be empty after trim
		if strings.TrimSpace(spec.ImageCacheDir) == "" {
			return fmt.Errorf("imageCacheDir cannot be whitespace-only")
		}
	}

	return nil
}

// validateTemplateRefsExist validates that all template references in the spec
// point to resources that actually exist in the spec. This catches typos like
// {{ .Networks.typo.InterfaceName }} early, before any resources are created.
// Note: .Env references are skipped as they come from runtime input.
func validateTemplateRefsExist(spec *v1.TestenvSpec) error {
	// Build resource name sets
	keyNames := make(map[string]bool)
	for _, k := range spec.Keys {
		keyNames[k.Name] = true
	}
	networkNames := make(map[string]bool)
	for _, n := range spec.Networks {
		networkNames[n.Name] = true
	}
	vmNames := make(map[string]bool)
	for _, vm := range spec.VMs {
		vmNames[vm.Name] = true
	}
	// Build image names set (include both name and alias)
	imageNames := make(map[string]bool)
	for _, img := range spec.Images {
		imageNames[img.Name] = true
		if img.Spec.Alias != "" {
			imageNames[img.Spec.Alias] = true
		}
	}

	// Extract all template refs from spec
	refs := ExtractTemplateRefs(spec)

	// Validate each ref exists
	for _, ref := range refs {
		switch ref.Kind {
		case "key":
			if !keyNames[ref.Name] {
				return fmt.Errorf("template reference to non-existent key %q", ref.Name)
			}
		case "network":
			if !networkNames[ref.Name] {
				return fmt.Errorf("template reference to non-existent network %q", ref.Name)
			}
		case "vm":
			if !vmNames[ref.Name] {
				return fmt.Errorf("template reference to non-existent vm %q", ref.Name)
			}
		case "image":
			if !imageNames[ref.Name] {
				return fmt.Errorf("template reference to non-existent image %q", ref.Name)
			}
		}
	}

	return nil
}

// ValidateResourceRefsLate performs Phase 2 validation on a rendered resource.
// It validates only fields that were templated in Phase 1, after template
// rendering has resolved the values.
//
// For network attachTo: accepts any non-empty resolved value. The provider
// is responsible for validating that the interface exists on the host.
//
// For vm.Network: verifies the resolved value is a valid network name from
// the spec, since VMs must reference networks by their logical name.
func ValidateResourceRefsLate(
	resourceKind string,
	resourceName string,
	renderedSpec interface{},
	fullSpec *v1.TestenvSpec,
	templatedFields *TemplatedFields,
) error {
	if templatedFields == nil {
		return nil
	}

	switch resourceKind {
	case "network":
		// Check if attachTo was templated for this network
		if !templatedFields.NetworkAttachTo[resourceName] {
			return nil // Not templated, already validated in Phase 1
		}

		// Get rendered attachTo value
		networkSpec, ok := renderedSpec.(*v1.NetworkResource)
		if !ok {
			return fmt.Errorf("invalid network spec type")
		}

		attachTo := networkSpec.Spec.AttachTo
		if attachTo == "" {
			return nil // Empty is valid (optional field)
		}

		// Check for self-reference after rendering
		if attachTo == resourceName {
			return fmt.Errorf("network %q: rendered attachTo cannot reference itself", resourceName)
		}

		// For templated attachTo: accept non-empty value
		// Provider validates if interface exists on host
		return nil

	case "vm":
		// Check if network was templated for this VM
		if !templatedFields.VMNetwork[resourceName] {
			return nil // Not templated, already validated in Phase 1
		}

		// Get rendered network value
		vmSpec, ok := renderedSpec.(*v1.VMResource)
		if !ok {
			return fmt.Errorf("invalid vm spec type")
		}

		network := vmSpec.Spec.Network
		if network == "" {
			return nil // Empty is valid (optional field)
		}

		// Build network name set
		networkNames := make(map[string]bool)
		for _, n := range fullSpec.Networks {
			networkNames[n.Name] = true
		}

		// Verify resolved value is a valid network name
		if !networkNames[network] {
			var available []string
			for name := range networkNames {
				available = append(available, name)
			}
			return fmt.Errorf("vm %q: rendered network value %q is not a valid network name (available: %v)",
				resourceName, network, available)
		}

		return nil
	}

	return nil
}
