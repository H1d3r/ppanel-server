package authmethodadmin

import (
	"context"

	"github.com/perfect-panel/server/pkg/sms"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/pkg/logger"
)

type GetSmsPlatformLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get sms support platform
func newGetSmsPlatformLogic(ctx context.Context, deps Deps) *GetSmsPlatformLogic {
	return &GetSmsPlatformLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetSmsPlatformLogic) GetSmsPlatform() (resp *dto.PlatformResponse, err error) {
	return &dto.PlatformResponse{
		List: sms.GetSupportedPlatforms(),
	}, nil
}
