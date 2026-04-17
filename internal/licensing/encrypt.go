// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package licensing

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

const (
	keyLen         = 32
	pbkdf2Iter     = 100_000
	derivationSalt = "siply-account-token-v1"
)

// Encrypt encrypts plaintext using AES-256-GCM with the given key.
func Encrypt(plaintext, key []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, errors.New("encrypt: plaintext must not be empty")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts ciphertext using AES-256-GCM with the given key.
func Decrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("decrypt: ciphertext too short")
	}
	return gcm.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], nil)
}

// DeriveKey derives a 32-byte AES key from machine-specific entropy using PBKDF2-SHA256.
func DeriveKey() ([]byte, error) {
	entropy, err := machineEntropy()
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	key := pbkdf2.Key([]byte(entropy), []byte(derivationSalt), pbkdf2Iter, keyLen, sha256.New)
	return key, nil
}

// machineEntropy combines machine-id (or platform equivalent) with hostname and UID.
func machineEntropy() (string, error) {
	var parts []string

	mid := readMachineID()
	if mid != "" {
		parts = append(parts, mid)
	}

	hostname, err := os.Hostname()
	if err == nil && hostname != "" {
		parts = append(parts, hostname)
	}

	u, err := user.Current()
	if err == nil && u.Uid != "" {
		parts = append(parts, u.Uid)
	}

	if len(parts) == 0 {
		return "", errors.New("no machine entropy available")
	}
	return strings.Join(parts, ":"), nil
}

// readMachineID reads the machine identifier from the OS.
func readMachineID() string {
	switch runtime.GOOS {
	case "linux":
		if data, err := os.ReadFile("/etc/machine-id"); err == nil {
			return strings.TrimSpace(string(data))
		}
		if data, err := os.ReadFile("/var/lib/dbus/machine-id"); err == nil {
			return strings.TrimSpace(string(data))
		}
	case "darwin":
		return readMacOSUUID()
	}
	return ""
}

// readMacOSUUID reads the IOPlatformUUID on macOS.
// Defined as a variable for testing.
var readMacOSUUID = defaultReadMacOSUUID

func defaultReadMacOSUUID() string {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "IOPlatformUUID") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.Trim(strings.TrimSpace(parts[1]), "\"")
			}
		}
	}
	return ""
}
