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

func waitForReadiness(spec *providerv1.ReadinessSpec, ip string) *providerv1.OperationError {
	if spec == nil {
		return nil
	}

	if ip == "" {
		return providerv1.NewProviderError("readiness check failed: VM has no IP address", true)
	}

	log.Printf("Readiness check for %s: SSH=%v CloudInit=%v TCP=%v",
		ip,
		spec.SSH != nil && spec.SSH.Enabled,
		spec.CloudInit != nil && spec.CloudInit.Enabled,
		spec.TCP != nil && spec.TCP.Port > 0,
	)

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

	if spec.SSH != nil && spec.SSH.Enabled {
		if err := waitForSSH(sshConfig, spec.SSH, ip); err != nil {
			return err
		}
		log.Printf("SSH readiness check passed for %s (fingerprint=%s)", ip, fingerprint)
		verifySSHSession(sshConfig, ip)
	}

	if spec.CloudInit != nil && spec.CloudInit.Enabled {
		if spec.SSH == nil || !spec.SSH.Enabled {
			return providerv1.NewInvalidSpecError("cloud-init readiness check requires SSH readiness to be enabled")
		}
		if err := waitForCloudInit(sshConfig, fingerprint, spec.CloudInit, spec.SSH, ip); err != nil {
			return err
		}
	}

	if spec.TCP != nil && spec.TCP.Port > 0 {
		if err := waitForTCP(spec.TCP, ip); err != nil {
			return err
		}
	}

	return nil
}

func verifySSHSession(sshConfig *ssh.ClientConfig, ip string) {
	addr := net.JoinHostPort(ip, "22")
	verifyConn, dialErr := ssh.Dial("tcp", addr, sshConfig)
	if dialErr != nil {
		log.Printf("WARNING: SSH verification dial failed immediately after waitForSSH for %s: %v", ip, dialErr)
		return
	}
	defer func() { _ = verifyConn.Close() }()
	session, sessErr := verifyConn.NewSession()
	if sessErr != nil {
		log.Printf("WARNING: SSH verification session failed for %s: %v", ip, sessErr)
		return
	}
	defer func() { _ = session.Close() }()
	var out bytes.Buffer
	session.Stdout = &out
	if runErr := session.Run("whoami"); runErr != nil {
		log.Printf("WARNING: SSH verification whoami failed for %s: %v", ip, runErr)
		return
	}
	log.Printf("SSH verification whoami=%q for %s", out.String(), ip)
}

func waitForTCP(spec *providerv1.TCPReadinessSpec, ip string) *providerv1.OperationError {
	timeout, specErr := readinessTimeout("tcp", spec.Timeout, 3*time.Minute)
	if specErr != nil {
		return specErr
	}
	host := ip
	if spec.Address != "" {
		host = spec.Address
	}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", spec.Port))
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err == nil {
			_ = conn.Close()
			log.Printf("TCP readiness check passed for %s", addr)
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return providerv1.NewTimeoutError(fmt.Sprintf("tcp readiness on %s after %s", addr, timeout))
}

func readinessTimeout(check, declared string, fallback time.Duration) (time.Duration, *providerv1.OperationError) {
	if declared == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(declared)
	if err != nil {
		return 0, providerv1.NewInvalidSpecError(fmt.Sprintf("invalid %s readiness timeout %q: %v", check, declared, err))
	}
	return parsed, nil
}

func ipResolutionBudget(readiness *providerv1.ReadinessSpec) (time.Duration, *providerv1.OperationError) {
	if readiness == nil {
		return 60 * time.Second, nil
	}
	if readiness.SSH != nil {
		return readinessTimeout("SSH", readiness.SSH.Timeout, 3*time.Minute)
	}
	if readiness.TCP != nil {
		return readinessTimeout("tcp", readiness.TCP.Timeout, 3*time.Minute)
	}
	return 60 * time.Second, nil
}

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

func waitForCloudInit(sshConfig *ssh.ClientConfig, fingerprint string, ciSpec *providerv1.CloudInitReadinessSpec, sshSpec *providerv1.SSHReadinessSpec, ip string) *providerv1.OperationError {
	timeout, err := time.ParseDuration(ciSpec.Timeout)
	if err != nil {
		return providerv1.NewInvalidSpecError(fmt.Sprintf("invalid cloud-init readiness timeout %q: %v", ciSpec.Timeout, err))
	}

	log.Printf("waitForCloudInit: user=%s, key=%s, fingerprint=%s, ip=%s", sshSpec.User, sshSpec.PrivateKey, fingerprint, ip)

	addr := net.JoinHostPort(ip, "22")
	deadline := time.Now().Add(timeout)
	pollInterval := 10 * time.Second

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
