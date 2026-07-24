package authMethod

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// UpdateAuthMethodConfigHandler documents Update auth method config.
//
// @Summary Update auth method config
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.UpdateAuthMethodConfigRequest true "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.AuthMethodConfig}
// @Router /v1/admin/auth-method/config [put]
func UpdateAuthMethodConfigHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.UpdateAuthMethodConfigRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}

		resp, err := svcCtx.Identity.UpdateAuthMethodConfig(ctx, &req)
		result.HttpResult(c, resp, err)
	}
}
