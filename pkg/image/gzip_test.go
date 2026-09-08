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
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	v1 "github.com/alexandremahdhaoui/testenv-vm/api/v1"
)

func gzipBytes(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write([]byte(content)); err != nil {
		t.Fatalf("writing gzip content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing gzip writer: %v", err)
	}
	return buf.Bytes()
}

func newGzipCacheManager(t *testing.T, compressed []byte) (*CacheManager, *httptest.Server, *int32) {
	t.Helper()
	var requests int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(compressed)
	}))
	t.Cleanup(server.Close)

	downloader := NewDownloader(
		WithHTTPClient(server.Client()),
		WithMaxRetries(1),
		WithBaseBackoff(1*time.Millisecond),
	)
	m, err := NewCacheManager(filepath.Join(t.TempDir(), "cache"), WithDownloader(downloader))
	if err != nil {
		t.Fatalf("NewCacheManager() unexpected error: %v", err)
	}
	return m, server, &requests
}

func TestEnsureImageVerifiesAGzipSourceOverTheCompressedBytesAndDecompressesItBeside(t *testing.T) {
	t.Cleanup(ResetRegistry)
	content := "raw disk image bytes"
	compressed := gzipBytes(t, content)
	sum := sha256.Sum256(compressed)

	m, server, _ := newGzipCacheManager(t, compressed)
	state, err := m.EnsureImage(context.Background(), "openwrt", v1.ImageSpec{
		Source: server.URL + "/openwrt-x86-64-generic-ext4-combined.img.gz",
		Sha256: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatalf("EnsureImage() unexpected error: %v", err)
	}

	if strings.HasSuffix(state.LocalPath, ".gz") {
		t.Errorf("LocalPath %q should point at the decompressed image", state.LocalPath)
	}
	if filepath.Dir(state.CompressedPath) != filepath.Dir(state.LocalPath) {
		t.Errorf("CompressedPath %q should sit beside LocalPath %q", state.CompressedPath, state.LocalPath)
	}
	got, err := os.ReadFile(state.LocalPath)
	if err != nil {
		t.Fatalf("reading decompressed image: %v", err)
	}
	if string(got) != content {
		t.Errorf("decompressed content = %q, want %q", string(got), content)
	}
	if state.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("SHA256 = %q, want the compressed checksum %q", state.SHA256, hex.EncodeToString(sum[:]))
	}
}

func TestEnsureImageRejectsAGzipSourceWhoseCompressedBytesDoNotMatchTheDeclaredSha256(t *testing.T) {
	t.Cleanup(ResetRegistry)
	compressed := gzipBytes(t, "raw disk image bytes")
	wrong := sha256.Sum256([]byte("something else"))

	m, server, _ := newGzipCacheManager(t, compressed)
	_, err := m.EnsureImage(context.Background(), "openwrt", v1.ImageSpec{
		Source: server.URL + "/image.img.gz",
		Sha256: hex.EncodeToString(wrong[:]),
	})
	if err == nil {
		t.Fatal("EnsureImage() should fail on a checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("error should name the checksum, got: %v", err)
	}
}

func TestEnsureImageSkipsTheDownloadOnASecondRunOfAGzipSource(t *testing.T) {
	t.Cleanup(ResetRegistry)
	compressed := gzipBytes(t, "raw disk image bytes")
	sum := sha256.Sum256(compressed)

	m, server, requests := newGzipCacheManager(t, compressed)
	spec := v1.ImageSpec{
		Source: server.URL + "/image.img.gz",
		Sha256: hex.EncodeToString(sum[:]),
	}
	first, err := m.EnsureImage(context.Background(), "openwrt", spec)
	if err != nil {
		t.Fatalf("EnsureImage() first call unexpected error: %v", err)
	}
	second, err := m.EnsureImage(context.Background(), "openwrt", spec)
	if err != nil {
		t.Fatalf("EnsureImage() second call unexpected error: %v", err)
	}
	if atomic.LoadInt32(requests) != 1 {
		t.Errorf("server saw %d requests, want 1", atomic.LoadInt32(requests))
	}
	if second.LocalPath != first.LocalPath {
		t.Errorf("second LocalPath = %q, want %q", second.LocalPath, first.LocalPath)
	}
}

func TestDecompressGzipRefusesAFileThatIsNotGzip(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.gz")
	if err := os.WriteFile(plain, []byte("not gzip"), 0o644); err != nil {
		t.Fatalf("writing file: %v", err)
	}
	err := decompressGzip(plain, filepath.Join(dir, "plain"))
	if err == nil {
		t.Fatal("decompressGzip() should fail on a plain file")
	}
	if !strings.Contains(err.Error(), "gzip header") {
		t.Errorf("error should name the gzip header, got: %v", err)
	}
}
