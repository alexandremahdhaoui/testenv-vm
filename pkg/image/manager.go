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

type CacheManager struct {
	cacheDir   string
	downloader *Downloader
	metadata   *CacheMetadata
	mu         sync.Mutex
}

type CacheManagerOption func(*CacheManager)

func WithDownloader(d *Downloader) CacheManagerOption {
	return func(m *CacheManager) {
		m.downloader = d
	}
}

func NewCacheManager(cacheDir string, opts ...CacheManagerOption) (*CacheManager, error) {
	m := &CacheManager{
		cacheDir:   cacheDir,
		downloader: NewDownloader(),
	}

	for _, opt := range opts {
		opt(m)
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	locksDir := filepath.Join(cacheDir, ".locks")
	if err := os.MkdirAll(locksDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create locks directory: %w", err)
	}

	if err := m.loadMetadata(); err != nil {
		return nil, fmt.Errorf("failed to load metadata: %w", err)
	}

	return m, nil
}

func (m *CacheManager) EnsureImage(ctx context.Context, name string, spec v1.ImageSpec) (*ImageState, error) {
	source := spec.Source
	var resolvedURL string
	var expectedSHA256 string

	wellKnown, isWellKnown := Resolve(source)
	if isWellKnown {
		resolvedURL = wellKnown.URL
		if spec.Sha256 != "" {
			expectedSHA256 = spec.Sha256
		} else {
			expectedSHA256 = wellKnown.SHA256
		}
	} else {
		if !strings.HasPrefix(source, "https://") {
			return nil, fmt.Errorf("direct URL must use HTTPS: %s", source)
		}
		resolvedURL = source
		expectedSHA256 = spec.Sha256
	}

	key := m.cacheKeyWithCustomize(source, spec.Customize)

	lockFile, err := m.acquireFileLock(key)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() {
		_ = m.releaseFileLock(lockFile)
	}()

	m.mu.Lock()
	existing, found := m.metadata.Images[key]
	m.mu.Unlock()

	if found && existing.Status == StatusReady {
		if _, err := os.Stat(existing.LocalPath); err == nil {
			if expectedSHA256 != "" {
				if err := m.downloader.VerifyChecksum(existing.verifiedPath(), expectedSHA256); err == nil {
					return existing, nil
				}
			} else {
				return existing, nil
			}
		}
	}

	if spec.Customize != nil && (len(spec.Customize.Packages) > 0 || len(spec.Customize.Runcmd) > 0) {
		return m.ensureCustomizedImage(ctx, name, spec, key)
	}

	imageDir := m.imageDirName(name)
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create image directory: %w", err)
	}

	filename := filepath.Base(resolvedURL)
	if filename == "" || filename == "." || filename == "/" {
		filename = "image"
	}
	localPath := filepath.Join(imageDir, filename)

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

	if err := m.downloader.Download(ctx, resolvedURL, localPath); err != nil {
		m.markFailed(key)
		return nil, fmt.Errorf("failed to download image: %w", err)
	}

	if expectedSHA256 != "" {
		if err := m.downloader.VerifyChecksum(localPath, expectedSHA256); err != nil {
			_ = os.Remove(localPath)
			m.markFailed(key)
			return nil, fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	actualSHA256, err := m.computeChecksum(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to compute checksum: %w", err)
	}

	compressedPath := ""
	if strings.HasSuffix(localPath, ".gz") {
		compressedPath = localPath
		localPath = strings.TrimSuffix(localPath, ".gz")
		if err := decompressGzip(compressedPath, localPath); err != nil {
			_ = os.Remove(localPath)
			m.markFailed(key)
			return nil, fmt.Errorf("decompressing image %s: %w", compressedPath, err)
		}
	}

	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat downloaded file: %w", err)
	}

	state := &ImageState{
		Name:           name,
		Source:         source,
		ResolvedURL:    resolvedURL,
		LocalPath:      localPath,
		CompressedPath: compressedPath,
		SHA256:         actualSHA256,
		Size:           fileInfo.Size(),
		DownloadedAt:   time.Now(),
		Status:         StatusReady,
	}
	if err := m.storeReady(key, state); err != nil {
		return nil, err
	}

	return state, nil
}

func (m *CacheManager) ensureCustomizedImage(ctx context.Context, name string, spec v1.ImageSpec, key string) (*ImageState, error) {
	if err := checkVirtCustomize(); err != nil {
		return nil, err
	}

	baseSpec := v1.ImageSpec{Source: spec.Source, Sha256: spec.Sha256}
	baseState, err := m.EnsureImage(ctx, name+"-base", baseSpec)
	if err != nil {
		return nil, fmt.Errorf("ensuring base image for customization: %w", err)
	}

	imageDir := m.imageDirName(name)
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating image directory: %w", err)
	}

	localPath := filepath.Join(imageDir, name+".qcow2")

	m.mu.Lock()
	m.metadata.Images[key] = &ImageState{
		Name:      name,
		Source:    spec.Source,
		LocalPath: localPath,
		Status:    StatusCustomizing,
	}
	if err := m.saveMetadata(); err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("saving metadata: %w", err)
	}
	m.mu.Unlock()

	if err := createQcow2Overlay(baseState.LocalPath, localPath); err != nil {
		cleanupPartialImage(localPath)
		m.markFailedNamed(key, name, spec.Source)
		return nil, fmt.Errorf("creating overlay: %w", err)
	}

	if err := runVirtCustomize(ctx, localPath, spec.Customize); err != nil {
		cleanupPartialImage(localPath)
		m.markFailedNamed(key, name, spec.Source)
		return nil, fmt.Errorf("customizing image: %w", err)
	}

	checksum, err := m.computeChecksum(localPath)
	if err != nil {
		return nil, fmt.Errorf("computing checksum: %w", err)
	}

	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("stat image: %w", err)
	}

	state := &ImageState{
		Name:         name,
		Source:       spec.Source,
		LocalPath:    localPath,
		SHA256:       checksum,
		Size:         fileInfo.Size(),
		DownloadedAt: time.Now(),
		Status:       StatusReady,
	}
	if err := m.storeReady(key, state); err != nil {
		return nil, err
	}

	return state, nil
}

func (m *CacheManager) markFailed(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state, ok := m.metadata.Images[key]; ok {
		state.Status = StatusFailed
	}
	_ = m.saveMetadata()
}

func (m *CacheManager) markFailedNamed(key, name, source string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metadata.Images[key] = &ImageState{Name: name, Source: source, Status: StatusFailed}
	_ = m.saveMetadata()
}

func (m *CacheManager) storeReady(key string, state *ImageState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metadata.Images[key] = state
	m.metadata.UpdatedAt = time.Now()
	if err := m.saveMetadata(); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}
	return nil
}

func (m *CacheManager) GetImagePath(name string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, img := range m.metadata.Images {
		if img.Name == name && img.Status == StatusReady {
			return img.LocalPath, true
		}
	}
	return "", false
}

func (m *CacheManager) loadMetadata() error {
	metadataPath := filepath.Join(m.cacheDir, "metadata.json")

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
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

	if metadata.Images == nil {
		metadata.Images = make(map[string]*ImageState)
	}

	m.metadata = &metadata
	return nil
}

func (m *CacheManager) saveMetadata() error {
	metadataPath := filepath.Join(m.cacheDir, "metadata.json")

	data, err := json.MarshalIndent(m.metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

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

func (m *CacheManager) acquireFileLock(key string) (*os.File, error) {
	lockPath := filepath.Join(m.cacheDir, ".locks", key+".lock")

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("failed to acquire flock: %w", err)
	}

	return f, nil
}

func (m *CacheManager) releaseFileLock(f *os.File) error {
	if f == nil {
		return nil
	}

	if err := unix.Flock(int(f.Fd()), unix.LOCK_UN); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to release flock: %w", err)
	}

	return f.Close()
}

func (m *CacheManager) cacheKey(source string) string {
	h := sha256.Sum256([]byte(source))
	return hex.EncodeToString(h[:])
}

func (m *CacheManager) cacheKeyWithCustomize(source string, customize *v1.ImageCustomizeSpec) string {
	if customize == nil || (len(customize.Packages) == 0 && len(customize.Runcmd) == 0) {
		return m.cacheKey(source)
	}
	customizeJSON, _ := json.Marshal(customize)
	combined := source + "|customize|" + string(customizeJSON)
	h := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(h[:])
}

func (m *CacheManager) imageDirName(name string) string {
	return filepath.Join(m.cacheDir, name)
}

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

func copyWithContext(ctx context.Context, dst interface{ Write([]byte) (int, error) }, src interface{ Read([]byte) (int, error) }) (int64, error) {
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
