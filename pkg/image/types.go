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

import "time"

// Status constants for ImageState.Status field.
const (
	// StatusReady indicates the image has been downloaded and is ready for use.
	StatusReady = "ready"
	// StatusDownloading indicates the image is currently being downloaded.
	StatusDownloading = "downloading"
	// StatusCustomizing indicates the image is being customized via virt-customize.
	StatusCustomizing = "customizing"
	// StatusFailed indicates the image download or verification failed.
	StatusFailed = "failed"
)

// MetadataVersion is the current version of the cache metadata format.
// This enables future schema migrations if the metadata structure changes.
const MetadataVersion = "v1"

// ImageState represents the state of a cached image.
// It tracks download status, file location, and verification information.
type ImageState struct {
	// Name is the user-provided name of the image resource from the spec.
	Name string `json:"name"`
	// Source is the original source reference (well-known name or URL).
	Source string `json:"source"`
	// ResolvedURL is the actual download URL after resolving well-known references.
	ResolvedURL string `json:"resolvedUrl"`
	// LocalPath is the absolute path to the cached image file.
	LocalPath string `json:"localPath"`
	// SHA256 is the computed SHA256 checksum of the downloaded file.
	SHA256 string `json:"sha256"`
	// Size is the size of the downloaded file in bytes.
	Size int64 `json:"size"`
	// DownloadedAt is the timestamp when the image was downloaded.
	DownloadedAt time.Time `json:"downloadedAt"`
	// Status indicates the current state of the image.
	// Valid values are: "ready", "downloading", "customizing", "failed".
	Status string `json:"status"`
}

// CacheMetadata is the persistent metadata for the image cache.
// It is stored as metadata.json in the cache directory and tracks
// all cached images across multiple test environments.
type CacheMetadata struct {
	// Version is the metadata format version for schema compatibility.
	Version string `json:"version"`
	// Images maps cache keys (SHA256 of source) to their ImageState.
	// Using SHA256 of source as key ensures consistent cache lookups
	// regardless of image name variations across specs.
	Images map[string]*ImageState `json:"images"`
	// UpdatedAt is the timestamp of the last metadata update.
	UpdatedAt time.Time `json:"updatedAt"`
}
