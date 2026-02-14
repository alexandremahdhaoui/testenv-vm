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

package libvirt

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	providerv1 "github.com/alexandremahdhaoui/testenv-vm/api/provider/v1"
	"golang.org/x/crypto/ssh"
)

// waitForReadiness performs readiness checks on a VM after it has been started.
// It checks SSH connectivity and cloud-init completion based on the readiness spec.
// Returns nil if all enabled checks pass, or an OperationError on failure.
func waitForReadiness(spec *providerv1.ReadinessSpec, ip string) *providerv1.OperationError {
	if spec == nil {
		return nil
	}

	if ip == "" {
		return providerv1.NewProviderError("readiness check failed: VM has no IP address", true)
	}

	log.Printf("Readiness check for %s: SSH=%v CloudInit=%v keyPath=%q user=%q",
		ip,
		spec.SSH != nil && spec.SSH.Enabled,
		spec.CloudInit != nil && spec.CloudInit.Enabled,
		func() string {
			if spec.SSH != nil {
				return spec.SSH.PrivateKey
			}
			return ""
		}(),
		func() string {
			if spec.SSH != nil {
				return spec.SSH.User
			}
			return ""
		}(),
	)

	// Build SSH config once and reuse for both phases.
	var sshConfig *ssh.ClientConfig
	var fingerprint string
	if spec.SSH != nil && spec.SSH.Enabled {
		cfg, fp, opErr := buildSSHClientConfig(spec.SSH)
		if opErr != nil {
			return opErr
		}
		sshConfig = cfg
		fingerprint = fp
		log.Printf("Built SSH config: user=%s keyPath=%s fingerprint=%s", spec.SSH.User, spec.SSH.PrivateKey, fingerprint)
	}

	// Phase 1: SSH readiness
	if spec.SSH != nil && spec.SSH.Enabled {
		if err := waitForSSH(sshConfig, spec.SSH, ip); err != nil {
			return err
		}
		log.Printf("SSH readiness check passed for %s (fingerprint=%s)", ip, fingerprint)

		// Immediately verify auth still works before entering cloud-init phase.
		addr := net.JoinHostPort(ip, "22")
		verifyConn, dialErr := ssh.Dial("tcp", addr, sshConfig)
		if dialErr != nil {
			log.Printf("WARNING: SSH verification dial failed immediately after waitForSSH for %s: %v", ip, dialErr)
		} else {
			session, sessErr := verifyConn.NewSession()
			if sessErr != nil {
				log.Printf("WARNING: SSH verification session failed for %s: %v", ip, sessErr)
			} else {
				var out bytes.Buffer
				session.Stdout = &out
				if runErr := session.Run("whoami"); runErr != nil {
					log.Printf("WARNING: SSH verification whoami failed for %s: %v", ip, runErr)
				} else {
					log.Printf("SSH verification whoami=%q for %s", out.String(), ip)
				}
				_ = session.Close()
			}
			_ = verifyConn.Close()
		}
	}

	// Phase 2: Cloud-init readiness (requires SSH)
	if spec.CloudInit != nil && spec.CloudInit.Enabled {
		if spec.SSH == nil || !spec.SSH.Enabled {
			return providerv1.NewInvalidSpecError("cloud-init readiness check requires SSH readiness to be enabled")
		}
		if err := waitForCloudInit(sshConfig, fingerprint, spec.CloudInit, spec.SSH, ip); err != nil {
			return err
		}
	}

	return nil
}

// waitForSSH polls for SSH connectivity until the timeout is reached.
func waitForSSH(sshConfig *ssh.ClientConfig, spec *providerv1.SSHReadinessSpec, ip string) *providerv1.OperationError {
	timeout, err := time.ParseDuration(spec.Timeout)
	if err != nil {
		return providerv1.NewInvalidSpecError(fmt.Sprintf("invalid SSH readiness timeout %q: %v", spec.Timeout, err))
	}

	addr := net.JoinHostPort(ip, "22")
	deadline := time.Now().Add(timeout)
	pollInterval := 5 * time.Second

	var lastErr error
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		conn, dialErr := ssh.Dial("tcp", addr, sshConfig)
		if dialErr == nil {
			// Verify auth by running a command.
			session, sessErr := conn.NewSession()
			if sessErr != nil {
				log.Printf("SSH check attempt %d: dial OK but session failed for %s: %v", attempt, ip, sessErr)
				_ = conn.Close()
				lastErr = sessErr
				time.Sleep(pollInterval)
				continue
			}
			var out bytes.Buffer
			session.Stdout = &out
			runErr := session.Run("echo ssh-ready")
			_ = session.Close()
			_ = conn.Close()
			if runErr != nil {
				log.Printf("SSH check attempt %d: dial+session OK but command failed for %s: %v", attempt, ip, runErr)
				lastErr = runErr
				time.Sleep(pollInterval)
				continue
			}
			log.Printf("SSH check attempt %d: fully verified for %s (output=%q)", attempt, ip, out.String())
			return nil
		}
		lastErr = dialErr
		if attempt <= 3 || attempt%10 == 0 {
			log.Printf("SSH check attempt %d: dial failed for %s: %v", attempt, ip, dialErr)
		}
		time.Sleep(pollInterval)
	}

	return providerv1.NewProviderError(
		fmt.Sprintf("SSH readiness timeout after %s for %s@%s (attempts=%d): %v", spec.Timeout, spec.User, ip, attempt, lastErr),
		true,
	)
}

// waitForCloudInit waits for cloud-init to finish by running a command over SSH.
func waitForCloudInit(sshConfig *ssh.ClientConfig, fingerprint string, ciSpec *providerv1.CloudInitReadinessSpec, sshSpec *providerv1.SSHReadinessSpec, ip string) *providerv1.OperationError {
	timeout, err := time.ParseDuration(ciSpec.Timeout)
	if err != nil {
		return providerv1.NewInvalidSpecError(fmt.Sprintf("invalid cloud-init readiness timeout %q: %v", ciSpec.Timeout, err))
	}

	log.Printf("waitForCloudInit: user=%s, key=%s, fingerprint=%s, ip=%s", sshSpec.User, sshSpec.PrivateKey, fingerprint, ip)

	addr := net.JoinHostPort(ip, "22")
	deadline := time.Now().Add(timeout)
	pollInterval := 10 * time.Second

	// cloud-init status --wait blocks until completion, but we add a timeout
	// wrapper to prevent indefinite hangs, plus a fallback check for the
	// boot-finished file. Consistent with pkg/client/client.go WaitReady().
	cmd := "timeout 60 cloud-init status --wait || test -f /var/lib/cloud/instance/boot-finished"

	var lastErr error
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		conn, dialErr := ssh.Dial("tcp", addr, sshConfig)
		if dialErr != nil {
			log.Printf("Cloud-init check attempt %d: SSH dial failed for %s: %v", attempt, ip, dialErr)
			lastErr = dialErr
			time.Sleep(pollInterval)
			continue
		}

		session, sessErr := conn.NewSession()
		if sessErr != nil {
			_ = conn.Close()
			log.Printf("Cloud-init check attempt %d: SSH session failed for %s: %v", attempt, ip, sessErr)
			lastErr = sessErr
			time.Sleep(pollInterval)
			continue
		}

		var stderrBuf bytes.Buffer
		session.Stderr = &stderrBuf

		runErr := session.Run(cmd)
		_ = session.Close()
		_ = conn.Close()

		if runErr == nil {
			log.Printf("Cloud-init check attempt %d: cloud-init completed for %s", attempt, ip)
			return nil
		}
		lastErr = fmt.Errorf("%w (stderr: %s)", runErr, stderrBuf.String())
		log.Printf("Cloud-init check attempt %d: command failed for %s: %v", attempt, ip, lastErr)
		time.Sleep(pollInterval)
	}

	return providerv1.NewProviderError(
		fmt.Sprintf("cloud-init readiness timeout after %s for %s (user=%s, key=%s, fingerprint=%s, attempts=%d): %v",
			ciSpec.Timeout, ip, sshSpec.User, sshSpec.PrivateKey, fingerprint, attempt, lastErr),
		true,
	)
}

// buildSSHClientConfig builds an ssh.ClientConfig from an SSHReadinessSpec.
// Returns the config, the key fingerprint, and an optional error.
func buildSSHClientConfig(spec *providerv1.SSHReadinessSpec) (*ssh.ClientConfig, string, *providerv1.OperationError) {
	if spec.PrivateKey == "" {
		return nil, "", providerv1.NewInvalidSpecError("SSH readiness check requires a private key path")
	}
	if spec.User == "" {
		return nil, "", providerv1.NewInvalidSpecError("SSH readiness check requires a user")
	}

	keyBytes, err := os.ReadFile(spec.PrivateKey)
	if err != nil {
		return nil, "", providerv1.NewProviderError(
			fmt.Sprintf("failed to read SSH private key %q: %v", spec.PrivateKey, err),
			false,
		)
	}
	log.Printf("Read SSH private key from %q (%d bytes)", spec.PrivateKey, len(keyBytes))

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, "", providerv1.NewProviderError(
			fmt.Sprintf("failed to parse SSH private key %q: %v", spec.PrivateKey, err),
			false,
		)
	}
	fingerprint := ssh.FingerprintSHA256(signer.PublicKey())
	log.Printf("SSH public key fingerprint: %s", fingerprint)

	return &ssh.ClientConfig{
		User: spec.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}, fingerprint, nil
}
