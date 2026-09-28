package protocolkey

import (
	"crypto/sha1"
	"fmt"
)

// GenerateShortID derives a REALITY short ID from the private key: the first
// eight hex digits of its SHA-1, so the same key keeps the same short ID.
func GenerateShortID(privateKey string) string {
	hash := sha1.New()
	hash.Write([]byte(privateKey))
	hashValue := hash.Sum(nil)
	hashString := fmt.Sprintf("%x", hashValue)
	return hashString[:8]
}
