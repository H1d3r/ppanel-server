package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/system"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetPrivacyPolicyConfigHandler documents get Privacy Policy Config.
//
// @Summary get Privacy Policy Config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.PrivacyPolicyConfig}
// @Router /v1/admin/system/privacy [get]
func GetPrivacyPolicyConfigHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		l := system.NewGetPrivacyPolicyConfigLogic(ctx, svcCtx)
		resp, err := l.GetPrivacyPolicyConfig()
		result.HttpResult(c, resp, err)
	}
}
