package common

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/common"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetStatHandler documents Get stat.
//
// @Summary Get stat
// @Tags common
// @Produce json
// @Success 200 {object} result.ResponseSuccessBean{data=dto.GetStatResponse}
// @Router /v1/common/site/stat [get]
func GetStatHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		l := common.NewGetStatLogic(ctx, svcCtx)
		resp, err := l.GetStat()
		result.HttpResult(c, resp, err)
	}
}
