package serverapi

import (
	"context"
	"errors"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/internal/model/entity/node"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

type ServerPushStatusLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewServerPushStatusLogic Push server status
func newServerPushStatusLogic(ctx context.Context, deps Deps) *ServerPushStatusLogic {
	return &ServerPushStatusLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *ServerPushStatusLogic) ServerPushStatus(req *dto.ServerPushStatusRequest) error {
	// Find server info
	serverInfo, err := l.deps.Store.Node().FindOneServer(l.ctx, req.ServerId)
	if err != nil || serverInfo.Id <= 0 {
		l.Errorw("[PushOnlineUsers] FindOne error", logger.Field("error", err))
		return errors.New("server not found")
	}
	err = l.deps.Store.Node().UpdateStatusCache(l.ctx, req.ServerId, &node.Status{
		Cpu:       req.Cpu,
		Mem:       req.Mem,
		Disk:      req.Disk,
		UpdatedAt: req.UpdatedAt,
	})
	if err != nil {
		l.Errorw("[ServerPushStatus] UpdateNodeStatus error", logger.Field("error", err))
		return errors.New("update node status failed")
	}
	now := timeutil.Now()
	serverInfo.LastReportedAt = &now

	err = l.deps.Store.Node().UpdateServer(l.ctx, serverInfo)
	if err != nil {
		l.Errorw("[ServerPushStatus] UpdateServer error", logger.Field("error", err))
		return nil
	}

	return nil
}
