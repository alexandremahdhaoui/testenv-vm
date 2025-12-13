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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Default configuration values for the Downloader.
const (
	defaultMaxRetries  = 3
	defaultBaseBackoff = 1 * time.Second
)

// Downloader handles HTTP downloads with retry logic and checksum verification.
// It supports streaming downloads for large files and implements exponential
// backoff for transient failures.
type Downloader struct {
	httpClient  *http.Client
	maxRetries  int
	baseBackoff time.Duration
}

// DownloaderOption is a functional option for configuring a Downloader.
type DownloaderOption func(*Downloader)

// WithHTTPClient sets a custom HTTP client for the Downloader.
// This is primarily used for testing with httptest servers.
func WithHTTPClient(client *http.Client) DownloaderOption {
	return func(d *Downloader) {
		d.httpClient = client
	}
}

// WithMaxRetries sets the maximum number of retry attempts.
func WithMaxRetries(retries int) DownloaderOption {
	return func(d *Downloader) {
		d.maxRetries = retries
	}
}

// WithBaseBackoff sets the base backoff duration for retries.
// The actual backoff will be baseBackoff * 2^attempt.
func WithBaseBackoff(backoff time.Duration) DownloaderOption {
	return func(d *Downloader) {
		d.baseBackoff = backoff
	}
}

// NewDownloader creates a new Downloader with the given options.
// Default values: maxRetries=3, baseBackoff=1s, httpClient=http.DefaultClient.
func NewDownloader(opts ...DownloaderOption) *Downloader {
	d := &Downloader{
		httpClient:  http.DefaultClient,
		maxRetries:  defaultMaxRetries,
		baseBackoff: defaultBaseBackoff,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// Download downloads a file from the given URL to the destination path.
// It validates that the URL uses HTTPS, creates parent directories if needed,
// streams the content to a temporary file, and atomically renames on success.
//
// Download implements exponential backoff retry for transient errors (5xx,
// network timeouts, connection reset). It does NOT retry on 404, 403, or
// invalid URLs.
func (d *Downloader) Download(ctx context.Context, downloadURL, destPath string) error {
	// Validate URL is HTTPS
	parsedURL, err := url.Parse(downloadURL)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", downloadURL, err)
	}

	if parsedURL.Scheme != "https" {
		return fmt.Errorf("URL must use HTTPS scheme, got %q", parsedURL.Scheme)
	}

	// Create parent directories if needed
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Temp file for atomic write
	tmpPath := destPath + ".tmp"

	var lastErr error
	for attempt := 0; attempt < d.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := d.baseBackoff * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		lastErr = d.downloadOnce(ctx, downloadURL, tmpPath)
		if lastErr == nil {
			// Success - atomic rename
			if err := os.Rename(tmpPath, destPath); err != nil {
				// Clean up temp file on rename failure
				_ = os.Remove(tmpPath)
				return fmt.Errorf("failed to rename temp file: %w", err)
			}
			return nil
		}

		// Check if error is retryable
		var httpErr *httpError
		if errors.As(lastErr, &httpErr) {
			if !isRetryableStatusCode(httpErr.StatusCode) {
				// Clean up temp file
				_ = os.Remove(tmpPath)
				return lastErr
			}
		} else if errors.Is(lastErr, context.Canceled) || errors.Is(lastErr, context.DeadlineExceeded) {
			// Context cancellation is not retryable
			_ = os.Remove(tmpPath)
			return lastErr
		}

		// For network errors, check if retryable
		if !isRetryableError(lastErr) {
			_ = os.Remove(tmpPath)
			return lastErr
		}
	}

	// Clean up temp file after all retries exhausted
	_ = os.Remove(tmpPath)
	return fmt.Errorf("download failed after %d attempts: %w", d.maxRetries, lastErr)
}

// downloadOnce performs a single download attempt.
func (d *Downloader) downloadOnce(ctx context.Context, downloadURL, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return &httpError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			URL:        downloadURL,
		}
	}

	// Create temp file for streaming
	tmpFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = tmpFile.Close() }()

	// Stream response body to file
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// VerifyChecksum verifies that the file at filePath matches the expected SHA256 checksum.
// If expectedSHA256 is empty, verification is skipped and nil is returned.
// This allows well-known images without embedded checksums to skip verification.
func (d *Downloader) VerifyChecksum(filePath, expectedSHA256 string) error {
	// Skip verification if no checksum provided
	if expectedSHA256 == "" {
		return nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for checksum verification: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("failed to compute checksum: %w", err)
	}

	actualChecksum := hex.EncodeToString(h.Sum(nil))

	// Case-insensitive comparison
	if !strings.EqualFold(actualChecksum, expectedSHA256) {
		return &checksumMismatchError{
			FilePath: filePath,
			Expected: expectedSHA256,
			Actual:   actualChecksum,
		}
	}

	return nil
}

// httpError represents an HTTP error response.
type httpError struct {
	StatusCode int
	Status     string
	URL        string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("HTTP %s for URL %s", e.Status, e.URL)
}

// checksumMismatchError represents a checksum verification failure.
type checksumMismatchError struct {
	FilePath string
	Expected string
	Actual   string
}

func (e *checksumMismatchError) Error() string {
	return fmt.Sprintf("checksum mismatch for %s: expected %s, got %s", e.FilePath, e.Expected, e.Actual)
}

// isRetryableStatusCode returns true if the HTTP status code indicates a
// transient error that should be retried.
func isRetryableStatusCode(statusCode int) bool {
	// 5xx server errors are retryable
	if statusCode >= 500 && statusCode < 600 {
		return true
	}
	return false
}

// isRetryableError determines if an error is transient and should be retried.
// Network timeouts, connection resets, and temporary failures are retryable.
// 404, 403, invalid URLs are NOT retryable.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for HTTP errors
	var httpErr *httpError
	if errors.As(err, &httpErr) {
		return isRetryableStatusCode(httpErr.StatusCode)
	}

	// Check error message for common network errors
	errStr := err.Error()
	retryablePatterns := []string{
		"connection reset",
		"connection refused",
		"no such host",
		"timeout",
		"temporary failure",
		"i/o timeout",
		"network is unreachable",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}

	return false
}
