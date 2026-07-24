package authmethodadmin

import (
	"context"

	"github.com/perfect-panel/server/pkg/email"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/pkg/logger"
)

type GetEmailPlatformLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get email support platform
func newGetEmailPlatformLogic(ctx context.Context, deps Deps) *GetEmailPlatformLogic {
	return &GetEmailPlatformLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetEmailPlatformLogic) GetEmailPlatform() (resp *dto.PlatformResponse, err error) {
	return &dto.PlatformResponse{
		List: email.GetSupportedPlatforms(),
	}, nil
}
