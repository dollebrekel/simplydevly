// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package licensing

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	plaintext := []byte("my-secret-oauth-token-12345")
	ciphertext, err := Encrypt(plaintext, key)
	require.NoError(t, err)

	decrypted, err := Decrypt(ciphertext, key)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestDecryptWithWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	_, _ = rand.Read(key1)
	_, _ = rand.Read(key2)

	ciphertext, err := Encrypt([]byte("secret"), key1)
	require.NoError(t, err)

	_, err = Decrypt(ciphertext, key2)
	assert.Error(t, err)
}

func TestDeriveKeyDeterministic(t *testing.T) {
	key1, err := DeriveKey()
	require.NoError(t, err)

	key2, err := DeriveKey()
	require.NoError(t, err)

	assert.Equal(t, key1, key2, "DeriveKey should return the same key on consecutive calls")
	assert.Len(t, key1, 32, "key should be 32 bytes")
}

func TestEncryptDifferentCiphertext(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)

	plaintext := []byte("same-input")
	ct1, err := Encrypt(plaintext, key)
	require.NoError(t, err)

	ct2, err := Encrypt(plaintext, key)
	require.NoError(t, err)

	assert.NotEqual(t, ct1, ct2, "random nonce should produce different ciphertext")
}

func TestDecryptCiphertextTooShort(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)

	_, err := Decrypt([]byte("short"), key)
	assert.Error(t, err)
}
