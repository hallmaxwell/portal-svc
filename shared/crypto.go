package shared

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// GenerateUUID generates a random UUIDv4.
func GenerateUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// Set version 4 and variant RFC4122
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// GenerateShortID generates a random 16-character hex string (8 bytes).
func GenerateShortID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GenerateX25519KeyPair generates a base64url encoded X25519 private and public key pair.
// Note: Sing-box uses base64url encoding without padding for X25519 keys in its configuration.
func GenerateX25519KeyPair() (privateKey string, publicKey string, err error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}

	pub := priv.PublicKey()

	privB64 := base64.RawURLEncoding.EncodeToString(priv.Bytes())
	pubB64 := base64.RawURLEncoding.EncodeToString(pub.Bytes())

	return privB64, pubB64, nil
}
