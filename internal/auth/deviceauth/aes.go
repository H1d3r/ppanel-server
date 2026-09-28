package deviceauth

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/forgoer/openssl"
)

// Encrypt encrypts plainText for the device transport with AES-256-CBC and
// PKCS#7 padding. It returns the base64 ciphertext and the nonce, the current
// time in hexadecimal nanoseconds. The IV is derived from the nonce and the
// key instead of being sent, so the receiver needs the nonce to decrypt; the
// envelope carries it as its time.
func Encrypt(plainText []byte, keyStr string) (string, string, error) {
	nonce := fmt.Sprintf("%x", time.Now().UnixNano())
	key := generateKey(keyStr)
	iv := generateIv(nonce, keyStr)
	dst, err := openssl.AesCBCEncrypt(plainText, key, iv, openssl.PKCS7_PADDING)
	return base64.StdEncoding.EncodeToString(dst), nonce, err
}

// Decrypt reverses Encrypt: cipherText is the base64 ciphertext and ivStr the
// nonce Encrypt returned with it.
func Decrypt(cipherText string, keyStr string, ivStr string) (string, error) {
	decode, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", err
	}
	key := generateKey(keyStr)
	iv := generateIv(ivStr, keyStr)
	dst, err := openssl.AesCBCDecrypt(decode, key, iv, openssl.PKCS7_PADDING)
	return string(dst), err
}

// generateKey hashes key with SHA-256, so a secret of any length yields the
// 32-byte key AES-256 needs.
func generateKey(key string) []byte {
	hash := sha256.Sum256([]byte(key))
	return hash[:32]
}

// generateIv derives the IV from the nonce iv and the key: the SHA-256 of
// hex(MD5(iv)) followed by the key, of which CBC uses the first 16 bytes. The
// device clients derive it the same way, so it must not change.
func generateIv(iv, key string) []byte {
	h := md5.New()
	h.Write([]byte(iv))
	return generateKey(hex.EncodeToString(h.Sum(nil)) + key)
}
