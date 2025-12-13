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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownload_Success(t *testing.T) {
	expectedContent := "test file content for download"

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(expectedContent))
	}))
	defer server.Close()

	downloader := NewDownloader(WithHTTPClient(server.Client()))

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded_file.txt")

	err := downloader.Download(context.Background(), server.URL, destPath)
	if err != nil {
		t.Fatalf("Download() unexpected error: %v", err)
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(content) != expectedContent {
		t.Errorf("Downloaded content mismatch: got %q, want %q", string(content), expectedContent)
	}
}

func TestDownload_HTTPSOnly(t *testing.T) {
	downloader := NewDownloader()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded_file.txt")

	// Attempt to download from HTTP URL (not HTTPS)
	err := downloader.Download(context.Background(), "http://example.com/file.txt", destPath)
	if err == nil {
		t.Fatal("Download() expected error for HTTP URL, got nil")
	}

	if !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("Error should mention HTTPS requirement, got: %v", err)
	}
}

func TestDownload_RetryOn503(t *testing.T) {
	var callCount int32

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count <= 2 {
			// First two calls return 503
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Third call succeeds
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success after retries"))
	}))
	defer server.Close()

	downloader := NewDownloader(
		WithHTTPClient(server.Client()),
		WithMaxRetries(3),
		WithBaseBackoff(1*time.Millisecond), // Fast backoff for tests
	)

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded_file.txt")

	err := downloader.Download(context.Background(), server.URL, destPath)
	if err != nil {
		t.Fatalf("Download() unexpected error: %v", err)
	}

	finalCount := atomic.LoadInt32(&callCount)
	if finalCount != 3 {
		t.Errorf("Expected 3 calls (2 retries + 1 success), got %d", finalCount)
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(content) != "success after retries" {
		t.Errorf("Downloaded content mismatch: got %q", string(content))
	}
}

func TestDownload_NoRetryOn404(t *testing.T) {
	var callCount int32

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	downloader := NewDownloader(
		WithHTTPClient(server.Client()),
		WithMaxRetries(3),
		WithBaseBackoff(1*time.Millisecond),
	)

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded_file.txt")

	err := downloader.Download(context.Background(), server.URL, destPath)
	if err == nil {
		t.Fatal("Download() expected error for 404, got nil")
	}

	finalCount := atomic.LoadInt32(&callCount)
	if finalCount != 1 {
		t.Errorf("Expected 1 call (no retries for 404), got %d", finalCount)
	}

	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Error should mention 404, got: %v", err)
	}
}

func TestDownload_ContextCancellation(t *testing.T) {
	// Server that delays response
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait for context to be cancelled
		<-r.Context().Done()
	}))
	defer server.Close()

	downloader := NewDownloader(WithHTTPClient(server.Client()))

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded_file.txt")

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := downloader.Download(ctx, server.URL, destPath)
	if err == nil {
		t.Fatal("Download() expected error on context cancellation, got nil")
	}

	if !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "canceled") {
		t.Errorf("Error should indicate context cancellation, got: %v", err)
	}
}

func TestVerifyChecksum_Match(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_file.txt")
	content := []byte("content to hash")

	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Compute expected checksum
	h := sha256.Sum256(content)
	expectedChecksum := hex.EncodeToString(h[:])

	downloader := NewDownloader()

	err := downloader.VerifyChecksum(filePath, expectedChecksum)
	if err != nil {
		t.Errorf("VerifyChecksum() unexpected error: %v", err)
	}

	// Test case-insensitive comparison
	err = downloader.VerifyChecksum(filePath, strings.ToUpper(expectedChecksum))
	if err != nil {
		t.Errorf("VerifyChecksum() should be case-insensitive, got error: %v", err)
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_file.txt")
	content := []byte("content to hash")

	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	wrongChecksum := "0000000000000000000000000000000000000000000000000000000000000000"

	downloader := NewDownloader()

	err := downloader.VerifyChecksum(filePath, wrongChecksum)
	if err == nil {
		t.Fatal("VerifyChecksum() expected error for mismatched checksum, got nil")
	}

	// Check that error message is descriptive
	errStr := err.Error()
	if !strings.Contains(errStr, "checksum mismatch") {
		t.Errorf("Error should mention 'checksum mismatch', got: %v", err)
	}
	if !strings.Contains(errStr, wrongChecksum) {
		t.Errorf("Error should contain expected checksum, got: %v", err)
	}
	if !strings.Contains(errStr, filePath) {
		t.Errorf("Error should contain file path, got: %v", err)
	}
}

func TestVerifyChecksum_FileNotFound(t *testing.T) {
	downloader := NewDownloader()

	err := downloader.VerifyChecksum("/nonexistent/path/to/file.txt", "somechecksum")
	if err == nil {
		t.Fatal("VerifyChecksum() expected error for missing file, got nil")
	}

	if !strings.Contains(err.Error(), "failed to open file") {
		t.Errorf("Error should indicate file open failure, got: %v", err)
	}
}

func TestVerifyChecksum_EmptyExpected(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test_file.txt")
	content := []byte("content to hash")

	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	downloader := NewDownloader()

	// Empty checksum should skip verification and return nil
	err := downloader.VerifyChecksum(filePath, "")
	if err != nil {
		t.Errorf("VerifyChecksum() with empty expected should return nil, got: %v", err)
	}
}
