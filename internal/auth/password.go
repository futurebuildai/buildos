// Package auth owns BuildOS's native identity primitives: argon2id
// password hashing and RS256 JWT minting/verification. BuildOS issues and
// validates its own tokens now — there is no external OIDC provider.
//
// PII / secret handling: cleartext passwords are PII-Restricted secret
// material. They must NEVER be logged, returned in errors, or written to a
// span/metric. Only the argon2id encoded hash (which is safe to store) ever
// leaves this package.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters. These follow the OWASP Password Storage Cheat Sheet
// recommendation for argon2id (m=19456 KiB, t=2, p=1) — a balance of
// resistance and latency suitable for an interactive login path. The values
// are encoded into every hash string, so raising them later does not break
// verification of existing hashes.
const (
	argonTime    = 2         // iterations (t)
	argonMemory  = 19 * 1024 // memory in KiB (~19 MiB)
	argonThreads = 1         // parallelism (p)
	argonKeyLen  = 32        // derived key length in bytes
	argonSaltLen = 16        // random salt length in bytes
)

// ErrPasswordMismatch is returned by VerifyPassword when the password does
// not match the stored hash. It is deliberately uniform and detail-free — a
// login handler should map it to the same generic "invalid credentials"
// response as an unknown email to avoid an account-enumeration oracle.
var ErrPasswordMismatch = errors.New("auth: password does not match")

// ErrInvalidHash is returned when a stored hash string is malformed or uses
// an unsupported algorithm/version. It signals data corruption or a hash
// produced by an incompatible scheme, not a wrong password.
var ErrInvalidHash = errors.New("auth: invalid password hash format")

// HashPassword derives an argon2id hash of the given cleartext password and
// returns it in the standard PHC string format:
//
//	$argon2id$v=19$m=19456,t=2,p=1$<b64salt>$<b64hash>
//
// A fresh CSPRNG salt is generated per call. The returned string is safe to
// store; the cleartext password must never be persisted or logged.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("auth: password must not be empty")
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		b64(salt), b64(hash),
	), nil
}

// VerifyPassword checks a cleartext password against an encoded argon2id hash
// produced by HashPassword. It returns nil on a match, ErrPasswordMismatch on
// a non-match, or ErrInvalidHash if the encoded string is malformed.
//
// The comparison uses subtle.ConstantTimeCompare to avoid leaking timing
// information about how many leading bytes matched.
func VerifyPassword(password, encodedHash string) error {
	params, salt, want, err := decodeHash(encodedHash)
	if err != nil {
		return err
	}
	got := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) == 1 {
		return nil
	}
	return ErrPasswordMismatch
}

// argonParams carries the cost parameters parsed out of an encoded hash so
// VerifyPassword can recompute the derived key with the exact settings the
// hash was created under.
type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

// decodeHash parses a PHC-format argon2id string into its parameters, salt,
// and derived-key bytes. Any structural deviation yields ErrInvalidHash.
func decodeHash(encodedHash string) (argonParams, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	// Expected layout: ["", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash]
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return argonParams{}, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return argonParams{}, nil, nil, ErrInvalidHash
	}

	var p argonParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return argonParams{}, nil, nil, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argonParams{}, nil, nil, ErrInvalidHash
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argonParams{}, nil, nil, ErrInvalidHash
	}
	return p, salt, hash, nil
}
