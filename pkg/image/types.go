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

const (
	StatusReady       = "ready"
	StatusDownloading = "downloading"
	StatusCustomizing = "customizing"
	StatusFailed      = "failed"
)

const MetadataVersion = "v1"

type ImageState struct {
	Name           string    `json:"name"`
	Source         string    `json:"source"`
	ResolvedURL    string    `json:"resolvedUrl"`
	LocalPath      string    `json:"localPath"`
	SHA256         string    `json:"sha256"`
	CompressedPath string    `json:"compressedPath,omitempty"`
	Size           int64     `json:"size"`
	DownloadedAt   time.Time `json:"downloadedAt"`
	Status         string    `json:"status"`
}

type CacheMetadata struct {
	Version   string                 `json:"version"`
	Images    map[string]*ImageState `json:"images"`
	UpdatedAt time.Time              `json:"updatedAt"`
}
