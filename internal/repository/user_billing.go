package repository

import (
	"context"

	"github.com/perfect-panel/server/internal/model/entity/user"
	"gorm.io/gorm"
)

// Billing-domain methods of the user repository: the wallet view onto the
// user row's money columns (data debt: step 5 moves them into their own
// table) and withdrawal records (ADR-001 step 5).

func (m *userRepo) UpdateBalanceFields(ctx context.Context, data *user.User, tx ...*gorm.DB) error {
	return m.ExecCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		return conn.Model(&user.User{}).
			Where("id = ?", data.Id).
			Updates(map[string]interface{}{
				"balance":     data.Balance,
				"gift_amount": data.GiftAmount,
			}).Error
	}, m.getCacheKeys(data)...)
}

// UpdateCommission deliberately updates only the commission balance. Financial
// writers often hold a row lock, but a full Save here could still overwrite an
// unrelated profile change made by another request.
func (m *userRepo) UpdateCommission(ctx context.Context, data *user.User, tx ...*gorm.DB) error {
	return m.ExecCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		return conn.Model(&user.User{}).
			Where("id = ?", data.Id).
			Update("commission", data.Commission).Error
	}, m.getCacheKeys(data)...)
}

// --- withdrawal ---

func (m *userRepo) InsertWithdrawal(ctx context.Context, data *user.Withdrawal, tx ...*gorm.DB) error {
	return m.ExecNoCacheCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		return conn.Create(data).Error
	})
}
