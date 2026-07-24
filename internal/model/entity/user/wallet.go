package user

import "time"

// Wallet is the billing-owned wallet row (ADR-001 step 5). During the
// transition every wallet movement dual-writes this table and the legacy
// money columns on the user row; readers still use the user row until they
// migrate to the wallet view.
type Wallet struct {
	UserId     int64 `gorm:"primaryKey"`
	Balance    int64 `gorm:"not null;default:0;comment:User Balance Amount"`
	GiftAmount int64 `gorm:"not null;default:0;comment:User Gift Amount"`
	Commission int64 `gorm:"not null;default:0;comment:Commission Amount"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (Wallet) TableName() string {
	return "user_wallet"
}
