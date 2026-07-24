package user

import "time"

// Wallet is the billing-owned wallet row (ADR-001 step 5): the single
// source of truth for balance, gift and commission since the legacy user
// columns dropped (migration 02144).
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
