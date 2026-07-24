package adminuser

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/model/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/tool"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateUserBasicInfoLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewUpdateUserBasicInfoLogic Update user basic info
func newUpdateUserBasicInfoLogic(ctx context.Context, deps Deps) *UpdateUserBasicInfoLogic {
	return &UpdateUserBasicInfoLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateUserBasicInfoLogic) UpdateUserBasicInfo(req *dto.UpdateUserBasiceInfoRequest) error {
	isDemo := strings.ToLower(os.Getenv("PPANEL_MODE")) == "demo"
	// The generic transaction is a documented transitional exception: the
	// admin edit updates identity profile fields and wallet columns (billing
	// money movement) on the same user row until the wallet table splits out
	// (ADR-001 step 5).
	err := l.deps.Store.InTx(l.ctx, func(store repository.Store) error {
		// Financial adjustments must compare and write the latest values
		// under a lock, with their audit logs in the same transaction.
		// Lock-ordering contract (ADR-001 step 5): the wallet lock comes
		// before the user-row lock, matching every wallet movement's
		// wallet-then-user write order.
		walletInfo, err := store.Wallet().FindOneForUpdate(l.ctx, req.UserId)
		if err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Find User Wallet Error")
		}
		userInfo, err := store.User().FindOneForUpdate(l.ctx, req.UserId)
		if err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Find User Error")
		}
		// The wallet values are authoritative from the wallet row.
		userInfo.Balance = walletInfo.Balance
		userInfo.GiftAmount = walletInfo.GiftAmount
		userInfo.Commission = walletInfo.Commission
		if err := validateAvatarUpdate(userInfo.Avatar, req.Avatar); err != nil {
			return err
		}
		if userInfo.Balance != req.Balance {
			content, _ := (&log.Balance{Type: log.BalanceTypeAdjust, Amount: req.Balance - userInfo.Balance, Balance: req.Balance, Timestamp: timeutil.Now().UnixMilli()}).Marshal()
			if err := store.Log().Insert(l.ctx, &log.SystemLog{Type: log.TypeBalance.Uint8(), Date: timeutil.Now().Format(time.DateOnly), ObjectID: userInfo.Id, Content: string(content)}); err != nil {
				return err
			}
		}
		if userInfo.GiftAmount != req.GiftAmount {
			changeType := log.GiftTypeReduce
			if req.GiftAmount > userInfo.GiftAmount {
				changeType = log.GiftTypeIncrease
			}
			content, _ := (&log.Gift{Type: changeType, Amount: req.GiftAmount - userInfo.GiftAmount, Balance: req.GiftAmount, Remark: "Admin adjustment", Timestamp: timeutil.Now().UnixMilli()}).Marshal()
			if err := store.Log().Insert(l.ctx, &log.SystemLog{Type: log.TypeGift.Uint8(), Date: timeutil.Now().Format(time.DateOnly), ObjectID: userInfo.Id, Content: string(content)}); err != nil {
				return err
			}
		}
		if userInfo.Commission != req.Commission {
			content, _ := (&log.Commission{Type: log.CommissionTypeAdjust, Amount: req.Commission - userInfo.Commission, Timestamp: timeutil.Now().UnixMilli()}).Marshal()
			if err := store.Log().Insert(l.ctx, &log.SystemLog{Type: log.TypeCommission.Uint8(), Date: timeutil.Now().Format(time.DateOnly), ObjectID: userInfo.Id, Content: string(content)}); err != nil {
				return err
			}
		}

		walletChanged := userInfo.Balance != req.Balance ||
			userInfo.GiftAmount != req.GiftAmount ||
			userInfo.Commission != req.Commission
		userInfo.Balance = req.Balance
		userInfo.GiftAmount = req.GiftAmount
		userInfo.Commission = req.Commission
		userInfo.Avatar = req.Avatar
		userInfo.ReferCode = req.ReferCode
		userInfo.RefererId = req.RefererId
		userInfo.OnlyFirstPurchase = &req.OnlyFirstPurchase
		userInfo.ReferralPercentage = req.ReferralPercentage
		userInfo.Enable = &req.Enable
		userInfo.IsAdmin = &req.IsAdmin
		if req.Password != "" && req.Password != "***" {
			if userInfo.Id == 2 && isDemo {
				return errors.Wrapf(xerr.NewErrCodeMsg(503, "Demo mode does not allow modification of the admin user password"), "UpdateUserBasicInfo failed: cannot update admin user password in demo mode")
			}
			userInfo.Password = tool.EncodePassWord(req.Password)
			userInfo.Algo = tool.PasswordAlgoArgon2id
			userInfo.Salt = ""
		}
		if err := store.User().Update(l.ctx, userInfo); err != nil {
			return err
		}
		// The profile save skips the billing-owned money columns; the
		// admin's wallet adjustment goes through the wallet view under the
		// same row lock (their audit logs are recorded above).
		if walletChanged {
			if err := store.Wallet().UpdateBalanceFields(l.ctx, userInfo); err != nil {
				return err
			}
			if err := store.Wallet().UpdateCommission(l.ctx, userInfo); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		l.Errorw("[UpdateUserBasicInfoLogic] Update User Error:", logger.Field("err", err.Error()), logger.Field("userId", req.UserId))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "Update User Error")
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

	if !tool.IsValidImageSize(requestedAvatar, 1024) {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "Invalid avatar")
	}

	return nil
}
