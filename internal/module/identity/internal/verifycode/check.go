package verifycode

import (
	"context"
	"fmt"
	"github.com/perfect-panel/server/internal/verification"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/pkg/authmethod"
	"github.com/perfect-panel/server/pkg/constant"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/phone"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type CheckVerificationCodeLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Check verification code
func newCheckVerificationCodeLogic(ctx context.Context, deps Deps) *CheckVerificationCodeLogic {
	return &CheckVerificationCodeLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *CheckVerificationCodeLogic) CheckVerificationCode(req *dto.CheckVerificationCodeRequest) (resp *dto.CheckVerificationCodeRespone, err error) {
	resp = &dto.CheckVerificationCodeRespone{}
	if req.Method == authmethod.Email {
		email, validationErr := authmethod.ValidateEmail(
			req.Account,
			l.deps.Config().DomainSuffixList,
			constant.ParseVerifyType(req.Type) == constant.Register && l.deps.Config().EnableDomainSuffix,
		)
		if validationErr != nil {
			return resp, nil
		}
		cacheKey := fmt.Sprintf("%s:%s:%s", config.AuthCodeCacheKey, constant.ParseVerifyType(req.Type), email)
		if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, false); err != nil {
			if errors.Is(err, verification.ErrVerificationAttemptsExceeded) {
				return nil, errors.Wrap(xerr.NewErrCode(xerr.TooManyRequests), "verification attempts exceeded")
			}
			return resp, nil
		}
		resp.Status = true
	}
	if req.Method == authmethod.Mobile {
		if !phone.CheckPhone(req.Account) {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.TelephoneError), "Invalid phone number")
		}
		cacheKey := fmt.Sprintf("%s:%s:+%s", config.AuthCodeTelephoneCacheKey, constant.ParseVerifyType(req.Type), req.Account)
		if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, false); err != nil {
			if errors.Is(err, verification.ErrVerificationAttemptsExceeded) {
				return nil, errors.Wrap(xerr.NewErrCode(xerr.TooManyRequests), "verification attempts exceeded")
			}
			return resp, nil
		}
		resp.Status = true
	}
	return resp, nil
}
