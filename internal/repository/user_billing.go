package repository

import (
	"context"
	"errors"

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

// FindOneForUpdate locks the billing-owned wallet row and composes it with
// a plain read of the identity profile: the money values come from the
// wallet table, everything else from the user row. Lock-ordering contract:
// any flow touching both domains takes the wallet lock before the user row
// (the dual-write's user-column UPDATE follows the same order), so wallet
// movements and profile edits cannot deadlock.
func (m *userBillingRepo) FindOneForUpdate(ctx context.Context, id int64) (*user.User, error) {
	var result *user.User
	err := m.QueryNoCacheCtx(ctx, &result, func(conn *gorm.DB, v interface{}) error {
		var w user.Wallet
		werr := conn.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", id).First(&w).Error
		if werr != nil && !errors.Is(werr, gorm.ErrRecordNotFound) {
			return werr
		}
		var u user.User
		if err := conn.Where("id = ?", id).First(&u).Error; err != nil {
			return err
		}
		if errors.Is(werr, gorm.ErrRecordNotFound) {
			// Account predating the wallet backfill: seed the row from the
			// user columns, then take the lock.
			seed := user.Wallet{UserId: id, Balance: u.Balance, GiftAmount: u.GiftAmount, Commission: u.Commission}
			if err := conn.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
				return err
			}
			if err := conn.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("user_id = ?", id).First(&w).Error; err != nil {
				return err
			}
		}
		u.Balance = w.Balance
		u.GiftAmount = w.GiftAmount
		u.Commission = w.Commission
		result = &u
		return nil
	})
	return result, err
}
