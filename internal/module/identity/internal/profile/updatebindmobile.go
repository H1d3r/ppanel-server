package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/perfect-panel/server/internal/auth/identifier"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/verification"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"gorm.io/gorm"
)

type UpdateBindMobileLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Update Bind Mobile
func newUpdateBindMobileLogic(ctx context.Context, deps Deps) *UpdateBindMobileLogic {
	return &UpdateBindMobileLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateBindMobileLogic) UpdateBindMobile(req *dto.UpdateBindMobileRequest) error {
	if err := l.deps.Policy.EnsureMethodEnabled(l.ctx, identifier.Mobile); err != nil {
		return err
	}
	u, ok := user.FromContext(l.ctx)
	if !ok {
		return fmt.Errorf("no signed-in user: %w", xerr.NewErrCode(xerr.InvalidAccess))
	}
	// verify mobile
	phoneNumber, err := identifier.FormatToE164(req.AreaCode, req.Mobile)
	if err != nil {
		return xerr.Wrapf(err, xerr.TelephoneError, "invalid phone number")
	}
	cacheKey := verification.MobileCodeKey(auth.Register, phoneNumber)
	if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, false); err != nil {
		return xerr.Wrapf(err, xerr.VerifyCodeError, "check verification code")
	}

	m, err := l.deps.UserAuth.FindUserAuthMethodByOpenID(l.ctx, "mobile", phoneNumber)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find identity")
	}
	if m.Id > 0 {
		return fmt.Errorf("the mobile number is bound to an account: %w", xerr.NewErrCode(xerr.UserExist))
	}

	method, err := l.deps.UserAuth.FindUserAuthMethodByUserId(l.ctx, "mobile", u.Id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find identity")
	}
	if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, true); err != nil {
		return xerr.Wrapf(err, xerr.VerifyCodeError, "check verification code")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		method = &user.AuthMethods{
			UserId:         u.Id,
			AuthType:       "mobile",
			AuthIdentifier: phoneNumber,
			Verified:       true,
		}
		if err := l.deps.UserAuth.InsertUserAuthMethods(l.ctx, method); err != nil {
			return xerr.Wrapf(err, xerr.DatabaseInsertError, "bind identity")
		}
	} else {
		method.Verified = true
		method.AuthIdentifier = phoneNumber
		if err := l.deps.UserAuth.UpdateUserAuthMethods(l.ctx, method); err != nil {
			return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update identity")
		}
	}
	return nil
}
