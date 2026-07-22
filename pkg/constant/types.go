package constant

import (
	"crypto/sha256"
	"encoding/hex"
)

import "encoding/json"

// Used for type cloning conversion
const (
	Int64   int64  = 0
	Uint32  uint32 = 0
	DevMode        = "dev"
)

// VerifyType is the type of verification code
type VerifyType uint8

const (
	Register VerifyType = iota + 1
	Security
)

func ParseVerifyType(i uint8) VerifyType {
	return VerifyType(i)
}

func (v VerifyType) String() string {
	switch v {
	case Register:
		return "register"
	case Security:
		return "security"
	default:
		return "unknown"
	}
}

// TempOrderCacheKey Cache to Redis Key
// eg: temp_order:order_no
const TempOrderCacheKey = "temp_order:%s"

// CheckoutTokenHash returns the durable representation of the guest checkout
// capability. Only the caller receives the original high-entropy token.
func CheckoutTokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

type TemporaryOrderInfo struct {
	OrderNo       string `json:"order_no"`
	CheckoutToken string `json:"checkout_token"`
	Identifier    string `json:"identifier"`
	AuthType      string `json:"auth_type"`
	// PasswordHash is the one-way encoded password needed to finish a guest
	// account after payment.  Never place the plaintext password in Redis.
	PasswordHash string `json:"password_hash,omitempty"`
	// Password is retained only to finish orders created by older releases.
	// New orders never populate it and it must never be logged.
	Password   string `json:"password,omitempty"`
	InviteCode string `json:"invite_code,omitempty"`
}

func (t *TemporaryOrderInfo) Unmarshal(data []byte) error {
	type Alias TemporaryOrderInfo
	aux := (*Alias)(t)
	return json.Unmarshal(data, aux)
}

func (t *TemporaryOrderInfo) Marshal() ([]byte, error) {
	type Alias TemporaryOrderInfo
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(t),
	})
}
