// Package cryptobox provides authenticated symmetric encryption for
// at-rest credential storage (e.g. per-org third-party API keys held
// inside the fork rather than in The Brain's Hub vault).
//
// The construction is AES-256-GCM: a 32-byte master key, a random
// 96-bit nonce per Seal, and GCM's built-in authentication tag so any
// tampering with the ciphertext is detected on Open.
//
// PII / secret handling: the plaintext passed to Seal and recovered
// from Open is PII-Restricted (per internal/pii) — it is upstream
// credential material — and the master key itself is the highest-value
// secret in the fork. NEITHER the plaintext NOR the master key may
// EVER be written to a log, error message, span attribute, or metric.
// Errors returned from this package are intentionally detail-free for
// exactly this reason (see ErrDecrypt).
package cryptobox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// keyLen is the required master-key length in bytes: 32 == AES-256.
const keyLen = 32

// nonceLen is the GCM nonce length in bytes: 12 == the 96-bit nonce
// recommended by NIST SP 800-38D for AES-GCM. Each Seal generates a
// fresh random nonce; storage persists it alongside the ciphertext.
const nonceLen = 12

// ErrDecrypt is returned by Open whenever authentication or decryption
// fails — wrong key, wrong/short nonce, or tampered ciphertext. It is
// intentionally uniform and detail-free: leaking *why* a decrypt failed
// gives an attacker a probing oracle, and the inputs are PII-Restricted
// secret material that must not appear in error text.
var ErrDecrypt = errors.New("cryptobox: decryption failed")

// Cipher wraps a configured AES-256-GCM AEAD plus the version of the
// master key it was built from. The version is returned by Seal so a
// key-rotation scheme can record which key encrypted each row and pick
// the right Cipher on Open.
type Cipher struct {
	aead       cipher.AEAD
	keyVersion int
}

// NewCipher builds a Cipher from a 32-byte (AES-256) master key and a
// caller-assigned key version. It returns a clear error when the key
// length is wrong — the only validation detail this package surfaces,
// since the key bytes themselves must never appear in the message.
//
// The master key is PII-Restricted secret material: callers must not
// log it, and should zero their copy when no longer needed.
func NewCipher(masterKey []byte, version int) (*Cipher, error) {
	if len(masterKey) != keyLen {
		return nil, fmt.Errorf("cryptobox: master key must be %d bytes (AES-256), got %d", keyLen, len(masterKey))
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		// aes.NewCipher only fails on a bad key length, which we
		// already guarded — but surface it without echoing the key.
		return nil, fmt.Errorf("cryptobox: build AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cryptobox: build GCM: %w", err)
	}
	return &Cipher{aead: aead, keyVersion: version}, nil
}

// Seal encrypts plaintext under the configured key. It generates a
// fresh random 12-byte nonce per call and returns the ciphertext, the
// nonce, and the key version separately so a storage layer can persist
// each in its own column.
//
// plaintext is PII-Restricted secret material — never log it. The
// returned ciphertext carries GCM's authentication tag appended, so
// Open will reject any later tampering.
func (c *Cipher) Seal(plaintext []byte) (ciphertext []byte, nonce []byte, version int, err error) {
	nonce = make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, 0, fmt.Errorf("cryptobox: generate nonce: %w", err)
	}
	ciphertext = c.aead.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, c.keyVersion, nil
}

// Open decrypts and authenticates a ciphertext produced by Seal, using
// the nonce stored alongside it. On any failure — wrong key, wrong or
// short nonce, or tampered ciphertext — it returns the uniform
// ErrDecrypt with no further detail.
//
// The recovered plaintext is PII-Restricted secret material — never
// log it.
func (c *Cipher) Open(ciphertext, nonce []byte) (plaintext []byte, err error) {
	if len(nonce) != nonceLen {
		// A malformed nonce can never authenticate; collapse it into
		// the same uniform error rather than panicking inside the AEAD.
		return nil, ErrDecrypt
	}
	plaintext, err = c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}

// KeyVersion returns the version assigned at construction. Useful when
// a caller holds several Cipher instances during a key rotation and
// needs to record which one sealed a given row.
func (c *Cipher) KeyVersion() int { return c.keyVersion }

// ParseMasterKey decodes a standard-base64 string into a 32-byte master
// key, validating the length. It is the canonical way to load a key
// from config/secret-source material.
//
// The decoded bytes are PII-Restricted secret material — never log the
// input or output.
func ParseMasterKey(b64 string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		// Do not echo the offending string — it is secret material.
		return nil, errors.New("cryptobox: master key is not valid base64")
	}
	if len(raw) != keyLen {
		return nil, fmt.Errorf("cryptobox: master key must decode to %d bytes (AES-256), got %d", keyLen, len(raw))
	}
	return raw, nil
}
