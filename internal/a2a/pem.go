package a2a

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// decodePEM decodes the first PEM block from the given data.
func decodePEM(data []byte) (*pem.Block, []byte) {
	return pem.Decode(data)
}

// parsePrivateKeyDER parses a DER-encoded private key.
// Supports PKCS#1 (RSA PRIVATE KEY) and PKCS#8 (PRIVATE KEY).
func parsePrivateKeyDER(der []byte) (crypto.PrivateKey, error) {
	// Try PKCS#8 first (more common in modern tooling)
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return key, nil
	}

	// Fall back to PKCS#1 (legacy RSA format)
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}

	return nil, fmt.Errorf("failed to parse private key as PKCS#8 or PKCS#1")
}
