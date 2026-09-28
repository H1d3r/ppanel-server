package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/pkg/httpx"
)

// LogoutHandler documents Logout.
//
// @Summary Logout
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean
// @Router /v1/public/user/logout [post]
func LogoutHandler(service identity.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		err := service.Logout(c)
		httpx.HttpResult(ctx, nil, err)
	}
}
