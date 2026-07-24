package common

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// SendEmailCodeHandler documents Get verification code.
//
// @Summary Get verification code
// @Tags common
// @Accept json
// @Produce json
// @Param request body dto.SendCodeRequest true "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.SendCodeResponse}
// @Router /v1/common/send_code [post]
func SendEmailCodeHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.SendCodeRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}

		resp, err := svcCtx.Identity.SendEmailCode(ctx, &req)
		result.HttpResult(c, resp, err)
	}
}
