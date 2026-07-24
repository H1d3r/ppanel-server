package repository

import (
	"context"
	"errors"

	"github.com/perfect-panel/server/internal/model/entity/user"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Billing-domain methods of the user repository family: the wallet table and
// withdrawal records (ADR-001 step 5).

// FindOneForUpdate locks the billing-owned wallet row, creating it on first
// use so every account has a wallet once money moves.
func (m *userBillingRepo) FindOneForUpdate(ctx context.Context, userId int64) (*user.Wallet, error) {
	var result *user.Wallet
	err := m.QueryNoCacheCtx(ctx, &result, func(conn *gorm.DB, v interface{}) error {
		var w user.Wallet
		err := conn.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", userId).First(&w).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			seed := user.Wallet{UserId: userId}
			if err := conn.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
				return err
			}
			err = conn.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("user_id = ?", userId).First(&w).Error
		}
		if err != nil {
			return err
		}
		result = &w
		return nil
	})
	return result, err
}

// FindWallet reads the wallet row without locking. A nil result (no error)
// means the account has no wallet row yet; callers treat it as zero values.
func (m *userBillingRepo) FindWallet(ctx context.Context, userId int64) (*user.Wallet, error) {
	var w *user.Wallet
	err := m.QueryNoCacheCtx(ctx, &w, func(conn *gorm.DB, v interface{}) error {
		var row user.Wallet
		err := conn.Where("user_id = ?", userId).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		w = &row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return w, nil
}

// FindWalletsByUserIds batch-reads wallet rows for display lists; missing
// rows are absent from the map (treat as zero values).
func (m *userBillingRepo) FindWalletsByUserIds(ctx context.Context, userIds []int64) (map[int64]*user.Wallet, error) {
	result := make(map[int64]*user.Wallet, len(userIds))
	if len(userIds) == 0 {
		return result, nil
	}
	var rows []*user.Wallet
	err := m.QueryNoCacheCtx(ctx, &rows, func(conn *gorm.DB, v interface{}) error {
		return conn.Where("user_id IN ?", userIds).Find(v).Error
	})
	if err != nil {
		return nil, err
	}
	for _, w := range rows {
		result[w.UserId] = w
	}
	return result, nil
}

// UpdateBalanceFields persists the balance and gift columns of a wallet row
// previously locked by FindOneForUpdate.
func (m *userBillingRepo) UpdateBalanceFields(ctx context.Context, data *user.Wallet, tx ...*gorm.DB) error {
	return m.ExecNoCacheCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		return conn.Model(&user.Wallet{}).
			Where("user_id = ?", data.UserId).
			Updates(map[string]interface{}{
				"balance":     data.Balance,
				"gift_amount": data.GiftAmount,
			}).Error
	})
}

// UpdateCommission persists only the commission column: balance movements
// and commission credits may race on different flows.
func (m *userBillingRepo) UpdateCommission(ctx context.Context, data *user.Wallet, tx ...*gorm.DB) error {
	return m.ExecNoCacheCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		return conn.Model(&user.Wallet{}).
			Where("user_id = ?", data.UserId).
			Update("commission", data.Commission).Error
	})
}

// --- withdrawal ---

func (m *userBillingRepo) InsertWithdrawal(ctx context.Context, data *user.Withdrawal, tx ...*gorm.DB) error {
	return m.ExecNoCacheCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		return conn.Create(data).Error
	})
}
