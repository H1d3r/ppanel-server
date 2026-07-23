package common

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/auth/registerpolicy"
	"github.com/perfect-panel/server/internal/logic/common"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// SendSmsCodeHandler documents Get sms verification code.
//
// @Summary Get sms verification code
// @Tags common
// @Accept json
// @Produce json
// @Param request body dto.SendSmsCodeRequest true "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.SendCodeResponse}
// @Router /v1/common/send_sms_code [post]
func SendSmsCodeHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.SendSmsCodeRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}

		l := common.NewSendSmsCodeLogic(ctx, common.SendSmsCodeDependencies{
			Store: svcCtx.Store,
			Redis: svcCtx.Redis,
			Queue: svcCtx.Queue,
			Config: common.SmsCodeConfig{
				VerifyCodeInterval: svcCtx.Config.VerifyCode.Interval,
				VerifyCodeLimit:    svcCtx.Config.VerifyCode.Limit,
				VerifyCodeExpire:   svcCtx.Config.VerifyCode.ExpireTime,
			},
			Policy: registerpolicy.NewServicePolicy(svcCtx),
		})
		resp, err := l.SendSmsCode(&req)
		result.HttpResult(c, resp, err)
	}
}
