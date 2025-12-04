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
	"os"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"gopkg.in/yaml.v3"
)

// Parse parses YAML bytes into a TestenvSpec.
// It returns an error if the YAML is invalid or cannot be unmarshaled.
func Parse(data []byte) (*v1.TestenvSpec, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("cannot parse empty data")
	}

	var spec v1.TestenvSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &spec, nil
}

// ParseFile reads a YAML file and parses it into a TestenvSpec.
// It returns an error if the file cannot be read or the YAML is invalid.
func ParseFile(path string) (*v1.TestenvSpec, error) {
	if path == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %q: %w", path, err)
	}

	spec, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file %q: %w", path, err)
	}

	return spec, nil
}

// ParseFromMap converts a map[string]any (typically from Forge CreateInput.Spec)
// into a TestenvSpec. This is the primary method used when receiving input from
// Forge, as Forge passes the spec as a map rather than raw YAML bytes.
//
// The implementation marshals the map back to YAML and then parses it,
// which is the simplest and most reliable approach for handling nested structures.
func ParseFromMap(m map[string]any) (*v1.TestenvSpec, error) {
	if m == nil {
		return nil, fmt.Errorf("cannot parse nil map")
	}

	// Marshal the map to YAML bytes
	data, err := yaml.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal map to YAML: %w", err)
	}

	// Parse the YAML bytes
	spec, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse map: %w", err)
	}

	return spec, nil
}
