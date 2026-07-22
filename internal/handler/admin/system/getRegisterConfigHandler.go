package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/system"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetRegisterConfigHandler documents Get register config.
//
// @Summary Get register config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.RegisterConfig}
// @Router /v1/admin/system/register_config [get]
func GetRegisterConfigHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		l := system.NewGetRegisterConfigLogic(ctx, svcCtx)
		resp, err := l.GetRegisterConfig()
		result.HttpResult(c, resp, err)
	}
}
