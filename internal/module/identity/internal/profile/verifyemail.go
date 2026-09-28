package profile

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/auth/identifier"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/verification"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

type VerifyEmailLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Verify Email
func newVerifyEmailLogic(ctx context.Context, deps Deps) *VerifyEmailLogic {
	return &VerifyEmailLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *VerifyEmailLogic) VerifyEmail(req *dto.VerifyEmailRequest) error {
	if err := l.deps.Policy.EnsureMethodEnabled(l.ctx, identifier.Email); err != nil {
		return err
	}
	domainList, restrict := l.deps.EmailDomains()
	email, err := identifier.ValidateEmail(req.Email, domainList, restrict)
	if err != nil {
		return xerr.Wrapf(err, xerr.InvalidParams, "invalid email")
	}
	cacheKey := verification.EmailCodeKey(auth.Security, email)
	if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, false); err != nil {
		return xerr.Wrapf(err, xerr.VerifyCodeError, "check verification code")
	}

	u, ok := user.FromContext(l.ctx)
	if !ok {
		return fmt.Errorf("no signed-in user: %w", xerr.NewErrCode(xerr.InvalidAccess))
	}
	method, err := l.deps.UserAuth.FindUserAuthMethodByOpenID(l.ctx, identifier.Email, email)
	if err != nil {
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find identity")
	}
	if method.UserId != u.Id {
		return fmt.Errorf("the email belongs to another account: %w", xerr.NewErrCode(xerr.InvalidAccess))
	}
	if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, true); err != nil {
		return xerr.Wrapf(err, xerr.VerifyCodeError, "check verification code")
	}
	method.Verified = true
	err = l.deps.UserAuth.UpdateUserAuthMethods(l.ctx, method)
	if err != nil {
		return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update identity")
	}
	return nil
}
