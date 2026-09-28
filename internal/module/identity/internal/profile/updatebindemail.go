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

type UpdateBindEmailLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewUpdateBindEmailLogic Update Bind Email
func newUpdateBindEmailLogic(ctx context.Context, deps Deps) *UpdateBindEmailLogic {
	return &UpdateBindEmailLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateBindEmailLogic) UpdateBindEmail(req *dto.UpdateBindEmailRequest) error {
	if err := l.deps.Policy.EnsureMethodEnabled(l.ctx, identifier.Email); err != nil {
		return err
	}
	domainList, restrict := l.deps.EmailDomains()
	email, err := identifier.ValidateEmail(req.Email, domainList, restrict)
	if err != nil {
		return xerr.Wrapf(err, xerr.InvalidParams, "invalid email")
	}
	req.Email = email
	// The new address becomes a login identifier, so its owner must prove
	// control of it first, as binding a mobile number does.
	cacheKey := verification.EmailCodeKey(auth.Register, email)
	if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, false); err != nil {
		return xerr.Wrapf(err, xerr.VerifyCodeError, "check verification code")
	}
	u, ok := user.FromContext(l.ctx)
	if !ok {
		return fmt.Errorf("no signed-in user: %w", xerr.NewErrCode(xerr.InvalidAccess))
	}
	method, err := l.deps.UserAuth.FindUserAuthMethodByUserId(l.ctx, "email", u.Id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find identity")
	}
	m, err := l.deps.UserAuth.FindUserAuthMethodByOpenID(l.ctx, "email", req.Email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find identity")
	}
	// email already bind
	if m.Id > 0 {
		return fmt.Errorf("the email is bound to an account: %w", xerr.NewErrCode(xerr.UserExist))
	}
	if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, true); err != nil {
		return xerr.Wrapf(err, xerr.VerifyCodeError, "check verification code")
	}
	if method.Id == 0 {
		method = &user.AuthMethods{
			UserId:         u.Id,
			AuthType:       "email",
			AuthIdentifier: req.Email,
			Verified:       true,
		}
		if err := l.deps.UserAuth.InsertUserAuthMethods(l.ctx, method); err != nil {
			return xerr.Wrapf(err, xerr.DatabaseInsertError, "bind identity")
		}
	} else {
		method.Verified = true
		method.AuthIdentifier = req.Email
		if err := l.deps.UserAuth.UpdateUserAuthMethods(l.ctx, method); err != nil {
			return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update identity")
		}
	}
	return nil
}
