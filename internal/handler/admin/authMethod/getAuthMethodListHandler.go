package authMethod

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetAuthMethodListHandler documents Get auth method list.
//
// @Summary Get auth method list
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.GetAuthMethodListResponse}
// @Router /v1/admin/auth-method/list [get]
func GetAuthMethodListHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := svcCtx.Identity.GetAuthMethodList(ctx)
		result.HttpResult(c, resp, err)
	}
}
