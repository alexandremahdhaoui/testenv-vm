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
)

// ValidKeyTypes defines the allowed key types.
var ValidKeyTypes = map[string]bool{
	"rsa":     true,
	"ed25519": true,
	"ecdsa":   true,
}

// Validate validates an entire TestenvSpec.
// It checks all providers, keys, networks, and VMs for correctness,
// as well as cross-references between resources.
func Validate(spec *v1.TestenvSpec) error {
	if spec == nil {
		return fmt.Errorf("spec cannot be nil")
	}

	// Validate providers first (other validations depend on provider names)
	if err := ValidateProviders(spec.Providers); err != nil {
		return fmt.Errorf("providers validation failed: %w", err)
	}

	// Build provider name set for cross-reference validation
	providerNames := make(map[string]bool)
	for _, p := range spec.Providers {
		providerNames[p.Name] = true
	}

	// Validate default provider reference if explicitly set
	if spec.DefaultProvider != "" && !providerNames[spec.DefaultProvider] {
		return fmt.Errorf("defaultProvider %q does not match any defined provider", spec.DefaultProvider)
	}

	// Check DefaultProvider/Default:true consistency
	if spec.DefaultProvider != "" {
		for _, p := range spec.Providers {
			if p.Default && p.Name != spec.DefaultProvider {
				return fmt.Errorf("provider %q is marked as default, but defaultProvider is set to %q (these must match)", p.Name, spec.DefaultProvider)
			}
		}
	}

	// Validate keys
	if err := ValidateKeys(spec.Keys); err != nil {
		return fmt.Errorf("keys validation failed: %w", err)
	}

	// Validate networks
	if err := ValidateNetworks(spec.Networks); err != nil {
		return fmt.Errorf("networks validation failed: %w", err)
	}

	// Validate VMs
	if err := ValidateVMs(spec.VMs); err != nil {
		return fmt.Errorf("vms validation failed: %w", err)
	}

	// Validate cross-references: provider references in resources
	if err := validateProviderRefs(spec, providerNames); err != nil {
		return err
	}

	// Validate cross-references: resource references (network.AttachTo, vm.Network)
	if err := validateResourceRefs(spec); err != nil {
		return err
	}

	return nil
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
func validateResourceRefs(spec *v1.TestenvSpec) error {
	// Build network name set
	networkNames := make(map[string]bool)
	for _, n := range spec.Networks {
		networkNames[n.Name] = true
	}

	// Check network AttachTo references
	for _, n := range spec.Networks {
		if n.Spec.AttachTo != "" {
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
		if vm.Spec.Network != "" && !networkNames[vm.Spec.Network] {
			return fmt.Errorf("vm %q: network %q not found", vm.Name, vm.Spec.Network)
		}
	}

	return nil
}
