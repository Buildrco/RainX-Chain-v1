package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

func Hash(b []byte) [32]byte  { return sha256.Sum256(b) }
func HashHex(b []byte) string { h := Hash(b); return hex.EncodeToString(h[:]) }

func Sign(priv ed25519.PrivateKey, msg []byte) []byte { return ed25519.Sign(priv, msg) }
func Verify(pub ed25519.PublicKey, msg, sig []byte) bool {
	return ed25519.Verify(pub, msg, sig)
}

// Equal32 avoids accidentally treating encoded hashes as strings with different casing.
func Equal32(a, b []byte) bool { return bytes.Equal(a, b) }

var ErrInvalidSignature = errors.New("invalid signature")
