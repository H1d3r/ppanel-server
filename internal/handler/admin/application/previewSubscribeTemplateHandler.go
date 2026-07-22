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

// PreviewSubscribeTemplateHandler documents Preview Template.
//
// @Summary Preview Template
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request query dto.PreviewSubscribeTemplateRequest false "Request parameters"
// @Success 200 {object} result.ResponseSuccessBean{data=dto.PreviewSubscribeTemplateResponse}
// @Router /v1/admin/application/preview [get]
func PreviewSubscribeTemplateHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.PreviewSubscribeTemplateRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			result.ParamErrorResult(c, err)
			return
		}
		validateErr := svcCtx.Validate(&req)
		if validateErr != nil {
			result.ParamErrorResult(c, validateErr)
			return
		}

		l := application.NewPreviewSubscribeTemplateLogic(ctx, svcCtx)
		resp, err := l.PreviewSubscribeTemplate(&req)
		result.HttpResult(c, resp, err)

	}
}
