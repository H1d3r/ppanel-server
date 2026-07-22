package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/system"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetCurrencyConfigHandler documents Get Currency Config.
//
// @Summary Get Currency Config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.CurrencyConfig}
// @Router /v1/admin/system/currency_config [get]
func GetCurrencyConfigHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		l := system.NewGetCurrencyConfigLogic(ctx, svcCtx)
		resp, err := l.GetCurrencyConfig()
		result.HttpResult(c, resp, err)
	}
}
