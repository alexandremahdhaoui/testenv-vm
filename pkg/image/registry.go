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

// Package image provides image management functionality including
// well-known image registry, downloading, and caching.
package image

import "sort"

// WellKnownImage represents a well-known VM base image that can be referenced
// by a short name (e.g., "ubuntu:24.04") instead of a full URL.
type WellKnownImage struct {
	// Reference is the short name used to reference the image (e.g., "ubuntu:24.04").
	Reference string
	// URL is the download URL for the image.
	URL string
	// SHA256 is the expected checksum of the image.
	// NOTE: For well-known images, this is intentionally left EMPTY.
	// Cloud providers periodically update their images with security patches,
	// which changes the checksum. Embedding checksums would cause failures
	// when images are updated. For well-known images, checksum verification
	// is skipped unless the user explicitly provides a SHA256 override in their spec.
	// For custom URLs, SHA256 is REQUIRED and enforced by the validator.
	SHA256 string
	// Description is a human-readable description of the image.
	Description string
}

// defaultRegistry contains the built-in well-known images.
// This is the canonical source of truth for well-known image references.
var defaultRegistry = map[string]WellKnownImage{
	"ubuntu:24.04": {
		Reference:   "ubuntu:24.04",
		URL:         "https://cloud-images.ubuntu.com/releases/24.04/release/ubuntu-24.04-server-cloudimg-amd64.img",
		SHA256:      "", // Intentionally empty - cloud images update periodically
		Description: "Ubuntu 24.04 LTS (Noble Numbat) Cloud Image",
	},
	"ubuntu:22.04": {
		Reference:   "ubuntu:22.04",
		URL:         "https://cloud-images.ubuntu.com/releases/22.04/release/ubuntu-22.04-server-cloudimg-amd64.img",
		SHA256:      "", // Intentionally empty - cloud images update periodically
		Description: "Ubuntu 22.04 LTS (Jammy Jellyfish) Cloud Image",
	},
	"debian:12": {
		Reference:   "debian:12",
		URL:         "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-genericcloud-amd64.qcow2",
		SHA256:      "", // Intentionally empty - cloud images update periodically
		Description: "Debian 12 (Bookworm) Generic Cloud Image",
	},
}

// activeRegistry is the currently active registry used for lookups.
// This can be overridden for testing via SetRegistry().
var activeRegistry = defaultRegistry

// Resolve looks up a source string in the well-known image registry.
// If found, it returns the WellKnownImage and true.
// If not found, it returns nil and false.
func Resolve(source string) (*WellKnownImage, bool) {
	img, ok := activeRegistry[source]
	if !ok {
		return nil, false
	}
	return &img, true
}

// IsWellKnown returns true if the source string is a well-known image reference.
func IsWellKnown(source string) bool {
	_, ok := activeRegistry[source]
	return ok
}

// ListWellKnown returns a sorted list of all well-known image references.
func ListWellKnown() []string {
	refs := make([]string, 0, len(activeRegistry))
	for ref := range activeRegistry {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

// SetRegistry overrides the active registry with a custom registry.
// This is intended for testing purposes to inject mock images.
// Use ResetRegistry() to restore the default registry after tests.
func SetRegistry(r map[string]WellKnownImage) {
	activeRegistry = r
}

// ResetRegistry restores the active registry to the default built-in registry.
// This should be called in test cleanup (e.g., t.Cleanup) after using SetRegistry().
func ResetRegistry() {
	activeRegistry = defaultRegistry
}
