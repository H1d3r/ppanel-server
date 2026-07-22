package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/admin/user"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// CurrentUserHandler documents Current user.
//
// @Summary Current user
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} result.ResponseSuccessBean{data=dto.User}
// @Router /v1/admin/user/current [get]
func CurrentUserHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		l := user.NewCurrentUserLogic(ctx, svcCtx)
		resp, err := l.CurrentUser()
		result.HttpResult(c, resp, err)
	}
}
