package abyss

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Crypto ────────────────────────────────────────────────────────────────────

func TestHashPassword_Roundtrip(t *testing.T) {
	hash, err := hashPassword("hunter2")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.True(t, comparePassword(hash, "hunter2"))
	assert.False(t, comparePassword(hash, "wrong"))
}

func TestHashPassword_DifferentHash(t *testing.T) {
	h1, _ := hashPassword("secret")
	h2, _ := hashPassword("secret")
	// bcrypt always produces a different hash because of random salt
	assert.NotEqual(t, h1, h2)
}

func TestRandomToken_Length(t *testing.T) {
	tok, err := randomToken(32)
	require.NoError(t, err)
	assert.NotEmpty(t, tok)
	// base64url-encoded 32 bytes → 43 characters (without padding)
	assert.GreaterOrEqual(t, len(tok), 40)
}

func TestRandomToken_Unique(t *testing.T) {
	t1, _ := randomToken(32)
	t2, _ := randomToken(32)
	assert.NotEqual(t, t1, t2)
}

func TestSha256Hex(t *testing.T) {
	h := sha256Hex("hello")
	assert.Len(t, h, 64)
	assert.Equal(t, sha256Hex("hello"), h) // deterministic
	assert.NotEqual(t, sha256Hex("hello"), sha256Hex("world"))
}

// ── MIME ──────────────────────────────────────────────────────────────────────

func TestDetectMIME_BySniff(t *testing.T) {
	// PNG magic bytes
	pngHeader := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 20))
	mime := detectMIME("unknown", pngHeader)
	assert.Contains(t, mime, "png")
}

func TestDetectMIME_ByExtension(t *testing.T) {
	cases := map[string]string{
		"photo.jpg":  "image/jpeg",
		"photo.jpeg": "image/jpeg",
		"icon.png":   "image/png",
		"anim.gif":   "image/gif",
		"img.webp":   "image/webp",
		"doc.pdf":    "application/pdf",
		"data.json":  "application/json",
		"read.txt":   "text/plain; charset=utf-8",
	}
	for name, want := range cases {
		mime := detectMIME(name, nil)
		assert.Equal(t, want, mime, "file: %s", name)
	}
}

func TestDetectMIME_Unknown(t *testing.T) {
	mime := detectMIME("file.unknownxyz", nil)
	assert.Equal(t, "application/octet-stream", mime)
}

// ── Image ─────────────────────────────────────────────────────────────────────

func TestResizeToFit_InvalidReader(t *testing.T) {
	err := resizeToFit(bytes.NewReader([]byte("not an image")), 100, 100, new(bytes.Buffer))
	assert.Error(t, err)
}

func TestDecodeImage_InvalidData(t *testing.T) {
	_, err := decodeImage(bytes.NewReader([]byte("garbage")))
	assert.Error(t, err)
}

// ── AES ──────────────────────────────────────────────────────────────────────

func TestAesEncryptDecrypt_Roundtrip(t *testing.T) {
	key := bytes.Repeat([]byte("k"), 32)
	plain := []byte("secret data")
	enc, nonce, err := aesEncrypt(plain, key)
	require.NoError(t, err)
	dec, err := aesDecrypt(enc, nonce, key)
	require.NoError(t, err)
	assert.Equal(t, plain, dec)
}

func TestAesDecrypt_BackwardCompatWithRawKey(t *testing.T) {
	// Simulate data encrypted with the raw key (pre-HKDF migration).
	key := bytes.Repeat([]byte("k"), 32)
	plain := []byte("legacy secret")

	// Encrypt using raw key directly (old behavior).
	enc, nonce, err := aesEncryptRaw(plain, key)
	require.NoError(t, err)

	// aesDecrypt should fall back to raw key and succeed.
	dec, err := aesDecrypt(enc, nonce, key)
	require.NoError(t, err)
	assert.Equal(t, plain, dec)
}

// aesEncryptRaw encrypts with a raw key (simulating pre-HKDF behavior).
func aesEncryptRaw(data, key []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return gcm.Seal(nil, nonce, data, nil), nonce, nil
}
