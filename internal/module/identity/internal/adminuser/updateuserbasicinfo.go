package adminuser

import (
	"context"
	"fmt"
	"time"

	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/auth/usersession"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
)

// UpdateUserBasicInfo applies an administrator's edit of an account: its
// profile columns and, in a billing transaction of its own, its wallet.
func (s *Service) UpdateUserBasicInfo(ctx context.Context, req *dto.UpdateUserBasicInfoRequest) error {
	// The admin edit spans two domains by design — identity profile fields
	// and a billing money adjustment — so it runs as two sequential domain
	// transactions. The identity transaction goes first because it carries
	// the request validations (avatar, demo-mode password): a rejected edit
	// then leaves the money untouched. A failure after the profile commit
	// leaves the money unadjusted for the admin to retry — the same
	// partial-failure surface the flows will have as services.
	accessStateChanged := false
	passwordChanged := false
	err := s.deps.Store.InIdentityTx(ctx, func(store repository.IdentityStore) error {
		userInfo, err := store.User().FindOneForUpdate(ctx, req.UserId)
		if err != nil {
			return xerr.Wrapf(err, xerr.DatabaseQueryError, "find user %d", req.UserId)
		}
		if err := validateAvatarUpdate(userInfo.Avatar, req.Avatar); err != nil {
			return err
		}
		accessStateChanged = userInfo.Enable == nil || *userInfo.Enable != req.Enable
		columns := map[string]any{
			"avatar":              req.Avatar,
			"refer_code":          req.ReferCode,
			"referer_id":          req.RefererId,
			"only_first_purchase": req.OnlyFirstPurchase,
			"referral_percentage": req.ReferralPercentage,
			"enable":              req.Enable,
			"is_admin":            req.IsAdmin,
		}
		if req.Password != "" && req.Password != "***" {
			if userInfo.Id == demoAdminID && demoMode() {
				return demoRestricted("modify the admin user's password")
			}
			for column, value := range password.UserColumns(req.Password) {
				columns[column] = value
			}
			passwordChanged = true
		}
		// Only these profile columns are written: the billing-owned money
		// columns go through the admin's wallet adjustment in its own
		// billing transaction below.
		if err := store.User().UpdateColumns(ctx, userInfo.Id, columns); err != nil {
			return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update user %d", userInfo.Id)
		}
		return nil
	})
	if err != nil {
		// The generic code keeps the validation's own (an invalid avatar,
		// the demo-mode refusal).
		return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update user %d", req.UserId)
	}
	// Account state changes must invalidate both subscription-token caches and
	// node-facing user lists. In particular, disabling a user takes effect at
	// the service plane immediately instead of waiting for the five-minute TTL.
	if accessStateChanged {
		clearUserAccessCaches(ctx, s.deps, []int64{req.UserId})
	}
	// An administrator sets a new password when the old one leaked; the
	// sessions opened with it end too.
	if passwordChanged {
		if err := usersession.Revoke(ctx, s.deps.Redis, req.UserId); err != nil {
			return xerr.Wrapf(err, xerr.ERROR, "revoke sessions of user %d", req.UserId)
		}
	}

	err = s.deps.Store.InBillingTx(ctx, func(store repository.BillingStore) error {
		// Financial adjustments must compare and write the latest values
		// under the wallet lock, with their audit logs in the same
		// transaction.
		walletInfo, err := store.Wallet().FindOneForUpdate(ctx, req.UserId)
		if err != nil {
			return xerr.Wrapf(err, xerr.DatabaseQueryError, "find wallet of user %d", req.UserId)
		}
		if walletInfo.Balance == req.Balance &&
			walletInfo.GiftAmount == req.GiftAmount &&
			walletInfo.Commission == req.Commission {
			return nil
		}
		if walletInfo.Balance != req.Balance {
			content, _ := (&log.Balance{Type: log.BalanceTypeAdjust, Amount: req.Balance - walletInfo.Balance, Balance: req.Balance, Timestamp: timeutil.Now().UnixMilli()}).Marshal()
			if err := store.Log().Insert(ctx, &log.SystemLog{Type: log.TypeBalance.Uint8(), Date: timeutil.Now().Format(time.DateOnly), ObjectID: req.UserId, Content: string(content)}); err != nil {
				return err
			}
		}
		if walletInfo.GiftAmount != req.GiftAmount {
			changeType := log.GiftTypeReduce
			if req.GiftAmount > walletInfo.GiftAmount {
				changeType = log.GiftTypeIncrease
			}
			content, _ := (&log.Gift{Type: changeType, Amount: req.GiftAmount - walletInfo.GiftAmount, Balance: req.GiftAmount, Remark: "Admin adjustment", Timestamp: timeutil.Now().UnixMilli()}).Marshal()
			if err := store.Log().Insert(ctx, &log.SystemLog{Type: log.TypeGift.Uint8(), Date: timeutil.Now().Format(time.DateOnly), ObjectID: req.UserId, Content: string(content)}); err != nil {
				return err
			}
		}
		if walletInfo.Commission != req.Commission {
			content, _ := (&log.Commission{Type: log.CommissionTypeAdjust, Amount: req.Commission - walletInfo.Commission, Timestamp: timeutil.Now().UnixMilli()}).Marshal()
			if err := store.Log().Insert(ctx, &log.SystemLog{Type: log.TypeCommission.Uint8(), Date: timeutil.Now().Format(time.DateOnly), ObjectID: req.UserId, Content: string(content)}); err != nil {
				return err
			}
		}
		walletInfo.Balance = req.Balance
		walletInfo.GiftAmount = req.GiftAmount
		walletInfo.Commission = req.Commission
		if err := store.Wallet().UpdateBalanceFields(ctx, walletInfo); err != nil {
			return err
		}
		return store.Wallet().UpdateCommission(ctx, walletInfo)
	})
	if err != nil {
		return xerr.Wrapf(err, xerr.DatabaseUpdateError, "adjust wallet of user %d", req.UserId)
	}
	return nil
}

// validateAvatarUpdate permits retaining or clearing an existing avatar. A new
// avatar must be a Base64 image no larger than 1024 KiB; OAuth providers may
// persist remote HTTPS avatar URLs, which must remain usable during unrelated
// profile updates.
func validateAvatarUpdate(currentAvatar, requestedAvatar string) error {
	if requestedAvatar == "" || requestedAvatar == currentAvatar {
		return nil
	}

	if !IsValidImageSize(requestedAvatar, 1024) {
		return fmt.Errorf("invalid avatar: %w", xerr.NewErrCode(xerr.InvalidParams))
	}

	return nil
}
