package application

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/application"
	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/result"
)

// CreateSubscribeApplicationHandler documents Create subscribe application.
//
// @Summary Create subscribe application
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.CreateSubscribeApplicationRequest true "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.SubscribeApplication}
// @Router /v1/admin/application/ [post]
func CreateSubscribeApplicationHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.CreateSubscribeApplicationRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}

		l := application.NewCreateSubscribeApplicationLogic(ctx, svcCtx)
		resp, err := l.CreateSubscribeApplication(&req)
		result.HttpResult(c, resp, err)
	}
}
