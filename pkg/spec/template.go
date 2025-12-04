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

// Package spec provides specification parsing, validation, and template rendering.
package spec

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"text/template"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

// TemplateContext holds the data available for template rendering.
// It is populated incrementally as resources are created.
type TemplateContext struct {
	// Keys contains template data for key resources, keyed by resource name.
	Keys map[string]KeyTemplateData
	// Networks contains template data for network resources, keyed by resource name.
	Networks map[string]NetworkTemplateData
	// VMs contains template data for VM resources, keyed by resource name.
	VMs map[string]VMTemplateData
	// Env contains environment variables available for templates.
	Env map[string]string
}

// KeyTemplateData contains the template-accessible fields for a key resource.
type KeyTemplateData struct {
	// PublicKey is the full public key content (e.g., "ssh-ed25519 AAAA...").
	PublicKey string
	// PrivateKeyPath is the file path to the private key.
	PrivateKeyPath string
	// PublicKeyPath is the file path to the public key.
	PublicKeyPath string
	// Fingerprint is the key fingerprint (e.g., "SHA256:...").
	Fingerprint string
}

// NetworkTemplateData contains the template-accessible fields for a network resource.
type NetworkTemplateData struct {
	// Name is the network name (may differ from resource name).
	Name string
	// IP is the gateway/interface IP address.
	IP string
	// CIDR is the network CIDR (e.g., "192.168.100.0/24").
	CIDR string
	// InterfaceName is the OS-level interface name (for bridges).
	InterfaceName string
	// UUID is the provider-specific unique identifier.
	UUID string
}

// VMTemplateData contains the template-accessible fields for a VM resource.
type VMTemplateData struct {
	// Name is the VM name.
	Name string
	// IP is the assigned IP address.
	IP string
	// MAC is the MAC address.
	MAC string
	// SSHCommand is a ready-to-use SSH command to connect to the VM.
	SSHCommand string
}

// NewTemplateContext creates a new empty TemplateContext with initialized maps.
func NewTemplateContext() *TemplateContext {
	return &TemplateContext{
		Keys:     make(map[string]KeyTemplateData),
		Networks: make(map[string]NetworkTemplateData),
		VMs:      make(map[string]VMTemplateData),
		Env:      make(map[string]string),
	}
}

// hyphenKeyPattern matches template expressions like .Keys.name-with-hyphens.Field
// and converts them to use index function: (index .Keys "name-with-hyphens").Field
var hyphenKeyPattern = regexp.MustCompile(`\.(Keys|Networks|VMs)\.([a-zA-Z0-9][a-zA-Z0-9_-]*[a-zA-Z0-9_-])\.(\w+)`)

// preprocessTemplate converts dot notation with hyphens to use index function.
// For example: {{ .Keys.test-key.PublicKey }} -> {{ (index .Keys "test-key").PublicKey }}
// This is necessary because Go templates don't support hyphens in identifiers.
func preprocessTemplate(tmpl string) string {
	return hyphenKeyPattern.ReplaceAllStringFunc(tmpl, func(match string) string {
		parts := hyphenKeyPattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		category := parts[1] // Keys, Networks, or VMs
		name := parts[2]     // the key name (may contain hyphens)
		field := parts[3]    // PublicKey, IP, etc.

		// Only use index if the name contains a hyphen
		if strings.Contains(name, "-") {
			return fmt.Sprintf(`(index .%s "%s").%s`, category, name, field)
		}
		return match
	})
}

// RenderString renders a single string using Go template syntax.
// If the string does not contain template syntax, it is returned unchanged.
// Automatically converts dot notation with hyphens to use index function.
// Returns an error if the template is invalid or fails to execute.
func RenderString(tmpl string, ctx *TemplateContext) (string, error) {
	// Quick check: if no template delimiters, return as-is
	if !strings.Contains(tmpl, "{{") {
		return tmpl, nil
	}

	// Preprocess to handle hyphens in key names
	processedTmpl := preprocessTemplate(tmpl)

	t, err := template.New("tmpl").Parse(processedTmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %q: %w", tmpl, err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("failed to execute template %q: %w", tmpl, err)
	}

	return buf.String(), nil
}

// RenderSpec renders all string fields in a struct recursively.
// It modifies the struct in place, replacing template strings with rendered values.
// The spec must be a pointer to a struct.
func RenderSpec(spec interface{}, ctx *TemplateContext) error {
	return renderValue(reflect.ValueOf(spec), ctx)
}

// renderValue recursively renders string fields in a reflect.Value.
func renderValue(v reflect.Value, ctx *TemplateContext) error {
	// Handle pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		return renderValue(v.Elem(), ctx)
	}

	// Handle interfaces
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		return renderValue(v.Elem(), ctx)
	}

	switch v.Kind() {
	case reflect.String:
		if v.CanSet() {
			rendered, err := RenderString(v.String(), ctx)
			if err != nil {
				return err
			}
			v.SetString(rendered)
		}

	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			if err := renderValue(field, ctx); err != nil {
				return err
			}
		}

	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if err := renderValue(v.Index(i), ctx); err != nil {
				return err
			}
		}

	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := renderValue(v.Index(i), ctx); err != nil {
				return err
			}
		}

	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key()
			val := iter.Value()

			// Map values are not addressable, so we need to render and replace
			rendered, err := renderMapValue(val, ctx)
			if err != nil {
				return err
			}
			if rendered != nil {
				v.SetMapIndex(key, reflect.ValueOf(rendered))
			}
		}
	}

	return nil
}

// renderMapValue recursively renders templates in a map value.
// Because map values are not addressable, we return a new value.
// Returns nil if the value should not be replaced (non-string types that weren't modified).
func renderMapValue(v reflect.Value, ctx *TemplateContext) (interface{}, error) {
	// Unwrap interfaces
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, nil
		}
		return renderMapValue(v.Elem(), ctx)
	}

	switch v.Kind() {
	case reflect.String:
		rendered, err := RenderString(v.String(), ctx)
		if err != nil {
			return nil, err
		}
		return rendered, nil

	case reflect.Map:
		newMap := reflect.MakeMap(v.Type())
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key()
			val := iter.Value()
			renderedVal, err := renderMapValue(val, ctx)
			if err != nil {
				return nil, err
			}
			if renderedVal != nil {
				newMap.SetMapIndex(key, reflect.ValueOf(renderedVal))
			} else {
				newMap.SetMapIndex(key, val)
			}
		}
		return newMap.Interface(), nil

	case reflect.Slice:
		newSlice := reflect.MakeSlice(v.Type(), v.Len(), v.Cap())
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i)
			renderedElem, err := renderMapValue(elem, ctx)
			if err != nil {
				return nil, err
			}
			if renderedElem != nil {
				newSlice.Index(i).Set(reflect.ValueOf(renderedElem))
			} else {
				newSlice.Index(i).Set(elem)
			}
		}
		return newSlice.Interface(), nil

	default:
		// For other types (int, bool, struct, etc.), return nil to keep original
		return nil, nil
	}
}

// templateRefPattern matches template references like {{ .Keys.name.Field }} or {{ .Networks.name.Field }}
// Group 1: Category (Keys, Networks, VMs)
// Group 2: Resource name
var templateRefPattern = regexp.MustCompile(`\{\{\s*\.(\w+)\.([^.}\s]+)`)

// ExtractTemplateRefs extracts resource references from template strings in a spec.
// It scans all string fields recursively and returns ResourceRefs for any
// template variables that reference Keys, Networks, or VMs.
func ExtractTemplateRefs(spec interface{}) []v1.ResourceRef {
	var refs []v1.ResourceRef
	seen := make(map[string]bool)

	extractFromValue(reflect.ValueOf(spec), &refs, seen)

	return refs
}

// extractFromValue recursively extracts template references from a reflect.Value.
func extractFromValue(v reflect.Value, refs *[]v1.ResourceRef, seen map[string]bool) {
	// Handle pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		extractFromValue(v.Elem(), refs, seen)
		return
	}

	// Handle interfaces
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}
		extractFromValue(v.Elem(), refs, seen)
		return
	}

	switch v.Kind() {
	case reflect.String:
		extractFromString(v.String(), refs, seen)

	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			extractFromValue(v.Field(i), refs, seen)
		}

	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			extractFromValue(v.Index(i), refs, seen)
		}

	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			extractFromValue(iter.Value(), refs, seen)
		}
	}
}

// extractFromString extracts template references from a single string.
func extractFromString(s string, refs *[]v1.ResourceRef, seen map[string]bool) {
	matches := templateRefPattern.FindAllStringSubmatch(s, -1)
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		category := match[1]
		name := match[2]

		// Map category to resource kind
		var kind string
		switch category {
		case "Keys":
			kind = "key"
		case "Networks":
			kind = "network"
		case "VMs":
			kind = "vm"
		default:
			// Skip unknown categories (e.g., Env)
			continue
		}

		// Create a unique key to avoid duplicates
		key := kind + ":" + name
		if seen[key] {
			continue
		}
		seen[key] = true

		*refs = append(*refs, v1.ResourceRef{
			Kind: kind,
			Name: name,
		})
	}
}
