package serverapi

import (
	"context"
	"errors"
	"fmt"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/pkg/logger"
)

// PushOnlineUsers records the users a server reports online over a protocol,
// with their IPs, for the server and in the global online count.
func (s *Service) PushOnlineUsers(ctx context.Context, req *dto.OnlineUsersRequest) error {
	// A report names its server and at least one user, each with a
	// subscription id and an IP.
	if req.ServerId <= 0 || len(req.Users) == 0 {
		return errors.New("invalid request parameters")
	}
	for _, user := range req.Users {
		if user.SID <= 0 || user.IP == "" {
			return fmt.Errorf("invalid user data: uid=%d, ip=%s", user.SID, user.IP)
		}
	}

	log := logger.WithContext(ctx)
	if _, err := s.deps.Servers.FindOneServer(ctx, req.ServerId); err != nil {
		log.Errorw("[PushOnlineUsers] FindOne error", logger.Field("error", err))
		return fmt.Errorf("server not found: %w", err)
	}

	onlineUsers := make(node.OnlineUserSubscribe)
	for _, user := range req.Users {
		onlineUsers[user.SID] = append(onlineUsers[user.SID], user.IP)
	}
	if err := s.deps.Online.UpdateOnlineUserSubscribe(ctx, req.ServerId, req.Protocol, onlineUsers); err != nil {
		log.Errorw("[PushOnlineUsers] cache operation error", logger.Field("error", err))
		return err
	}
	if err := s.deps.Online.UpdateOnlineUserSubscribeGlobal(ctx, onlineUsers); err != nil {
		log.Errorw("[PushOnlineUsers] cache operation error", logger.Field("error", err))
		return err
	}
	return nil
}
