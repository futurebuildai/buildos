package cryptobox

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

// testKey is a deterministic 32-byte key for the round-trip tests. It
// is test-only material; production keys come from the secret source.
func testKey() []byte {
	k := make([]byte, keyLen)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	c, err := NewCipher(testKey(), 1)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}

	plaintext := []byte("re_super_secret_resend_api_key")
	ciphertext, nonce, version, err := c.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if version != 1 {
		t.Fatalf("version = %d, want 1", version)
	}
	if len(nonce) != nonceLen {
		t.Fatalf("nonce len = %d, want %d", len(nonce), nonceLen)
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("ciphertext must not equal plaintext")
	}

	got, err := c.Open(ciphertext, nonce)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip mismatch: got %q want %q", got, plaintext)
	}
}

func TestSealOpenEmptyPlaintext(t *testing.T) {
	c, err := NewCipher(testKey(), 7)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	ct, nonce, _, err := c.Seal([]byte{})
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	got, err := c.Open(ct, nonce)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty plaintext, got %d bytes", len(got))
	}
}

func TestNewCipherRejectsWrongKeyLength(t *testing.T) {
	for _, n := range []int{0, 1, 16, 24, 31, 33, 64} {
		_, err := NewCipher(make([]byte, n), 1)
		if err == nil {
			t.Fatalf("NewCipher(%d bytes): expected error, got nil", n)
		}
	}
}

func TestOpenDetectsTamper(t *testing.T) {
	c, err := NewCipher(testKey(), 1)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	ct, nonce, _, err := c.Seal([]byte("tamper me"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	// Flip a single bit in the ciphertext.
	tampered := append([]byte(nil), ct...)
	tampered[0] ^= 0x01

	_, err = c.Open(tampered, nonce)
	if !errors.Is(err, ErrDecrypt) {
		t.Fatalf("tampered ciphertext: got err=%v, want ErrDecrypt", err)
	}
}

func TestOpenWrongKey(t *testing.T) {
	c1, _ := NewCipher(testKey(), 1)
	otherKey := make([]byte, keyLen)
	for i := range otherKey {
		otherKey[i] = byte(255 - i)
	}
	c2, _ := NewCipher(otherKey, 2)

	ct, nonce, _, err := c1.Seal([]byte("for c1 only"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	_, err = c2.Open(ct, nonce)
	if !errors.Is(err, ErrDecrypt) {
		t.Fatalf("wrong key: got err=%v, want ErrDecrypt", err)
	}
}

func TestOpenBadNonceLength(t *testing.T) {
	c, _ := NewCipher(testKey(), 1)
	ct, _, _, err := c.Seal([]byte("data"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	_, err = c.Open(ct, []byte("short"))
	if !errors.Is(err, ErrDecrypt) {
		t.Fatalf("bad nonce length: got err=%v, want ErrDecrypt", err)
	}
}

func TestNonceUniqueness(t *testing.T) {
	c, _ := NewCipher(testKey(), 1)
	plaintext := []byte("same plaintext both times")

	ct1, nonce1, _, err := c.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal #1: %v", err)
	}
	ct2, nonce2, _, err := c.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal #2: %v", err)
	}

	if bytes.Equal(nonce1, nonce2) {
		t.Fatal("two Seals produced identical nonces; nonce reuse breaks GCM")
	}
	if bytes.Equal(ct1, ct2) {
		t.Fatal("two Seals of same plaintext produced identical ciphertext")
	}
}

func TestKeyVersionAccessor(t *testing.T) {
	c, _ := NewCipher(testKey(), 42)
	if c.KeyVersion() != 42 {
		t.Fatalf("KeyVersion = %d, want 42", c.KeyVersion())
	}
}

func TestParseMasterKeyValid(t *testing.T) {
	raw := testKey()
	b64 := base64.StdEncoding.EncodeToString(raw)
	got, err := ParseMasterKey(b64)
	if err != nil {
		t.Fatalf("ParseMasterKey: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("ParseMasterKey did not round-trip the key bytes")
	}
}

func TestParseMasterKeyInvalid(t *testing.T) {
	t.Run("not base64", func(t *testing.T) {
		if _, err := ParseMasterKey("!!!not base64!!!"); err == nil {
			t.Fatal("expected error for non-base64 input")
		}
	})
	t.Run("wrong length", func(t *testing.T) {
		short := base64.StdEncoding.EncodeToString([]byte("too short"))
		if _, err := ParseMasterKey(short); err == nil {
			t.Fatal("expected error for wrong-length key")
		}
	})
	t.Run("empty", func(t *testing.T) {
		if _, err := ParseMasterKey(""); err == nil {
			t.Fatal("expected error for empty input")
		}
	})
}
