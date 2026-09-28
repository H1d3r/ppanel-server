package usersub

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewToken returns a new subscription token: 128 bits from crypto/rand,
// hex-encoded to the 32 characters stored tokens already have. The token is
// the only credential of the subscription URL, so it must not be derivable
// from anything another party can see, such as an order number or the time
// it was issued.
func NewToken() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(fmt.Errorf("generate subscription token: %w", err))
	}
	return hex.EncodeToString(buf[:])
}
