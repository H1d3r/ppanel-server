package adminuser

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/model/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/tool"
	"github.com/perfect-panel/server/pkg/uuidx"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type CreateUserLogic struct {
	ctx  context.Context
	deps Deps
	logger.Logger
}

func newCreateUserLogic(ctx context.Context, deps Deps) *CreateUserLogic {
	return &CreateUserLogic{
		ctx:    ctx,
		deps:   deps,
		Logger: logger.WithContext(ctx),
	}
}
func (l *CreateUserLogic) CreateUser(req *dto.CreateUserRequest) error {
	if req.ReferCode == "" {
		// timestamp replaces user id
		req.ReferCode = uuidx.UserInviteCode(timeutil.Now().UnixMicro())
	}
	if req.Password == "" {
		req.Password = req.Email
	}
	pwd := tool.EncodePassWord(req.Password)
	newUser := &user.User{
		Password:           pwd,
		Algo:               tool.PasswordAlgoArgon2id,
		ReferralPercentage: req.ReferralPercentage,
		OnlyFirstPurchase:  &req.OnlyFirstPurchase,
		ReferCode:          req.ReferCode,
		IsAdmin:            &req.IsAdmin,
	}
	var ams []user.AuthMethods

	if req.TelephoneAreaCode != "" && req.Telephone != "" {
		phone := fmt.Sprintf("%s-%s", req.TelephoneAreaCode, req.Telephone)
		_, err := l.deps.UserAuths.FindUserAuthMethodByOpenID(l.ctx, "mobile", phone)
		if err == nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.TelephoneExist), "telephone exist")
		}
		ams = append(ams, user.AuthMethods{
			AuthType:       "mobile",
			AuthIdentifier: phone,
		})
	}
	if req.Email != "" {
		_, err := l.deps.UserAuths.FindUserAuthMethodByOpenID(l.ctx, "email", req.Email)
		if err == nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.EmailExist), "email exist")
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find email auth method failed: %v", err.Error())
		}
		ams = append(ams, user.AuthMethods{
			AuthType:       "email",
			AuthIdentifier: req.Email,
		})
	}

	newUser.AuthMethods = ams

	// todo: get product id and duration
	if req.RefererUser != "" {
		// get referer user id
		u, err := l.deps.Users.FindOneByEmail(l.ctx, req.RefererUser)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.Wrapf(xerr.NewErrCode(xerr.UserNotExist), "referer user not found: %v", err.Error())
			}
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find referer user failed: %v", err.Error())
		}
		newUser.RefererId = u.Id
	}

	// Account creation and the initial wallet credit commit atomically; a
	// half-created account would otherwise block the retry on "email
	// exists". The generic transaction is deliberate: this admin flow spans
	// identity (account) and billing (initial money) by design.
	err := l.deps.Store.InTx(l.ctx, func(store repository.Store) error {
		if err := store.User().Insert(l.ctx, newUser); err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "insert user failed: %v", err.Error())
		}
		if req.Balance == 0 && req.Commission == 0 && req.GiftAmount == 0 {
			return nil
		}
		w, err := store.Wallet().FindOneForUpdate(l.ctx, newUser.Id)
		if err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "load new user wallet failed: %v", err.Error())
		}
		w.Balance = req.Balance
		w.GiftAmount = req.GiftAmount
		w.Commission = req.Commission
		if err := store.Wallet().UpdateBalanceFields(l.ctx, w); err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "credit new user wallet failed: %v", err.Error())
		}
		if err := store.Wallet().UpdateCommission(l.ctx, w); err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "credit new user commission failed: %v", err.Error())
		}
		return nil
	})
	return err
}
