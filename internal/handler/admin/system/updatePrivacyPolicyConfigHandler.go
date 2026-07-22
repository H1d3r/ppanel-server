package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/system"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// UpdatePrivacyPolicyConfigHandler documents Update Privacy Policy Config.
//
// @Summary Update Privacy Policy Config
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.PrivacyPolicyConfig true "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean
// @Router /v1/admin/system/privacy [put]
func UpdatePrivacyPolicyConfigHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.PrivacyPolicyConfig
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}

		l := system.NewUpdatePrivacyPolicyConfigLogic(ctx, svcCtx)
		err := l.UpdatePrivacyPolicyConfig(&req)
		result.HttpResult(c, nil, err)
	}
}
