package authMethod

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/authMethod"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetSmsPlatformHandler documents Get sms support platform.
//
// @Summary Get sms support platform
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.PlatformResponse}
// @Router /v1/admin/auth-method/sms_platform [get]
func GetSmsPlatformHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		l := authMethod.NewGetSmsPlatformLogic(ctx, svcCtx)
		resp, err := l.GetSmsPlatform()
		result.HttpResult(c, resp, err)
	}
}
