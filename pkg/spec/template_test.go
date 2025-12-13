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
	"strings"
	"testing"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

func TestRenderString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		ctx       *TemplateContext
		want      string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "no template returns unchanged",
			input:   "plain string without templates",
			ctx:     NewTemplateContext(),
			want:    "plain string without templates",
			wantErr: false,
		},
		{
			name:    "empty string returns unchanged",
			input:   "",
			ctx:     NewTemplateContext(),
			want:    "",
			wantErr: false,
		},
		{
			name: "valid template renders correctly - key reference",
			input: "{{ .Keys.mykey.PublicKey }}",
			ctx: func() *TemplateContext {
				ctx := NewTemplateContext()
				ctx.Keys["mykey"] = KeyTemplateData{
					PublicKey:      "ssh-ed25519 AAAA...",
					PrivateKeyPath: "/tmp/key",
					PublicKeyPath:  "/tmp/key.pub",
					Fingerprint:    "SHA256:abc123",
				}
				return ctx
			}(),
			want:    "ssh-ed25519 AAAA...",
			wantErr: false,
		},
		{
			name: "valid template renders correctly - network reference",
			input: "{{ .Networks.mynet.IP }}",
			ctx: func() *TemplateContext {
				ctx := NewTemplateContext()
				ctx.Networks["mynet"] = NetworkTemplateData{
					Name:          "mynet",
					IP:            "192.168.1.1",
					CIDR:          "192.168.1.0/24",
					InterfaceName: "br0",
					UUID:          "uuid-123",
				}
				return ctx
			}(),
			want:    "192.168.1.1",
			wantErr: false,
		},
		{
			name: "valid template renders correctly - VM reference",
			input: "{{ .VMs.myvm.SSHCommand }}",
			ctx: func() *TemplateContext {
				ctx := NewTemplateContext()
				ctx.VMs["myvm"] = VMTemplateData{
					Name:       "myvm",
					IP:         "192.168.1.100",
					MAC:        "00:11:22:33:44:55",
					SSHCommand: "ssh user@192.168.1.100",
				}
				return ctx
			}(),
			want:    "ssh user@192.168.1.100",
			wantErr: false,
		},
		{
			name: "valid template renders correctly - env reference",
			input: "{{ .Env.HOME }}",
			ctx: func() *TemplateContext {
				ctx := NewTemplateContext()
				ctx.Env["HOME"] = "/home/user"
				return ctx
			}(),
			want:    "/home/user",
			wantErr: false,
		},
		{
			name: "template with embedded text renders correctly",
			input: "The key path is {{ .Keys.mykey.PrivateKeyPath }} for SSH",
			ctx: func() *TemplateContext {
				ctx := NewTemplateContext()
				ctx.Keys["mykey"] = KeyTemplateData{
					PrivateKeyPath: "/tmp/key",
				}
				return ctx
			}(),
			want:    "The key path is /tmp/key for SSH",
			wantErr: false,
		},
		{
			name:      "invalid template syntax returns error",
			input:     "{{ .Keys.mykey.PublicKey",
			ctx:       NewTemplateContext(),
			wantErr:   true,
			errSubstr: "failed to parse template",
		},
		{
			name:      "template execution error returns error",
			input:     "{{ .NonExistent.Field }}",
			ctx:       NewTemplateContext(),
			wantErr:   true,
			errSubstr: "failed to execute template",
		},
		{
			name: "multiple templates in one string",
			input: "{{ .Keys.key1.PublicKey }} and {{ .Networks.net1.IP }}",
			ctx: func() *TemplateContext {
				ctx := NewTemplateContext()
				ctx.Keys["key1"] = KeyTemplateData{PublicKey: "pubkey"}
				ctx.Networks["net1"] = NetworkTemplateData{IP: "10.0.0.1"}
				return ctx
			}(),
			want:    "pubkey and 10.0.0.1",
			wantErr: false,
		},
		{
			name:  "image path reference",
			input: "{{ .Images.ubuntu.Path }}",
			ctx: func() *TemplateContext {
				ctx := NewTemplateContext()
				ctx.Images["ubuntu"] = ImageTemplateData{Path: "/cache/ubuntu.qcow2", Name: "ubuntu"}
				return ctx
			}(),
			want:    "/cache/ubuntu.qcow2",
			wantErr: false,
		},
		{
			name:  "default base image",
			input: "{{ .DefaultBaseImage }}",
			ctx: func() *TemplateContext {
				ctx := NewTemplateContext()
				ctx.DefaultBaseImage = "/cache/default.qcow2"
				return ctx
			}(),
			want:    "/cache/default.qcow2",
			wantErr: false,
		},
		{
			name:  "image with hyphens in name",
			input: "{{ .Images.ubuntu-24-04.Path }}",
			ctx: func() *TemplateContext {
				ctx := NewTemplateContext()
				ctx.Images["ubuntu-24-04"] = ImageTemplateData{Path: "/cache/ubuntu-24-04.qcow2", Name: "ubuntu-24-04"}
				return ctx
			}(),
			want:    "/cache/ubuntu-24-04.qcow2",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderString(tt.input, tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("RenderString() error = %v, want error containing %q", err, tt.errSubstr)
				}
				return
			}
			if got != tt.want {
				t.Errorf("RenderString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderSpec(t *testing.T) {
	t.Run("modifies struct fields in place", func(t *testing.T) {
		type SimpleStruct struct {
			Field1 string
			Field2 string
		}
		s := &SimpleStruct{
			Field1: "{{ .Keys.mykey.PublicKey }}",
			Field2: "static value",
		}
		ctx := NewTemplateContext()
		ctx.Keys["mykey"] = KeyTemplateData{PublicKey: "rendered-key"}

		err := RenderSpec(s, ctx)
		if err != nil {
			t.Fatalf("RenderSpec() error = %v", err)
		}

		if s.Field1 != "rendered-key" {
			t.Errorf("Field1 = %q, want %q", s.Field1, "rendered-key")
		}
		if s.Field2 != "static value" {
			t.Errorf("Field2 = %q, want %q", s.Field2, "static value")
		}
	})

	t.Run("handles nested structs", func(t *testing.T) {
		type Inner struct {
			Value string
		}
		type Outer struct {
			Inner Inner
			Name  string
		}
		s := &Outer{
			Name: "{{ .Networks.net1.Name }}",
			Inner: Inner{
				Value: "{{ .Networks.net1.IP }}",
			},
		}
		ctx := NewTemplateContext()
		ctx.Networks["net1"] = NetworkTemplateData{Name: "mynet", IP: "10.0.0.1"}

		err := RenderSpec(s, ctx)
		if err != nil {
			t.Fatalf("RenderSpec() error = %v", err)
		}

		if s.Name != "mynet" {
			t.Errorf("Name = %q, want %q", s.Name, "mynet")
		}
		if s.Inner.Value != "10.0.0.1" {
			t.Errorf("Inner.Value = %q, want %q", s.Inner.Value, "10.0.0.1")
		}
	})

	t.Run("handles slices", func(t *testing.T) {
		type StructWithSlice struct {
			Items []string
		}
		s := &StructWithSlice{
			Items: []string{
				"{{ .Keys.key1.PublicKey }}",
				"static",
				"{{ .Keys.key2.PublicKey }}",
			},
		}
		ctx := NewTemplateContext()
		ctx.Keys["key1"] = KeyTemplateData{PublicKey: "key1-pub"}
		ctx.Keys["key2"] = KeyTemplateData{PublicKey: "key2-pub"}

		err := RenderSpec(s, ctx)
		if err != nil {
			t.Fatalf("RenderSpec() error = %v", err)
		}

		expected := []string{"key1-pub", "static", "key2-pub"}
		for i, want := range expected {
			if s.Items[i] != want {
				t.Errorf("Items[%d] = %q, want %q", i, s.Items[i], want)
			}
		}
	})

	t.Run("handles nested map[string]any", func(t *testing.T) {
		type StructWithMap struct {
			Config map[string]any
		}
		s := &StructWithMap{
			Config: map[string]any{
				"simple": "{{ .Keys.mykey.PublicKey }}",
				"nested": map[string]any{
					"deep": "{{ .Networks.net1.IP }}",
					"deeper": map[string]any{
						"value": "{{ .VMs.vm1.SSHCommand }}",
					},
				},
				"list": []any{
					"{{ .Keys.mykey.PrivateKeyPath }}",
					"static",
				},
			},
		}
		ctx := NewTemplateContext()
		ctx.Keys["mykey"] = KeyTemplateData{
			PublicKey:      "pubkey",
			PrivateKeyPath: "/path/to/key",
		}
		ctx.Networks["net1"] = NetworkTemplateData{IP: "192.168.1.1"}
		ctx.VMs["vm1"] = VMTemplateData{SSHCommand: "ssh vm1"}

		err := RenderSpec(s, ctx)
		if err != nil {
			t.Fatalf("RenderSpec() error = %v", err)
		}

		if s.Config["simple"] != "pubkey" {
			t.Errorf("Config['simple'] = %v, want %q", s.Config["simple"], "pubkey")
		}

		nested, ok := s.Config["nested"].(map[string]any)
		if !ok {
			t.Fatalf("Config['nested'] is not map[string]any")
		}
		if nested["deep"] != "192.168.1.1" {
			t.Errorf("Config['nested']['deep'] = %v, want %q", nested["deep"], "192.168.1.1")
		}

		deeper, ok := nested["deeper"].(map[string]any)
		if !ok {
			t.Fatalf("Config['nested']['deeper'] is not map[string]any")
		}
		if deeper["value"] != "ssh vm1" {
			t.Errorf("Config['nested']['deeper']['value'] = %v, want %q", deeper["value"], "ssh vm1")
		}

		list, ok := s.Config["list"].([]any)
		if !ok {
			t.Fatalf("Config['list'] is not []any")
		}
		if list[0] != "/path/to/key" {
			t.Errorf("Config['list'][0] = %v, want %q", list[0], "/path/to/key")
		}
	})

	t.Run("handles nil pointer", func(t *testing.T) {
		var s *struct{ Field string }
		ctx := NewTemplateContext()

		err := RenderSpec(s, ctx)
		if err != nil {
			t.Errorf("RenderSpec() with nil pointer should not error, got %v", err)
		}
	})

	t.Run("handles slice of structs", func(t *testing.T) {
		type Item struct {
			Name  string
			Value string
		}
		type Container struct {
			Items []Item
		}
		s := &Container{
			Items: []Item{
				{Name: "item1", Value: "{{ .Keys.k1.PublicKey }}"},
				{Name: "item2", Value: "{{ .Keys.k2.PublicKey }}"},
			},
		}
		ctx := NewTemplateContext()
		ctx.Keys["k1"] = KeyTemplateData{PublicKey: "key1"}
		ctx.Keys["k2"] = KeyTemplateData{PublicKey: "key2"}

		err := RenderSpec(s, ctx)
		if err != nil {
			t.Fatalf("RenderSpec() error = %v", err)
		}

		if s.Items[0].Value != "key1" {
			t.Errorf("Items[0].Value = %q, want %q", s.Items[0].Value, "key1")
		}
		if s.Items[1].Value != "key2" {
			t.Errorf("Items[1].Value = %q, want %q", s.Items[1].Value, "key2")
		}
	})

	t.Run("returns error on invalid template", func(t *testing.T) {
		type SimpleStruct struct {
			Field string
		}
		s := &SimpleStruct{
			Field: "{{ .Keys.mykey.PublicKey",
		}
		ctx := NewTemplateContext()

		err := RenderSpec(s, ctx)
		if err == nil {
			t.Error("RenderSpec() should return error for invalid template")
		}
	})
}

func TestExtractTemplateRefs(t *testing.T) {
	t.Run("finds key references", func(t *testing.T) {
		spec := &v1.TestenvSpec{
			VMs: []v1.VMResource{
				{
					Name: "vm1",
					Spec: v1.VMSpec{
						Memory: 1024,
						VCPUs:  2,
						CloudInit: &v1.CloudInitSpec{
							Users: []v1.UserSpec{
								{
									Name:              "ubuntu",
									SSHAuthorizedKeys: []string{"{{ .Keys.my-key.PublicKey }}"},
								},
							},
						},
					},
				},
			},
		}

		refs := ExtractTemplateRefs(spec)

		found := false
		for _, ref := range refs {
			if ref.Kind == "key" && ref.Name == "my-key" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find key reference 'my-key', got refs: %+v", refs)
		}
	})

	t.Run("finds network references", func(t *testing.T) {
		spec := &v1.TestenvSpec{
			Networks: []v1.NetworkResource{
				{
					Name: "net2",
					Kind: "libvirt",
					Spec: v1.NetworkSpec{
						AttachTo: "{{ .Networks.net1.InterfaceName }}",
					},
				},
			},
		}

		refs := ExtractTemplateRefs(spec)

		found := false
		for _, ref := range refs {
			if ref.Kind == "network" && ref.Name == "net1" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find network reference 'net1', got refs: %+v", refs)
		}
	})

	t.Run("finds VM references", func(t *testing.T) {
		type CustomSpec struct {
			ConnectTo string
		}
		// Use a map since providerSpec is map[string]any
		spec := map[string]any{
			"vms": []any{
				map[string]any{
					"name": "vm2",
					"providerSpec": map[string]any{
						"connectTo": "{{ .VMs.vm1.IP }}",
					},
				},
			},
		}

		refs := ExtractTemplateRefs(spec)

		found := false
		for _, ref := range refs {
			if ref.Kind == "vm" && ref.Name == "vm1" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find VM reference 'vm1', got refs: %+v", refs)
		}
	})

	t.Run("handles nested structures", func(t *testing.T) {
		spec := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"level3": "{{ .Keys.deep-key.PublicKey }}",
				},
			},
		}

		refs := ExtractTemplateRefs(spec)

		found := false
		for _, ref := range refs {
			if ref.Kind == "key" && ref.Name == "deep-key" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find key reference 'deep-key', got refs: %+v", refs)
		}
	})

	t.Run("deduplicates refs", func(t *testing.T) {
		spec := map[string]any{
			"field1": "{{ .Keys.same-key.PublicKey }}",
			"field2": "{{ .Keys.same-key.PrivateKeyPath }}",
			"field3": "{{ .Keys.same-key.Fingerprint }}",
		}

		refs := ExtractTemplateRefs(spec)

		count := 0
		for _, ref := range refs {
			if ref.Kind == "key" && ref.Name == "same-key" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected exactly 1 reference to 'same-key', got %d", count)
		}
	})

	t.Run("handles slices in nested structures", func(t *testing.T) {
		spec := map[string]any{
			"items": []any{
				"{{ .Keys.key1.PublicKey }}",
				"{{ .Networks.net1.IP }}",
				map[string]any{
					"nested": "{{ .VMs.vm1.SSHCommand }}",
				},
			},
		}

		refs := ExtractTemplateRefs(spec)

		keyFound := false
		netFound := false
		vmFound := false
		for _, ref := range refs {
			if ref.Kind == "key" && ref.Name == "key1" {
				keyFound = true
			}
			if ref.Kind == "network" && ref.Name == "net1" {
				netFound = true
			}
			if ref.Kind == "vm" && ref.Name == "vm1" {
				vmFound = true
			}
		}
		if !keyFound || !netFound || !vmFound {
			t.Errorf("expected to find all three refs, got: %+v", refs)
		}
	})

	t.Run("ignores Env references", func(t *testing.T) {
		spec := map[string]any{
			"field": "{{ .Env.HOME }}",
		}

		refs := ExtractTemplateRefs(spec)

		if len(refs) != 0 {
			t.Errorf("expected no refs for Env, got: %+v", refs)
		}
	})

	t.Run("handles nil input", func(t *testing.T) {
		refs := ExtractTemplateRefs(nil)
		// nil is acceptable as an empty result
		if len(refs) != 0 {
			t.Errorf("expected empty/nil slice, got: %+v", refs)
		}
	})

	t.Run("handles TestenvSpec with providerSpec maps", func(t *testing.T) {
		spec := &v1.TestenvSpec{
			Providers: []v1.ProviderConfig{
				{Name: "p1", Engine: "test"},
			},
			Keys: []v1.KeyResource{
				{
					Name: "key1",
					Spec: v1.KeySpec{Type: "rsa"},
					ProviderSpec: map[string]any{
						"custom": "{{ .Networks.net1.IP }}",
					},
				},
			},
		}

		refs := ExtractTemplateRefs(spec)

		found := false
		for _, ref := range refs {
			if ref.Kind == "network" && ref.Name == "net1" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find network reference in providerSpec, got refs: %+v", refs)
		}
	})

	t.Run("finds image references", func(t *testing.T) {
		spec := map[string]any{
			"disk": map[string]any{
				"baseImage": "{{ .Images.ubuntu.Path }}",
			},
		}

		refs := ExtractTemplateRefs(spec)

		found := false
		for _, ref := range refs {
			if ref.Kind == "image" && ref.Name == "ubuntu" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find image reference 'ubuntu', got refs: %+v", refs)
		}
	})

	t.Run("DefaultBaseImage does not extract ResourceRef", func(t *testing.T) {
		// {{ .DefaultBaseImage }} is a plain string value, NOT a resource reference
		// It should NOT be extracted as a ResourceRef
		spec := map[string]any{
			"disk": map[string]any{
				"baseImage": "{{ .DefaultBaseImage }}",
			},
		}

		refs := ExtractTemplateRefs(spec)

		// DefaultBaseImage should NOT create any ResourceRef
		if len(refs) != 0 {
			t.Errorf("expected no refs for DefaultBaseImage, got: %+v", refs)
		}
	})

	t.Run("mixed image and key refs", func(t *testing.T) {
		spec := map[string]any{
			"disk":    "{{ .Images.img.Path }}",
			"sshKeys": "{{ .Keys.key.PublicKey }}",
		}

		refs := ExtractTemplateRefs(spec)

		imageFound := false
		keyFound := false
		for _, ref := range refs {
			if ref.Kind == "image" && ref.Name == "img" {
				imageFound = true
			}
			if ref.Kind == "key" && ref.Name == "key" {
				keyFound = true
			}
		}
		if !imageFound || !keyFound {
			t.Errorf("expected to find both image and key refs, got: %+v", refs)
		}
	})

	t.Run("image with hyphens in name", func(t *testing.T) {
		spec := map[string]any{
			"disk": "{{ .Images.ubuntu-24-04.Path }}",
		}

		refs := ExtractTemplateRefs(spec)

		found := false
		for _, ref := range refs {
			if ref.Kind == "image" && ref.Name == "ubuntu-24-04" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find image reference 'ubuntu-24-04', got refs: %+v", refs)
		}
	})
}

func TestNewTemplateContext(t *testing.T) {
	ctx := NewTemplateContext()

	if ctx == nil {
		t.Fatal("NewTemplateContext() returned nil")
	}
	if ctx.Keys == nil {
		t.Error("Keys map is nil")
	}
	if ctx.Networks == nil {
		t.Error("Networks map is nil")
	}
	if ctx.VMs == nil {
		t.Error("VMs map is nil")
	}
	if ctx.Images == nil {
		t.Error("Images map is nil")
	}
	if ctx.Env == nil {
		t.Error("Env map is nil")
	}
}
