package authmethodadmin

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/infra/sms"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

type TestSmsSendLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Test sms send
func newTestSmsSendLogic(ctx context.Context, deps Deps) *TestSmsSendLogic {
	return &TestSmsSendLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *TestSmsSendLogic) TestSmsSend(req *dto.TestSmsSendRequest) error {
	client, err := sms.NewSender(l.deps.Config().MobilePlatform, l.deps.Config().MobilePlatformConfig)
	if err != nil {
		return xerr.Wrapf(err, xerr.ERROR, "new sms sender")
	}
	err = client.SendCode(req.AreaCode, req.Telephone, "123456")
	if err != nil {
		// The administrator is testing the sender, so the failure itself is
		// the answer.
		return fmt.Errorf("send test sms: %w", xerr.NewErrCodeMsg(xerr.SenderTestFailed, fmt.Sprintf("send sms err: %v", err.Error())))
	}
	return nil
}
