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

package client

import "errors"

// VMInfo holds connection information for a VM.
type VMInfo struct {
	// Host is the IP address or hostname of the VM
	Host string
	// Port is the SSH port (default "22")
	Port string
	// User is the SSH username
	User string
	// PrivateKey is the SSH private key content ([]byte for crypto/ssh)
	PrivateKey []byte
}

// Validate returns an error if required fields are missing.
// Both nil and empty PrivateKey return an error.
func (v *VMInfo) Validate() error {
	if v.Host == "" {
		return errors.New("vminfo: Host is required")
	}
	if v.Port == "" {
		return errors.New("vminfo: Port is required")
	}
	if v.User == "" {
		return errors.New("vminfo: User is required")
	}
	if len(v.PrivateKey) == 0 {
		return errors.New("vminfo: PrivateKey is required")
	}
	return nil
}
