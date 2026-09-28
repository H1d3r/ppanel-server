package wallet

import (
	"context"
	"time"

	walletEntity "github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
)

// FindWallet reads the user's wallet for display; a user without a wallet
// row reads as nil.
func (s *Service) FindWallet(ctx context.Context, userID int64) (*walletEntity.Wallet, error) {
	return s.deps.Store.Wallet().FindWallet(ctx, userID)
}

// FindWallets reads the wallets of the users for display; a user without a
// wallet row is absent from the map.
func (s *Service) FindWallets(ctx context.Context, userIDs []int64) (map[int64]*walletEntity.Wallet, error) {
	return s.deps.Store.Wallet().FindWalletsByUserIds(ctx, userIDs)
}

// OpenWallet sets the opening balance, gift amount and commission of an
// account an administrator created. It runs in a billing transaction of its
// own after the identity transaction that created the account; a failure
// leaves an uncredited account the administrator can adjust.
func (s *Service) OpenWallet(ctx context.Context, opening walletEntity.Wallet) error {
	return s.deps.Store.InBillingTx(ctx, func(store repository.BillingStore) error {
		w, err := store.Wallet().FindOneForUpdate(ctx, opening.UserId)
		if err != nil {
			return xerr.Wrapf(err, xerr.DatabaseQueryError, "load new user wallet")
		}
		w.Balance = opening.Balance
		w.GiftAmount = opening.GiftAmount
		w.Commission = opening.Commission
		if err := store.Wallet().UpdateBalanceFields(ctx, w); err != nil {
			return xerr.Wrapf(err, xerr.DatabaseUpdateError, "credit new user wallet")
		}
		if err := store.Wallet().UpdateCommission(ctx, w); err != nil {
			return xerr.Wrapf(err, xerr.DatabaseUpdateError, "credit new user commission")
		}
		return nil
	})
}

// AdjustWallet applies an administrator's edit of the user's wallet: it sets
// the balance, gift amount and commission to the target's. It runs in a
// billing transaction of its own after the identity transaction of the edit;
// a failure leaves the money unadjusted for the administrator to retry.
func (s *Service) AdjustWallet(ctx context.Context, target walletEntity.Wallet) error {
	return s.deps.Store.InBillingTx(ctx, func(store repository.BillingStore) error {
		// Financial adjustments must compare and write the latest values
		// under the wallet lock, with their audit logs in the same
		// transaction.
		walletInfo, err := store.Wallet().FindOneForUpdate(ctx, target.UserId)
		if err != nil {
			return xerr.Wrapf(err, xerr.DatabaseQueryError, "find wallet of user %d", target.UserId)
		}
		if walletInfo.Balance == target.Balance &&
			walletInfo.GiftAmount == target.GiftAmount &&
			walletInfo.Commission == target.Commission {
			return nil
		}
		if walletInfo.Balance != target.Balance {
			content, _ := (&log.Balance{Type: log.BalanceTypeAdjust, Amount: target.Balance - walletInfo.Balance, Balance: target.Balance, Timestamp: timeutil.Now().UnixMilli()}).Marshal()
			if err := store.Log().Insert(ctx, &log.SystemLog{Type: log.TypeBalance.Uint8(), Date: timeutil.Now().Format(time.DateOnly), ObjectID: target.UserId, Content: string(content)}); err != nil {
				return err
			}
		}
		if walletInfo.GiftAmount != target.GiftAmount {
			changeType := log.GiftTypeReduce
			if target.GiftAmount > walletInfo.GiftAmount {
				changeType = log.GiftTypeIncrease
			}
			content, _ := (&log.Gift{Type: changeType, Amount: target.GiftAmount - walletInfo.GiftAmount, Balance: target.GiftAmount, Remark: "Admin adjustment", Timestamp: timeutil.Now().UnixMilli()}).Marshal()
			if err := store.Log().Insert(ctx, &log.SystemLog{Type: log.TypeGift.Uint8(), Date: timeutil.Now().Format(time.DateOnly), ObjectID: target.UserId, Content: string(content)}); err != nil {
				return err
			}
		}
		if walletInfo.Commission != target.Commission {
			content, _ := (&log.Commission{Type: log.CommissionTypeAdjust, Amount: target.Commission - walletInfo.Commission, Timestamp: timeutil.Now().UnixMilli()}).Marshal()
			if err := store.Log().Insert(ctx, &log.SystemLog{Type: log.TypeCommission.Uint8(), Date: timeutil.Now().Format(time.DateOnly), ObjectID: target.UserId, Content: string(content)}); err != nil {
				return err
			}
		}
		walletInfo.Balance = target.Balance
		walletInfo.GiftAmount = target.GiftAmount
		walletInfo.Commission = target.Commission
		if err := store.Wallet().UpdateBalanceFields(ctx, walletInfo); err != nil {
			return err
		}
		return store.Wallet().UpdateCommission(ctx, walletInfo)
	})
}
