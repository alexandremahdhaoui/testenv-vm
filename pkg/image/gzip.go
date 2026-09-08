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
	"compress/gzip"
	"fmt"
	"io"
	"os"
)

func (s *ImageState) verifiedPath() string {
	if s.CompressedPath != "" {
		return s.CompressedPath
	}
	return s.LocalPath
}

func decompressGzip(compressedPath, outputPath string) error {
	source, err := os.Open(compressedPath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", compressedPath, err)
	}
	defer func() { _ = source.Close() }()

	reader, err := gzip.NewReader(source)
	if err != nil {
		return fmt.Errorf("reading gzip header of %s: %w", compressedPath, err)
	}
	defer func() { _ = reader.Close() }()

	output, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", outputPath, err)
	}

	if _, err := io.Copy(output, reader); err != nil {
		_ = output.Close()
		return fmt.Errorf("writing %s: %w", outputPath, err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", outputPath, err)
	}
	return nil
}
