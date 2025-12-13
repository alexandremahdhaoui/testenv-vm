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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
	"golang.org/x/sys/unix"
)

// CacheManager orchestrates image downloading and caching.
// It handles concurrent access using file-based locking (flock) for cross-process
// safety and a mutex for in-memory metadata access within a single process.
type CacheManager struct {
	// cacheDir is the root directory for the image cache.
	cacheDir string
	// downloader handles HTTP downloads.
	downloader *Downloader
	// metadata is the in-memory cache metadata.
	metadata *CacheMetadata
	// mu protects metadata access within a single process.
	mu sync.Mutex
}

// CacheManagerOption is a functional option for configuring a CacheManager.
type CacheManagerOption func(*CacheManager)

// WithDownloader sets a custom Downloader for the CacheManager.
// This is primarily used for testing with custom HTTP clients.
func WithDownloader(d *Downloader) CacheManagerOption {
	return func(m *CacheManager) {
		m.downloader = d
	}
}

// NewCacheManager creates a new CacheManager with the given cache directory.
// It creates the cache directory and locks subdirectory if they don't exist,
// and loads or initializes the metadata.json file.
func NewCacheManager(cacheDir string, opts ...CacheManagerOption) (*CacheManager, error) {
	m := &CacheManager{
		cacheDir:   cacheDir,
		downloader: NewDownloader(),
	}

	// Apply options
	for _, opt := range opts {
		opt(m)
	}

	// Create cache directory if not exists
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Create locks directory
	locksDir := filepath.Join(cacheDir, ".locks")
	if err := os.MkdirAll(locksDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create locks directory: %w", err)
	}

	// Load or initialize metadata
	if err := m.loadMetadata(); err != nil {
		return nil, fmt.Errorf("failed to load metadata: %w", err)
	}

	return m, nil
}

// EnsureImage ensures an image is available in the cache.
// It resolves the source (well-known or direct URL), checks the cache,
// and downloads the image if necessary. It uses file-based locking for
// cross-process safety.
//
// Returns the ImageState with LocalPath set to the cached image file.
func (m *CacheManager) EnsureImage(ctx context.Context, name string, spec v1.ImageSpec) (*ImageState, error) {
	// Resolve source
	source := spec.Source
	var resolvedURL string
	var expectedSHA256 string

	wellKnown, isWellKnown := Resolve(source)
	if isWellKnown {
		resolvedURL = wellKnown.URL
		// Use user-provided SHA256 if set, otherwise use registry's (which may be empty)
		if spec.SHA256 != "" {
			expectedSHA256 = spec.SHA256
		} else {
			expectedSHA256 = wellKnown.SHA256
		}
	} else {
		// Direct URL - validate HTTPS
		if !strings.HasPrefix(source, "https://") {
			return nil, fmt.Errorf("direct URL must use HTTPS: %s", source)
		}
		resolvedURL = source
		expectedSHA256 = spec.SHA256
	}

	// Compute cache key from source
	key := m.cacheKey(source)

	// Acquire file-based lock for cross-process safety
	lockFile, err := m.acquireFileLock(key)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() {
		_ = m.releaseFileLock(lockFile)
	}()

	// Check cache (re-check after acquiring lock)
	m.mu.Lock()
	existing, found := m.metadata.Images[key]
	m.mu.Unlock()

	if found && existing.Status == StatusReady {
		// Verify file still exists
		if _, err := os.Stat(existing.LocalPath); err == nil {
			// Verify checksum if we have one
			if expectedSHA256 != "" {
				if err := m.downloader.VerifyChecksum(existing.LocalPath, expectedSHA256); err == nil {
					// Cache hit - return existing state
					return existing, nil
				}
				// Checksum mismatch - need to re-download
			} else {
				// No checksum to verify - trust existing file
				return existing, nil
			}
		}
		// File missing or corrupted - fall through to download
	}

	// Cache miss or corrupted - need to download
	imageDir := m.imageDirName(name)
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create image directory: %w", err)
	}

	// Determine filename from URL
	filename := filepath.Base(resolvedURL)
	if filename == "" || filename == "." || filename == "/" {
		filename = "image"
	}
	localPath := filepath.Join(imageDir, filename)

	// Update metadata to "downloading" status
	m.mu.Lock()
	m.metadata.Images[key] = &ImageState{
		Name:        name,
		Source:      source,
		ResolvedURL: resolvedURL,
		LocalPath:   localPath,
		Status:      StatusDownloading,
	}
	if err := m.saveMetadata(); err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("failed to save metadata: %w", err)
	}
	m.mu.Unlock()

	// Download the image
	if err := m.downloader.Download(ctx, resolvedURL, localPath); err != nil {
		// Update metadata to failed status
		m.mu.Lock()
		m.metadata.Images[key].Status = StatusFailed
		_ = m.saveMetadata()
		m.mu.Unlock()
		return nil, fmt.Errorf("failed to download image: %w", err)
	}

	// Verify checksum if provided
	if expectedSHA256 != "" {
		if err := m.downloader.VerifyChecksum(localPath, expectedSHA256); err != nil {
			// Remove corrupted file
			_ = os.Remove(localPath)
			// Update metadata to failed status
			m.mu.Lock()
			m.metadata.Images[key].Status = StatusFailed
			_ = m.saveMetadata()
			m.mu.Unlock()
			return nil, fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	// Get file info
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat downloaded file: %w", err)
	}

	// Compute actual checksum
	actualSHA256, err := m.computeChecksum(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to compute checksum: %w", err)
	}

	// Update metadata with success
	m.mu.Lock()
	state := &ImageState{
		Name:         name,
		Source:       source,
		ResolvedURL:  resolvedURL,
		LocalPath:    localPath,
		SHA256:       actualSHA256,
		Size:         fileInfo.Size(),
		DownloadedAt: time.Now(),
		Status:       StatusReady,
	}
	m.metadata.Images[key] = state
	m.metadata.UpdatedAt = time.Now()
	if err := m.saveMetadata(); err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("failed to save metadata: %w", err)
	}
	m.mu.Unlock()

	return state, nil
}

// GetImagePath returns the local path for an already-ensured image.
// It performs a quick lookup in the in-memory metadata without locking.
// Returns the path and true if found, or empty string and false if not found.
func (m *CacheManager) GetImagePath(name string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Search through all images for one with matching name
	for _, img := range m.metadata.Images {
		if img.Name == name && img.Status == StatusReady {
			return img.LocalPath, true
		}
	}
	return "", false
}

// loadMetadata loads metadata from metadata.json or initializes it if not found.
func (m *CacheManager) loadMetadata() error {
	metadataPath := filepath.Join(m.cacheDir, "metadata.json")

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Initialize new metadata
			m.metadata = &CacheMetadata{
				Version:   MetadataVersion,
				Images:    make(map[string]*ImageState),
				UpdatedAt: time.Now(),
			}
			return nil
		}
		return fmt.Errorf("failed to read metadata file: %w", err)
	}

	var metadata CacheMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("failed to parse metadata file: %w", err)
	}

	// Ensure Images map is initialized
	if metadata.Images == nil {
		metadata.Images = make(map[string]*ImageState)
	}

	m.metadata = &metadata
	return nil
}

// saveMetadata writes metadata to metadata.json.
// Caller must hold m.mu lock.
func (m *CacheManager) saveMetadata() error {
	metadataPath := filepath.Join(m.cacheDir, "metadata.json")

	data, err := json.MarshalIndent(m.metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Write atomically via temp file
	tmpPath := metadataPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	if err := os.Rename(tmpPath, metadataPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename metadata file: %w", err)
	}

	return nil
}

// acquireFileLock acquires an exclusive file lock for the given cache key.
// Returns the locked file handle which must be released via releaseFileLock.
func (m *CacheManager) acquireFileLock(key string) (*os.File, error) {
	lockPath := filepath.Join(m.cacheDir, ".locks", key+".lock")

	// Create or open lock file
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	// Acquire exclusive lock (blocking)
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("failed to acquire flock: %w", err)
	}

	return f, nil
}

// releaseFileLock releases a file lock and closes the file.
func (m *CacheManager) releaseFileLock(f *os.File) error {
	if f == nil {
		return nil
	}

	// Release lock
	if err := unix.Flock(int(f.Fd()), unix.LOCK_UN); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to release flock: %w", err)
	}

	return f.Close()
}

// cacheKey computes a consistent cache key from a source string.
// The key is the SHA256 hash of the source, ensuring consistent lookups
// regardless of how the source is formatted.
func (m *CacheManager) cacheKey(source string) string {
	h := sha256.Sum256([]byte(source))
	return hex.EncodeToString(h[:])
}

// imageDirName returns the directory path for storing image files.
// The directory is named after the image name for human readability.
func (m *CacheManager) imageDirName(name string) string {
	return filepath.Join(m.cacheDir, name)
}

// computeChecksum computes the SHA256 checksum of a file.
func (m *CacheManager) computeChecksum(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := copyWithContext(context.Background(), h, f); err != nil {
		return "", fmt.Errorf("failed to compute hash: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyWithContext copies from src to dst, respecting context cancellation.
// This is a simple wrapper around io.Copy for now, but can be extended
// to support progress reporting and cancellation.
func copyWithContext(ctx context.Context, dst interface{ Write([]byte) (int, error) }, src interface{ Read([]byte) (int, error) }) (int64, error) {
	// Simple implementation - just use a buffer-based copy
	buf := make([]byte, 32*1024)
	var written int64
	for {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		nr, err := src.Read(buf)
		if nr > 0 {
			nw, wErr := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if wErr != nil {
				return written, wErr
			}
			if nr != nw {
				return written, fmt.Errorf("short write")
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				return written, nil
			}
			return written, err
		}
	}
}
