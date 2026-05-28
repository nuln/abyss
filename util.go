package abyss

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/hkdf"
)

// ── Crypto ──────────────────────────────────────────────────────────────────

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func comparePassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func randomToken(n int) (string, error) { //nolint:unparam // Kept configurable for future callers/tests.
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ── MIME ─────────────────────────────────────────────────────────────────────

func detectMIME(filename string, data []byte) string {
	if len(data) > 0 {
		return http.DetectContentType(data)
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return "application/octet-stream"
	}
	if t := mimeByExt(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

func mimeByExt(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".json":
		return "application/json"
	case ".pdf":
		return "application/pdf"
	default:
		return ""
	}
}

// ── Image ────────────────────────────────────────────────────────────────────

func decodeImage(r io.Reader) (image.Image, error) {
	img, err := imaging.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}

func resizeToFit(in io.Reader, width, height int, out io.Writer) error {
	img, err := decodeImage(in)
	if err != nil {
		return err
	}
	resized := imaging.Fit(img, width, height, imaging.Lanczos)
	if err := imaging.Encode(out, resized, imaging.JPEG); err != nil {
		return fmt.Errorf("encode image: %w", err)
	}
	return nil
}

func aesEncrypt(data, key []byte) (encrypted, nonce []byte, err error) {
	key, err = deriveEncryptionKey(key)
	if err != nil {
		return nil, nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	encrypted = gcm.Seal(nil, nonce, data, nil)
	return encrypted, nonce, nil
}

func aesDecrypt(encrypted, nonce, key []byte) ([]byte, error) {
	// Try derived key first (current scheme).
	derived, err := deriveEncryptionKey(key)
	if err == nil {
		if plain, decErr := aesDecryptRaw(encrypted, nonce, derived); decErr == nil {
			return plain, nil
		}
	}
	// Fallback: try the raw key for data encrypted before the HKDF migration.
	return aesDecryptRaw(encrypted, nonce, key)
}

func aesDecryptRaw(encrypted, nonce, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, encrypted, nil)
}

func deriveEncryptionKey(key []byte) ([]byte, error) {
	reader := hkdf.New(sha256.New, key, nil, []byte("abyss/aes-gcm"))
	derived := make([]byte, 32)
	if _, err := io.ReadFull(reader, derived); err != nil {
		return nil, err
	}
	return derived, nil
}
