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

package image

import (
	"sort"
	"testing"
)

func TestResolve_WellKnownImage(t *testing.T) {
	tests := []struct {
		name           string
		source         string
		wantFound      bool
		wantReference  string
		wantURLPrefix  string
		wantSHA256     string
	}{
		{
			name:          "ubuntu 24.04 resolves correctly",
			source:        "ubuntu:24.04",
			wantFound:     true,
			wantReference: "ubuntu:24.04",
			wantURLPrefix: "https://cloud-images.ubuntu.com/releases/24.04/",
			wantSHA256:    "", // Intentionally empty for well-known images
		},
		{
			name:          "ubuntu 22.04 resolves correctly",
			source:        "ubuntu:22.04",
			wantFound:     true,
			wantReference: "ubuntu:22.04",
			wantURLPrefix: "https://cloud-images.ubuntu.com/releases/22.04/",
			wantSHA256:    "", // Intentionally empty for well-known images
		},
		{
			name:          "debian 12 resolves correctly",
			source:        "debian:12",
			wantFound:     true,
			wantReference: "debian:12",
			wantURLPrefix: "https://cloud.debian.org/images/cloud/bookworm/",
			wantSHA256:    "", // Intentionally empty for well-known images
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(ResetRegistry)

			img, found := Resolve(tt.source)
			if found != tt.wantFound {
				t.Errorf("Resolve(%q) found = %v, want %v", tt.source, found, tt.wantFound)
				return
			}
			if !found {
				return
			}
			if img == nil {
				t.Errorf("Resolve(%q) returned nil image when found=true", tt.source)
				return
			}
			if img.Reference != tt.wantReference {
				t.Errorf("Resolve(%q).Reference = %q, want %q", tt.source, img.Reference, tt.wantReference)
			}
			if len(tt.wantURLPrefix) > 0 && len(img.URL) < len(tt.wantURLPrefix) {
				t.Errorf("Resolve(%q).URL = %q, want prefix %q", tt.source, img.URL, tt.wantURLPrefix)
			} else if len(tt.wantURLPrefix) > 0 && img.URL[:len(tt.wantURLPrefix)] != tt.wantURLPrefix {
				t.Errorf("Resolve(%q).URL = %q, want prefix %q", tt.source, img.URL, tt.wantURLPrefix)
			}
			if img.SHA256 != tt.wantSHA256 {
				t.Errorf("Resolve(%q).SHA256 = %q, want %q", tt.source, img.SHA256, tt.wantSHA256)
			}
		})
	}
}

func TestResolve_NotFound(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "unknown image reference",
			source: "unknown:image",
		},
		{
			name:   "nonexistent distribution",
			source: "centos:8",
		},
		{
			name:   "empty string",
			source: "",
		},
		{
			name:   "just a version number",
			source: "24.04",
		},
		{
			name:   "partial match",
			source: "ubuntu",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(ResetRegistry)

			img, found := Resolve(tt.source)
			if found {
				t.Errorf("Resolve(%q) found = true, want false", tt.source)
			}
			if img != nil {
				t.Errorf("Resolve(%q) img = %v, want nil", tt.source, img)
			}
		})
	}
}

func TestIsWellKnown_True(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "ubuntu 24.04", source: "ubuntu:24.04"},
		{name: "ubuntu 22.04", source: "ubuntu:22.04"},
		{name: "debian 12", source: "debian:12"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(ResetRegistry)

			if !IsWellKnown(tt.source) {
				t.Errorf("IsWellKnown(%q) = false, want true", tt.source)
			}
		})
	}
}

func TestIsWellKnown_False(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "https URL", source: "https://example.com/image.qcow2"},
		{name: "http URL", source: "http://example.com/image.qcow2"},
		{name: "unknown reference", source: "fedora:39"},
		{name: "empty string", source: ""},
		{name: "local path", source: "/var/lib/libvirt/images/base.qcow2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(ResetRegistry)

			if IsWellKnown(tt.source) {
				t.Errorf("IsWellKnown(%q) = true, want false", tt.source)
			}
		})
	}
}

func TestListWellKnown(t *testing.T) {
	t.Cleanup(ResetRegistry)

	refs := ListWellKnown()

	// Verify we get the expected count
	if len(refs) != 3 {
		t.Errorf("ListWellKnown() returned %d refs, want 3", len(refs))
	}

	// Verify the list is sorted
	if !sort.StringsAreSorted(refs) {
		t.Errorf("ListWellKnown() returned unsorted list: %v", refs)
	}

	// Verify all expected images are present
	expected := []string{"debian:12", "ubuntu:22.04", "ubuntu:24.04"}
	for _, exp := range expected {
		found := false
		for _, ref := range refs {
			if ref == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ListWellKnown() missing expected image %q, got: %v", exp, refs)
		}
	}
}

func TestDefaultRegistry_HasRequiredFields(t *testing.T) {
	t.Cleanup(ResetRegistry)

	refs := ListWellKnown()
	for _, ref := range refs {
		img, found := Resolve(ref)
		if !found {
			t.Errorf("Resolve(%q) should find image from ListWellKnown", ref)
			continue
		}

		// Verify Reference is set
		if img.Reference == "" {
			t.Errorf("Image %q has empty Reference", ref)
		}

		// Verify URL is non-empty
		if img.URL == "" {
			t.Errorf("Image %q has empty URL", ref)
		}

		// Verify URL is HTTPS
		if len(img.URL) < 8 || img.URL[:8] != "https://" {
			t.Errorf("Image %q URL should start with https://, got: %q", ref, img.URL)
		}

		// Verify Description is non-empty
		if img.Description == "" {
			t.Errorf("Image %q has empty Description", ref)
		}

		// Note: SHA256 is intentionally empty for well-known images
		// (cloud providers update images periodically)
	}
}

func TestSetRegistry_Override(t *testing.T) {
	t.Cleanup(ResetRegistry)

	// Create a custom registry
	customRegistry := map[string]WellKnownImage{
		"test:image": {
			Reference:   "test:image",
			URL:         "https://test.example.com/image.qcow2",
			SHA256:      "abc123",
			Description: "Test image for unit tests",
		},
	}

	// Override the registry
	SetRegistry(customRegistry)

	// Verify the custom image is now resolvable
	img, found := Resolve("test:image")
	if !found {
		t.Error("Resolve('test:image') should find custom image after SetRegistry")
	}
	if img == nil {
		t.Fatal("Resolve('test:image') returned nil when found=true")
	}
	if img.URL != "https://test.example.com/image.qcow2" {
		t.Errorf("img.URL = %q, want %q", img.URL, "https://test.example.com/image.qcow2")
	}
	if img.SHA256 != "abc123" {
		t.Errorf("img.SHA256 = %q, want %q", img.SHA256, "abc123")
	}

	// Verify original images are no longer resolvable
	_, found = Resolve("ubuntu:24.04")
	if found {
		t.Error("Resolve('ubuntu:24.04') should not find default image after SetRegistry override")
	}

	// Verify ListWellKnown returns only the custom image
	refs := ListWellKnown()
	if len(refs) != 1 {
		t.Errorf("ListWellKnown() returned %d refs, want 1", len(refs))
	}
	if len(refs) > 0 && refs[0] != "test:image" {
		t.Errorf("ListWellKnown()[0] = %q, want %q", refs[0], "test:image")
	}
}

func TestResetRegistry(t *testing.T) {
	// Override with custom registry
	customRegistry := map[string]WellKnownImage{
		"custom:only": {
			Reference:   "custom:only",
			URL:         "https://custom.example.com/only.qcow2",
			SHA256:      "",
			Description: "Custom only image",
		},
	}
	SetRegistry(customRegistry)

	// Verify custom registry is active
	_, found := Resolve("custom:only")
	if !found {
		t.Fatal("Resolve('custom:only') should find custom image before reset")
	}

	// Reset the registry
	ResetRegistry()

	// Verify custom image is no longer resolvable
	_, found = Resolve("custom:only")
	if found {
		t.Error("Resolve('custom:only') should not find custom image after ResetRegistry")
	}

	// Verify default images are back
	_, found = Resolve("ubuntu:24.04")
	if !found {
		t.Error("Resolve('ubuntu:24.04') should find default image after ResetRegistry")
	}
	_, found = Resolve("ubuntu:22.04")
	if !found {
		t.Error("Resolve('ubuntu:22.04') should find default image after ResetRegistry")
	}
	_, found = Resolve("debian:12")
	if !found {
		t.Error("Resolve('debian:12') should find default image after ResetRegistry")
	}

	// Verify count is back to 3
	refs := ListWellKnown()
	if len(refs) != 3 {
		t.Errorf("ListWellKnown() returned %d refs after reset, want 3", len(refs))
	}
}

func TestSetRegistry_EmptyRegistry(t *testing.T) {
	t.Cleanup(ResetRegistry)

	// Set an empty registry
	SetRegistry(map[string]WellKnownImage{})

	// Verify no images are resolvable
	_, found := Resolve("ubuntu:24.04")
	if found {
		t.Error("Resolve('ubuntu:24.04') should not find image in empty registry")
	}

	// Verify ListWellKnown returns empty slice
	refs := ListWellKnown()
	if len(refs) != 0 {
		t.Errorf("ListWellKnown() returned %d refs for empty registry, want 0", len(refs))
	}
}

func TestSetRegistry_MultipleImages(t *testing.T) {
	t.Cleanup(ResetRegistry)

	// Create a registry with multiple custom images
	customRegistry := map[string]WellKnownImage{
		"test:v1": {
			Reference:   "test:v1",
			URL:         "https://test.example.com/v1.qcow2",
			SHA256:      "sha1",
			Description: "Test v1",
		},
		"test:v2": {
			Reference:   "test:v2",
			URL:         "https://test.example.com/v2.qcow2",
			SHA256:      "sha2",
			Description: "Test v2",
		},
		"other:latest": {
			Reference:   "other:latest",
			URL:         "https://other.example.com/latest.qcow2",
			SHA256:      "sha3",
			Description: "Other latest",
		},
	}

	SetRegistry(customRegistry)

	// Verify all custom images are resolvable
	for ref, expected := range customRegistry {
		img, found := Resolve(ref)
		if !found {
			t.Errorf("Resolve(%q) should find custom image", ref)
			continue
		}
		if img.URL != expected.URL {
			t.Errorf("Resolve(%q).URL = %q, want %q", ref, img.URL, expected.URL)
		}
		if img.SHA256 != expected.SHA256 {
			t.Errorf("Resolve(%q).SHA256 = %q, want %q", ref, img.SHA256, expected.SHA256)
		}
	}

	// Verify ListWellKnown returns all custom images
	refs := ListWellKnown()
	if len(refs) != 3 {
		t.Errorf("ListWellKnown() returned %d refs, want 3", len(refs))
	}
}
