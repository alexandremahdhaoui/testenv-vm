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

package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/crc32"
)

// ResourcePrefix returns a 6-character hex string derived from SHA256(testID)[:6].
// This prefix is used to make resource names unique across parallel test environments.
//
// Algorithm: SHA256(testID) -> hex-encode -> take first 6 characters.
// Returns "" if testID is empty.
func ResourcePrefix(testID string) string {
	if testID == "" {
		return ""
	}
	h := sha256.Sum256([]byte(testID))
	return hex.EncodeToString(h[:])[:6]
}

// SubnetOctet returns an integer in [20, 219] derived from CRC32(testID) % 200 + 20.
// This value is used as the third octet in 192.168.X.0/24 subnets for network isolation
// between parallel test environments.
//
// Algorithm: CRC32(testID) % 200 + 20, producing a value in [20, 219].
// Returns 100 if testID is empty, which is backward compatible with the existing
// default subnet 192.168.100.0/24.
//
// Collision probability: with 200 possible values, the birthday paradox gives
// approximately 5% collision probability at 5 concurrent environments and approximately
// 20% at 10 concurrent environments. This is acceptable for the current use case of
// 2-4 concurrent forge test stages. If more than 5 concurrent environments are ever
// needed, rework to file-based locking or a wider range.
func SubnetOctet(testID string) int {
	if testID == "" {
		return 100
	}
	crc := crc32.ChecksumIEEE([]byte(testID))
	return int(crc%200) + 20
}

// PrefixName returns prefix + "-" + name, producing a namespaced resource name.
// Returns name unchanged if prefix is empty, allowing non-isolated (single-env)
// usage to work without modification.
func PrefixName(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return fmt.Sprintf("%s-%s", prefix, name)
}
