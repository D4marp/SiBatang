// Package crypto: token acak, hashing token bearer, dan enkripsi AES-GCM untuk data restricted.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"errors"
)

// RandomToken menghasilkan token opaque 256-bit (base64url tanpa padding).
func RandomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken: SHA-256 cukup untuk token acak berentropi tinggi (bukan password).
func HashToken(t string) []byte {
	h := sha256.Sum256([]byte(t))
	return h[:]
}

func Equal(a, b []byte) bool { return subtle.ConstantTimeCompare(a, b) == 1 }

var refEnc = base32.NewEncoding("ABCDEFGHJKMNPQRSTUVWXYZ23456789a").WithPadding(base32.NoPadding)

// RandomRef menghasilkan kode referensi pendek yang tidak ambigu (bukan kredensial).
func RandomRef(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable")
	}
	s := refEnc.EncodeToString(b)
	out := make([]byte, 0, n)
	for i := 0; i < len(s) && len(out) < n; i++ {
		c := s[i]
		if c == 'a' { // karakter padding alfabet; ganti
			c = 'Z'
		}
		out = append(out, c)
	}
	return string(out)
}

// RandomActivationCode: 10 karakter dari alfabet tanpa karakter ambigu.
func RandomActivationCode() string { return RandomRef(10) }

type Box struct{ aead cipher.AEAD }

func NewBox(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes")
	}
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	return &Box{aead: g}, nil
}

// Seal: nonce || ciphertext. aad mengikat ciphertext ke konteks (mis. tenant).
func (b *Box) Seal(plain, aad []byte) []byte {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic("crypto/rand unavailable")
	}
	return b.aead.Seal(nonce, nonce, plain, aad)
}

func (b *Box) Open(sealed, aad []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return nil, errors.New("ciphertext too short")
	}
	return b.aead.Open(nil, sealed[:n], sealed[n:], aad)
}
