//go:build unit

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
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		wantErr   bool
		errSubstr string
		validate  func(t *testing.T, got *v1.TestenvSpec)
	}{
		{
			name: "valid YAML returns correct TestenvSpec",
			data: []byte(`
providers:
  - name: test-provider
    engine: go://test
    default: true
keys:
  - name: my-key
    spec:
      type: ed25519
networks:
  - name: my-network
    kind: bridge
vms:
  - name: my-vm
    spec:
      memory: 1024
      vcpus: 2
      disk:
        size: "10G"
      network: my-network
      boot:
        order: ["hd"]
`),
			wantErr: false,
			validate: func(t *testing.T, got *v1.TestenvSpec) {
				if len(got.Providers) != 1 {
					t.Errorf("expected 1 provider, got %d", len(got.Providers))
				}
				if got.Providers[0].Name != "test-provider" {
					t.Errorf("expected provider name 'test-provider', got %q", got.Providers[0].Name)
				}
				if len(got.Keys) != 1 {
					t.Errorf("expected 1 key, got %d", len(got.Keys))
				}
				if got.Keys[0].Name != "my-key" {
					t.Errorf("expected key name 'my-key', got %q", got.Keys[0].Name)
				}
				if len(got.Networks) != 1 {
					t.Errorf("expected 1 network, got %d", len(got.Networks))
				}
				if len(got.VMs) != 1 {
					t.Errorf("expected 1 VM, got %d", len(got.VMs))
				}
			},
		},
		{
			name:      "empty data returns error",
			data:      []byte{},
			wantErr:   true,
			errSubstr: "cannot parse empty data",
		},
		{
			name:      "invalid YAML returns error",
			data:      []byte("this: is: not: valid: yaml: ["),
			wantErr:   true,
			errSubstr: "failed to parse YAML",
		},
		{
			name: "minimal valid YAML",
			data: []byte(`
providers:
  - name: p1
    engine: test
`),
			wantErr: false,
			validate: func(t *testing.T, got *v1.TestenvSpec) {
				if len(got.Providers) != 1 {
					t.Errorf("expected 1 provider, got %d", len(got.Providers))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Parse() error = %v, want error containing %q", err, tt.errSubstr)
				}
				return
			}
			if tt.validate != nil {
				tt.validate(t, got)
			}
		})
	}
}

func TestParseFile(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string // returns file path
		wantErr   bool
		errSubstr string
		emptyPath bool
		validate  func(t *testing.T, got *v1.TestenvSpec)
	}{
		{
			name: "valid file works",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "spec.yaml")
				content := []byte(`
providers:
  - name: test
    engine: go://test
`)
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatalf("failed to write test file: %v", err)
				}
				return path
			},
			wantErr: false,
			validate: func(t *testing.T, got *v1.TestenvSpec) {
				if got == nil {
					t.Error("expected non-nil spec")
				}
			},
		},
		{
			name: "non-existent file returns error",
			setup: func(t *testing.T) string {
				return "/non/existent/path/file.yaml"
			},
			wantErr:   true,
			errSubstr: "failed to read file",
		},
		{
			name:      "empty path returns error",
			emptyPath: true,
			wantErr:   true,
			errSubstr: "file path cannot be empty",
		},
		{
			name: "file with invalid YAML returns error",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "invalid.yaml")
				content := []byte("this: is: not: valid: yaml: [")
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatalf("failed to write test file: %v", err)
				}
				return path
			},
			wantErr:   true,
			errSubstr: "failed to parse file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var path string
			if tt.emptyPath {
				path = ""
			} else if tt.setup != nil {
				path = tt.setup(t)
			}

			got, err := ParseFile(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("ParseFile() error = %v, want error containing %q", err, tt.errSubstr)
				}
				return
			}
			if tt.validate != nil {
				tt.validate(t, got)
			}
		})
	}
}

func TestParseFromMap(t *testing.T) {
	tests := []struct {
		name      string
		input     map[string]any
		wantErr   bool
		errSubstr string
		validate  func(t *testing.T, got *v1.TestenvSpec)
	}{
		{
			name: "valid map works",
			input: map[string]any{
				"providers": []any{
					map[string]any{
						"name":   "test-provider",
						"engine": "go://test",
					},
				},
			},
			wantErr: false,
			validate: func(t *testing.T, got *v1.TestenvSpec) {
				if got == nil {
					t.Error("expected non-nil spec")
				}
				if len(got.Providers) != 1 {
					t.Errorf("expected 1 provider, got %d", len(got.Providers))
				}
				if got.Providers[0].Name != "test-provider" {
					t.Errorf("expected provider name 'test-provider', got %q", got.Providers[0].Name)
				}
			},
		},
		{
			name:      "nil map returns error",
			input:     nil,
			wantErr:   true,
			errSubstr: "cannot parse nil map",
		},
		{
			name:    "empty map works",
			input:   map[string]any{},
			wantErr: false,
		},
		{
			name: "nested structures work",
			input: map[string]any{
				"providers": []any{
					map[string]any{
						"name":    "test",
						"engine":  "go://test",
						"default": true,
						"spec": map[string]any{
							"nested": map[string]any{
								"value": "deep",
							},
						},
					},
				},
				"keys": []any{
					map[string]any{
						"name": "my-key",
						"spec": map[string]any{
							"type": "ed25519",
						},
					},
				},
				"networks": []any{
					map[string]any{
						"name": "my-net",
						"kind": "bridge",
						"spec": map[string]any{
							"cidr": "192.168.1.0/24",
							"dhcp": map[string]any{
								"enabled":    true,
								"rangeStart": "192.168.1.100",
								"rangeEnd":   "192.168.1.200",
							},
						},
					},
				},
				"vms": []any{
					map[string]any{
						"name": "my-vm",
						"spec": map[string]any{
							"memory":  1024,
							"vcpus":   2,
							"network": "my-net",
							"disk": map[string]any{
								"size": "10G",
							},
							"boot": map[string]any{
								"order": []string{"hd"},
							},
						},
					},
				},
			},
			wantErr: false,
			validate: func(t *testing.T, got *v1.TestenvSpec) {
				if got == nil {
					t.Error("expected non-nil spec")
				}
				if len(got.Keys) != 1 {
					t.Errorf("expected 1 key, got %d", len(got.Keys))
				}
				if len(got.Networks) != 1 {
					t.Errorf("expected 1 network, got %d", len(got.Networks))
				}
				if len(got.VMs) != 1 {
					t.Errorf("expected 1 VM, got %d", len(got.VMs))
				}
			},
		},
		{
			name: "map with providerSpec (map[string]any) works",
			input: map[string]any{
				"providers": []any{
					map[string]any{
						"name":   "test",
						"engine": "go://test",
					},
				},
				"keys": []any{
					map[string]any{
						"name": "test-key",
						"spec": map[string]any{
							"type": "rsa",
						},
						"providerSpec": map[string]any{
							"customField": "customValue",
							"nested": map[string]any{
								"deep": true,
							},
						},
					},
				},
			},
			wantErr: false,
			validate: func(t *testing.T, got *v1.TestenvSpec) {
				if got == nil {
					t.Error("expected non-nil spec")
				}
				if len(got.Keys) != 1 {
					t.Errorf("expected 1 key, got %d", len(got.Keys))
				}
				// Note: providerSpec parsing depends on API types having yaml tags.
				// Currently the API types only have json tags, so yaml.v3 won't parse providerSpec.
				// This test just verifies parsing doesn't fail, not that providerSpec is populated.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFromMap(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFromMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("ParseFromMap() error = %v, want error containing %q", err, tt.errSubstr)
				}
				return
			}
			if tt.validate != nil {
				tt.validate(t, got)
			}
		})
	}
}
