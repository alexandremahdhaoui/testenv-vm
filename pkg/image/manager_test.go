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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

func TestNewCacheManager_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "new-cache-dir")

	// Verify directory does not exist initially
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Fatal("Cache directory should not exist before NewCacheManager")
	}

	m, err := NewCacheManager(cacheDir)
	if err != nil {
		t.Fatalf("NewCacheManager() unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("NewCacheManager() returned nil manager")
	}

	// Verify cache directory was created
	info, err := os.Stat(cacheDir)
	if err != nil {
		t.Fatalf("Cache directory should exist after NewCacheManager: %v", err)
	}
	if !info.IsDir() {
		t.Error("Cache path should be a directory")
	}

	// Verify locks directory was created
	locksDir := filepath.Join(cacheDir, ".locks")
	info, err = os.Stat(locksDir)
	if err != nil {
		t.Fatalf("Locks directory should exist after NewCacheManager: %v", err)
	}
	if !info.IsDir() {
		t.Error("Locks path should be a directory")
	}
}

func TestNewCacheManager_LoadsExistingMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	// Create cache directory structure
	if err := os.MkdirAll(filepath.Join(cacheDir, ".locks"), 0o755); err != nil {
		t.Fatalf("Failed to create cache directory: %v", err)
	}

	// Pre-populate metadata.json with existing image state
	existingMetadata := &CacheMetadata{
		Version: MetadataVersion,
		Images: map[string]*ImageState{
			"testkey123": {
				Name:         "test-image",
				Source:       "test:source",
				ResolvedURL:  "https://example.com/image.qcow2",
				LocalPath:    "/path/to/image.qcow2",
				SHA256:       "abc123",
				Size:         1024,
				DownloadedAt: time.Now().Add(-1 * time.Hour),
				Status:       StatusReady,
			},
		},
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}

	metadataPath := filepath.Join(cacheDir, "metadata.json")
	data, err := json.MarshalIndent(existingMetadata, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal metadata: %v", err)
	}
	if err := os.WriteFile(metadataPath, data, 0o644); err != nil {
		t.Fatalf("Failed to write metadata file: %v", err)
	}

	// Create CacheManager and verify it loads existing metadata
	m, err := NewCacheManager(cacheDir)
	if err != nil {
		t.Fatalf("NewCacheManager() unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("NewCacheManager() returned nil manager")
	}

	// Verify metadata was loaded
	if m.metadata == nil {
		t.Fatal("Metadata should be loaded")
	}
	if len(m.metadata.Images) != 1 {
		t.Errorf("Expected 1 image in metadata, got %d", len(m.metadata.Images))
	}

	imgState, found := m.metadata.Images["testkey123"]
	if !found {
		t.Error("Expected to find 'testkey123' in loaded metadata")
	} else {
		if imgState.Name != "test-image" {
			t.Errorf("Image name = %q, want %q", imgState.Name, "test-image")
		}
		if imgState.Status != StatusReady {
			t.Errorf("Image status = %q, want %q", imgState.Status, StatusReady)
		}
	}
}

func TestEnsureImage_CacheMiss(t *testing.T) {
	t.Cleanup(ResetRegistry)

	expectedContent := "test image content for cache miss"
	h := sha256.Sum256([]byte(expectedContent))
	expectedChecksum := hex.EncodeToString(h[:])

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedContent))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	// Create downloader with test HTTP client
	downloader := NewDownloader(
		WithHTTPClient(server.Client()),
		WithMaxRetries(1),
		WithBaseBackoff(1*time.Millisecond),
	)

	m, err := NewCacheManager(cacheDir, WithDownloader(downloader))
	if err != nil {
		t.Fatalf("NewCacheManager() unexpected error: %v", err)
	}

	spec := v1.ImageSpec{
		Source: server.URL + "/image.qcow2",
		Sha256: expectedChecksum,
	}

	// First call - should download
	state, err := m.EnsureImage(context.Background(), "test-image", spec)
	if err != nil {
		t.Fatalf("EnsureImage() first call unexpected error: %v", err)
	}
	if state == nil {
		t.Fatal("EnsureImage() returned nil state")
	}
	if state.Status != StatusReady {
		t.Errorf("EnsureImage() status = %q, want %q", state.Status, StatusReady)
	}
	if state.Name != "test-image" {
		t.Errorf("EnsureImage() name = %q, want %q", state.Name, "test-image")
	}

	// Verify file was downloaded
	content, err := os.ReadFile(state.LocalPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}
	if string(content) != expectedContent {
		t.Errorf("Downloaded content mismatch: got %q, want %q", string(content), expectedContent)
	}

	// Second call - should use cache (file already exists)
	state2, err := m.EnsureImage(context.Background(), "test-image", spec)
	if err != nil {
		t.Fatalf("EnsureImage() second call unexpected error: %v", err)
	}
	if state2 == nil {
		t.Fatal("EnsureImage() second call returned nil state")
	}
	if state2.LocalPath != state.LocalPath {
		t.Errorf("Second call should return same path: got %q, want %q", state2.LocalPath, state.LocalPath)
	}
}

func TestEnsureImage_CacheHit(t *testing.T) {
	t.Cleanup(ResetRegistry)

	imageContent := "pre-cached image content"
	h := sha256.Sum256([]byte(imageContent))
	checksum := hex.EncodeToString(h[:])

	// This server should never be called if cache hit works
	serverCalled := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(imageContent))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	source := server.URL + "/cached.qcow2"

	// Compute cache key the same way as the manager
	cacheKeyHash := sha256.Sum256([]byte(source))
	cacheKey := hex.EncodeToString(cacheKeyHash[:])

	// Pre-create the cache directory and image file
	imageName := "cached-image"
	imageDir := filepath.Join(cacheDir, imageName)
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		t.Fatalf("Failed to create image directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cacheDir, ".locks"), 0o755); err != nil {
		t.Fatalf("Failed to create locks directory: %v", err)
	}

	localPath := filepath.Join(imageDir, "cached.qcow2")
	if err := os.WriteFile(localPath, []byte(imageContent), 0o644); err != nil {
		t.Fatalf("Failed to write cached image: %v", err)
	}

	// Pre-populate metadata.json
	existingMetadata := &CacheMetadata{
		Version: MetadataVersion,
		Images: map[string]*ImageState{
			cacheKey: {
				Name:         imageName,
				Source:       source,
				ResolvedURL:  source,
				LocalPath:    localPath,
				SHA256:       checksum,
				Size:         int64(len(imageContent)),
				DownloadedAt: time.Now().Add(-1 * time.Hour),
				Status:       StatusReady,
			},
		},
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}

	metadataPath := filepath.Join(cacheDir, "metadata.json")
	data, err := json.MarshalIndent(existingMetadata, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal metadata: %v", err)
	}
	if err := os.WriteFile(metadataPath, data, 0o644); err != nil {
		t.Fatalf("Failed to write metadata file: %v", err)
	}

	// Create downloader with test HTTP client
	downloader := NewDownloader(
		WithHTTPClient(server.Client()),
		WithMaxRetries(1),
		WithBaseBackoff(1*time.Millisecond),
	)

	m, err := NewCacheManager(cacheDir, WithDownloader(downloader))
	if err != nil {
		t.Fatalf("NewCacheManager() unexpected error: %v", err)
	}

	spec := v1.ImageSpec{
		Source: source,
		Sha256: checksum,
	}

	// This call should use the cache, not the server
	state, err := m.EnsureImage(context.Background(), imageName, spec)
	if err != nil {
		t.Fatalf("EnsureImage() unexpected error: %v", err)
	}
	if state == nil {
		t.Fatal("EnsureImage() returned nil state")
	}
	if state.LocalPath != localPath {
		t.Errorf("EnsureImage() localPath = %q, want %q", state.LocalPath, localPath)
	}
	if serverCalled {
		t.Error("Server should not have been called - image was cached")
	}
}

func TestEnsureImage_WellKnown(t *testing.T) {
	t.Cleanup(ResetRegistry)

	imageContent := "ubuntu cloud image content"
	h := sha256.Sum256([]byte(imageContent))
	checksum := hex.EncodeToString(h[:])

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(imageContent))
	}))
	defer server.Close()

	// Override registry with a test well-known image that points to our test server
	testRegistry := map[string]WellKnownImage{
		"ubuntu:24.04": {
			Reference:   "ubuntu:24.04",
			URL:         server.URL + "/ubuntu-24.04.img",
			SHA256:      "", // Well-known images typically don't have embedded checksums
			Description: "Test Ubuntu 24.04",
		},
	}
	SetRegistry(testRegistry)

	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	// Create downloader with test HTTP client
	downloader := NewDownloader(
		WithHTTPClient(server.Client()),
		WithMaxRetries(1),
		WithBaseBackoff(1*time.Millisecond),
	)

	m, err := NewCacheManager(cacheDir, WithDownloader(downloader))
	if err != nil {
		t.Fatalf("NewCacheManager() unexpected error: %v", err)
	}

	spec := v1.ImageSpec{
		Source: "ubuntu:24.04", // Well-known reference
		Sha256: checksum,       // User can optionally provide SHA256
	}

	state, err := m.EnsureImage(context.Background(), "my-ubuntu", spec)
	if err != nil {
		t.Fatalf("EnsureImage() unexpected error: %v", err)
	}
	if state == nil {
		t.Fatal("EnsureImage() returned nil state")
	}
	if state.Status != StatusReady {
		t.Errorf("EnsureImage() status = %q, want %q", state.Status, StatusReady)
	}
	if state.Source != "ubuntu:24.04" {
		t.Errorf("EnsureImage() source = %q, want %q", state.Source, "ubuntu:24.04")
	}
	// ResolvedURL should be the actual URL from the registry
	if !strings.HasPrefix(state.ResolvedURL, server.URL) {
		t.Errorf("EnsureImage() resolvedURL = %q, want prefix %q", state.ResolvedURL, server.URL)
	}
}

func TestEnsureImage_DirectURL(t *testing.T) {
	t.Cleanup(ResetRegistry)

	imageContent := "direct url image content"
	h := sha256.Sum256([]byte(imageContent))
	checksum := hex.EncodeToString(h[:])

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(imageContent))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	// Create downloader with test HTTP client
	downloader := NewDownloader(
		WithHTTPClient(server.Client()),
		WithMaxRetries(1),
		WithBaseBackoff(1*time.Millisecond),
	)

	m, err := NewCacheManager(cacheDir, WithDownloader(downloader))
	if err != nil {
		t.Fatalf("NewCacheManager() unexpected error: %v", err)
	}

	directURL := server.URL + "/custom-image.qcow2"
	spec := v1.ImageSpec{
		Source: directURL,
		Sha256: checksum,
	}

	state, err := m.EnsureImage(context.Background(), "custom-image", spec)
	if err != nil {
		t.Fatalf("EnsureImage() unexpected error: %v", err)
	}
	if state == nil {
		t.Fatal("EnsureImage() returned nil state")
	}
	if state.Status != StatusReady {
		t.Errorf("EnsureImage() status = %q, want %q", state.Status, StatusReady)
	}
	if state.Source != directURL {
		t.Errorf("EnsureImage() source = %q, want %q", state.Source, directURL)
	}
	if state.ResolvedURL != directURL {
		t.Errorf("EnsureImage() resolvedURL = %q, want %q", state.ResolvedURL, directURL)
	}

	// Verify file content
	content, err := os.ReadFile(state.LocalPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}
	if string(content) != imageContent {
		t.Errorf("Downloaded content mismatch: got %q, want %q", string(content), imageContent)
	}
}

func TestEnsureImage_HTTPRejected(t *testing.T) {
	t.Cleanup(ResetRegistry)

	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	m, err := NewCacheManager(cacheDir)
	if err != nil {
		t.Fatalf("NewCacheManager() unexpected error: %v", err)
	}

	spec := v1.ImageSpec{
		Source: "http://insecure.example.com/image.qcow2", // HTTP, not HTTPS
		Sha256: "abc123",
	}

	_, err = m.EnsureImage(context.Background(), "insecure-image", spec)
	if err == nil {
		t.Fatal("EnsureImage() expected error for HTTP URL, got nil")
	}
	if !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("Error should mention HTTPS requirement, got: %v", err)
	}
}

func TestGetImagePath_Found(t *testing.T) {
	t.Cleanup(ResetRegistry)

	imageContent := "image for path lookup"
	h := sha256.Sum256([]byte(imageContent))
	checksum := hex.EncodeToString(h[:])

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(imageContent))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	downloader := NewDownloader(
		WithHTTPClient(server.Client()),
		WithMaxRetries(1),
		WithBaseBackoff(1*time.Millisecond),
	)

	m, err := NewCacheManager(cacheDir, WithDownloader(downloader))
	if err != nil {
		t.Fatalf("NewCacheManager() unexpected error: %v", err)
	}

	// First ensure the image
	spec := v1.ImageSpec{
		Source: server.URL + "/testimg.qcow2",
		Sha256: checksum,
	}

	state, err := m.EnsureImage(context.Background(), "lookup-test", spec)
	if err != nil {
		t.Fatalf("EnsureImage() unexpected error: %v", err)
	}

	// Now test GetImagePath
	path, found := m.GetImagePath("lookup-test")
	if !found {
		t.Error("GetImagePath() should find the ensured image")
	}
	if path != state.LocalPath {
		t.Errorf("GetImagePath() path = %q, want %q", path, state.LocalPath)
	}

	// Verify the file exists at the returned path
	if _, err := os.Stat(path); err != nil {
		t.Errorf("File should exist at returned path: %v", err)
	}
}

func TestGetImagePath_NotFound(t *testing.T) {
	t.Cleanup(ResetRegistry)

	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	m, err := NewCacheManager(cacheDir)
	if err != nil {
		t.Fatalf("NewCacheManager() unexpected error: %v", err)
	}

	// Try to get path for non-existent image
	path, found := m.GetImagePath("nonexistent-image")
	if found {
		t.Error("GetImagePath() should not find non-existent image")
	}
	if path != "" {
		t.Errorf("GetImagePath() path should be empty for non-existent image, got %q", path)
	}
}

func TestGetImagePath_NotReadyImage(t *testing.T) {
	t.Cleanup(ResetRegistry)

	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	// Create cache directory structure
	if err := os.MkdirAll(filepath.Join(cacheDir, ".locks"), 0o755); err != nil {
		t.Fatalf("Failed to create cache directory: %v", err)
	}

	// Pre-populate metadata with a failed image
	existingMetadata := &CacheMetadata{
		Version: MetadataVersion,
		Images: map[string]*ImageState{
			"failedkey": {
				Name:         "failed-image",
				Source:       "https://example.com/failed.qcow2",
				ResolvedURL:  "https://example.com/failed.qcow2",
				LocalPath:    "/path/to/failed.qcow2",
				Status:       StatusFailed, // Not ready
			},
		},
		UpdatedAt: time.Now(),
	}

	metadataPath := filepath.Join(cacheDir, "metadata.json")
	data, err := json.MarshalIndent(existingMetadata, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal metadata: %v", err)
	}
	if err := os.WriteFile(metadataPath, data, 0o644); err != nil {
		t.Fatalf("Failed to write metadata file: %v", err)
	}

	m, err := NewCacheManager(cacheDir)
	if err != nil {
		t.Fatalf("NewCacheManager() unexpected error: %v", err)
	}

	// GetImagePath should return false for failed images
	path, found := m.GetImagePath("failed-image")
	if found {
		t.Error("GetImagePath() should not find failed image")
	}
	if path != "" {
		t.Errorf("GetImagePath() path should be empty for failed image, got %q", path)
	}
}

func TestEnsureImage_ChecksumMismatch(t *testing.T) {
	t.Cleanup(ResetRegistry)

	imageContent := "content with wrong checksum"

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(imageContent))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	downloader := NewDownloader(
		WithHTTPClient(server.Client()),
		WithMaxRetries(1),
		WithBaseBackoff(1*time.Millisecond),
	)

	m, err := NewCacheManager(cacheDir, WithDownloader(downloader))
	if err != nil {
		t.Fatalf("NewCacheManager() unexpected error: %v", err)
	}

	spec := v1.ImageSpec{
		Source: server.URL + "/image.qcow2",
		Sha256: "0000000000000000000000000000000000000000000000000000000000000000", // Wrong checksum
	}

	_, err = m.EnsureImage(context.Background(), "bad-checksum", spec)
	if err == nil {
		t.Fatal("EnsureImage() expected error for checksum mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("Error should mention checksum, got: %v", err)
	}
}

func TestCacheManager_MetadataPersistence(t *testing.T) {
	t.Cleanup(ResetRegistry)

	imageContent := "persistence test content"
	h := sha256.Sum256([]byte(imageContent))
	checksum := hex.EncodeToString(h[:])

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(imageContent))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	downloader := NewDownloader(
		WithHTTPClient(server.Client()),
		WithMaxRetries(1),
		WithBaseBackoff(1*time.Millisecond),
	)

	// First manager instance
	m1, err := NewCacheManager(cacheDir, WithDownloader(downloader))
	if err != nil {
		t.Fatalf("NewCacheManager() unexpected error: %v", err)
	}

	spec := v1.ImageSpec{
		Source: server.URL + "/persist.qcow2",
		Sha256: checksum,
	}

	state1, err := m1.EnsureImage(context.Background(), "persist-test", spec)
	if err != nil {
		t.Fatalf("EnsureImage() unexpected error: %v", err)
	}

	// Create a second manager instance - should load persisted metadata
	m2, err := NewCacheManager(cacheDir, WithDownloader(downloader))
	if err != nil {
		t.Fatalf("NewCacheManager() second instance unexpected error: %v", err)
	}

	// GetImagePath should find the image from first instance
	path, found := m2.GetImagePath("persist-test")
	if !found {
		t.Error("GetImagePath() should find image persisted by first manager")
	}
	if path != state1.LocalPath {
		t.Errorf("GetImagePath() path = %q, want %q", path, state1.LocalPath)
	}
}
