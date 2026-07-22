package common

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/logic/common"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/result"
)

// GetPrivacyPolicyHandler documents Get Privacy Policy.
//
// @Summary Get Privacy Policy
// @Tags common
// @Produce json
// @Success 200 {object} result.ResponseSuccessBean{data=dto.PrivacyPolicyConfig}
// @Router /v1/common/site/privacy [get]
func GetPrivacyPolicyHandler(svcCtx *svc.ServiceContext) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		l := common.NewGetPrivacyPolicyLogic(ctx, svcCtx)
		resp, err := l.GetPrivacyPolicy()
		result.HttpResult(c, resp, err)
	}
}
