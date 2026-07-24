package repository

import (
	"context"

	"github.com/perfect-panel/server/internal/model/entity/user"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Billing-domain methods of the user repository: the wallet view onto the
// user row's money columns (data debt: step 5 moves them into their own
// table) and withdrawal records (ADR-001 step 5).

func (m *userBillingRepo) UpdateBalanceFields(ctx context.Context, data *user.User, tx ...*gorm.DB) error {
	return m.ExecCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		if err := conn.Model(&user.User{}).
			Where("id = ?", data.Id).
			Updates(map[string]interface{}{
				"balance":     data.Balance,
				"gift_amount": data.GiftAmount,
			}).Error; err != nil {
			return err
		}
		return upsertWallet(conn, data, "balance", "gift_amount")
	}, data.GetCacheKeys()...)
}

// UpdateCommission deliberately updates only the commission balance. Financial
// writers often hold a row lock, but a full Save here could still overwrite an
// unrelated profile change made by another request.
func (m *userBillingRepo) UpdateCommission(ctx context.Context, data *user.User, tx ...*gorm.DB) error {
	return m.ExecCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		if err := conn.Model(&user.User{}).
			Where("id = ?", data.Id).
			Update("commission", data.Commission).Error; err != nil {
			return err
		}
		return upsertWallet(conn, data, "commission")
	}, data.GetCacheKeys()...)
}

// upsertWallet is the transitional dual-write onto the billing-owned wallet
// table (ADR-001 step 5): the legacy user-row columns above stay the read
// model until readers migrate to the wallet view.
func upsertWallet(conn *gorm.DB, data *user.User, columns ...string) error {
	return conn.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns(columns),
	}).Create(&user.Wallet{
		UserId:     data.Id,
		Balance:    data.Balance,
		GiftAmount: data.GiftAmount,
		Commission: data.Commission,
	}).Error
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

// FindOneForUpdate locks the user row for a wallet movement. The money
// columns still live on the identity-owned user table (recorded data debt),
// so the lock goes through the identity repo until they move out.
func (m *userBillingRepo) FindOneForUpdate(ctx context.Context, id int64) (*user.User, error) {
	return m.users.FindOneForUpdate(ctx, id)
}
