package serverapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"uuid"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/slicesx"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type GetServerUserListLogic struct {
	logger.Logger
	ctx      context.Context
	deps     Deps
	request  RequestMeta
	response ResponseMeta
}

// NewGetServerUserListLogic Get user list
func newGetServerUserListLogic(ctx context.Context, deps Deps, request RequestMeta) *GetServerUserListLogic {
	return &GetServerUserListLogic{
		Logger:   logger.WithContext(ctx),
		ctx:      ctx,
		deps:     deps,
		request:  request,
		response: NewResponseMeta(),
	}
}

func (l *GetServerUserListLogic) ResponseMeta() ResponseMeta {
	return l.response
}

// placeholderServerUserID is the id of the stand-in user an empty list
// carries. It is also a valid subscription id.
const placeholderServerUserID int64 = 1

// The placeholder is generated only while rebuilding an empty user list.
// Cache hits reuse the serialized UUID and ETag until the list expires/changes.
func placeholderServerUser() dto.ServerUser {
	return dto.ServerUser{
		Id:   placeholderServerUserID,
		UUID: uuid.NewV7().String(),
	}
}

func serverUserListCacheKey(serverID int64, protocol string) string {
	return fmt.Sprintf("%s%d:%s", node.ServerUserListCacheKey, serverID, protocol)
}

func (l *GetServerUserListLogic) queryMatchedSubscribes(nodeIds []int64, nodeTags []string) ([]*subscribe.Subscribe, error) {
	return l.deps.Store.Subscribe().FindByNodeScope(l.ctx, nodeIds, nodeTags)
}

func (l *GetServerUserListLogic) GetServerUserList(req *dto.GetServerUserListRequest) (resp *dto.GetServerUserListResponse, err error) {
	cacheKey := serverUserListCacheKey(req.ServerId, req.Protocol)
	cache, err := l.deps.Redis.Get(l.ctx, cacheKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		// An unreadable cache must not cost the node its user list: fall
		// through to the database like a miss.
		l.Errorw("[GetServerUserList] read cached user list failed",
			logger.Field("server_id", req.ServerId),
			logger.Field("protocol", req.Protocol),
			logger.Field("error", err.Error()))
		cache = ""
	}
	if cache != "" {
		etag := httpx.GenerateETag([]byte(cache))
		resp = &dto.GetServerUserListResponse{}
		//  Check If-None-Match header
		if match := l.request.IfNoneMatch; match == etag {
			return nil, xerr.StatusNotModified
		}
		l.response.SetHeader("ETag", etag)
		err = json.Unmarshal([]byte(cache), resp)
		if err != nil {
			l.Errorw("[ServerUserListCacheKey] json unmarshal error", logger.Field("error", err.Error()))
			return nil, err
		}
		return resp, nil
	}
	list, err := l.rebuildUserList(req.ServerId, req.Protocol)
	if err != nil {
		return nil, err
	}
	etag := httpx.GenerateETag(list.payload)
	l.response.SetHeader("ETag", etag)
	//  Check If-None-Match header
	if match := l.request.IfNoneMatch; match == etag {
		return nil, xerr.StatusNotModified
	}
	return list.resp, nil
}

// userList is a rebuilt node user list with its cached serialization.
type userList struct {
	resp    *dto.GetServerUserListResponse
	payload []byte
	// served is false when no subscription is served and the list only
	// carries the placeholder user.
	served bool
}

// rebuildUserList resolves the server protocol's user list from the database
// and caches it for the next reader.
func (l *GetServerUserListLogic) rebuildUserList(serverID int64, protocol string) (*userList, error) {
	generation, err := l.deps.Store.Node().ServerCacheGeneration(l.ctx, serverID)
	if err != nil {
		return nil, err
	}
	server, err := l.deps.Store.Node().FindOneServer(l.ctx, serverID)
	if err != nil {
		return nil, err
	}
	users, err := l.servedUsers(server, protocol)
	if err != nil {
		return nil, err
	}
	list := &userList{served: len(users) > 0}
	if !list.served {
		users = []dto.ServerUser{placeholderServerUser()}
	}
	list.resp = &dto.GetServerUserListResponse{Users: users}
	list.payload, _ = json.Marshal(list.resp)
	if err := l.deps.Store.Node().SetServerCache(l.ctx, serverID, serverUserListCacheKey(serverID, protocol), string(list.payload), generation); err != nil {
		l.Errorw("[ServerUserListCacheKey] cache set error", logger.Field("error", err.Error()))
	}
	return list, nil
}

// servedUsers resolves the users the server serves over the protocol: the
// servable subscriptions of the plans scoped to its nodes, owned by enabled
// accounts.
func (l *GetServerUserListLogic) servedUsers(server *node.Server, protocol string) ([]dto.ServerUser, error) {
	nodes, err := l.deps.Store.Node().ListNodes(l.ctx, &node.FilterNodeParams{
		ServerId: []int64{server.Id},
		Protocol: protocol,
	})
	if err != nil {
		l.Errorw("FilterNodeList error", logger.Field("error", err.Error()))
		return nil, err
	}
	var nodeTag []string
	var nodeIds []int64
	for _, n := range nodes {
		nodeIds = append(nodeIds, n.Id)
		if n.Tags != "" {
			nodeTag = append(nodeTag, strings.Split(n.Tags, ",")...)
		}
	}

	subs, err := l.queryMatchedSubscribes(nodeIds, nodeTag)
	if err != nil {
		l.Errorw("QuerySubscribeIdsByServerIdAndServerGroupId error", logger.Field("error", err.Error()))
		return nil, err
	}
	if len(subs) == 0 {
		return nil, nil
	}
	type candidate struct {
		userSub *usersub.Subscribe
		plan    *subscribe.Subscribe
	}
	planIDs := make([]int64, 0, len(subs))
	plansByID := make(map[int64]*subscribe.Subscribe, len(subs))
	for _, sub := range subs {
		planIDs = append(planIDs, sub.Id)
		plansByID[sub.Id] = sub
	}
	// A read endpoint: it selects the servable subscriptions and writes
	// nothing (legacy Pending rows are servable as they are).
	data, err := l.deps.Store.UserSubscription().FindUsersSubscribeBySubscribeIds(l.ctx, planIDs)
	if err != nil {
		return nil, err
	}
	candidates := make([]candidate, 0, len(data))
	for _, datum := range data {
		if plan := plansByID[datum.SubscribeId]; plan != nil {
			candidates = append(candidates, candidate{userSub: datum, plan: plan})
		}
	}
	userIDs := make([]int64, 0, len(candidates))
	for _, item := range candidates {
		userIDs = append(userIDs, item.userSub.UserId)
	}
	enabledIDs, err := l.deps.Store.User().FindEnabledUserIDs(l.ctx, slicesx.RemoveDuplicateElements(userIDs...))
	if err != nil {
		return nil, err
	}
	enabled := make(map[int64]struct{}, len(enabledIDs))
	for _, id := range enabledIDs {
		enabled[id] = struct{}{}
	}
	users := make([]dto.ServerUser, 0, len(candidates))
	for _, item := range candidates {
		if _, ok := enabled[item.userSub.UserId]; !ok {
			continue
		}
		users = append(users, dto.ServerUser{
			Id: item.userSub.Id, UUID: item.userSub.UUID,
			SpeedLimit: item.plan.SpeedLimit, DeviceLimit: item.plan.DeviceLimit,
		})
	}
	return users, nil
}

// servedSubscriptionIDs returns the ids of the subscriptions the server
// serves over the protocol: the users GET /v1/server/user hands the node,
// read from that endpoint's cache. A miss rebuilds and caches the list as the
// endpoint does, so checking a traffic report costs no per-entry lookups.
func (l *GetServerUserListLogic) servedSubscriptionIDs(serverID int64, protocol string) (map[int64]struct{}, error) {
	var users []dto.ServerUser
	cached, err := l.deps.Redis.Get(l.ctx, serverUserListCacheKey(serverID, protocol)).Result()
	switch {
	case err != nil && !errors.Is(err, redis.Nil):
		return nil, err
	case cached == "":
		list, err := l.rebuildUserList(serverID, protocol)
		if err != nil {
			return nil, err
		}
		if list.served {
			users = list.resp.Users
		}
	default:
		var resp dto.GetServerUserListResponse
		if err := json.Unmarshal([]byte(cached), &resp); err != nil {
			return nil, err
		}
		if users, err = l.withoutPlaceholder(resp.Users); err != nil {
			return nil, err
		}
	}
	ids := make(map[int64]struct{}, len(users))
	for _, user := range users {
		ids[user.Id] = struct{}{}
	}
	return ids, nil
}

// withoutPlaceholder drops the stand-in user of a cached empty list. Its id
// is a valid subscription id, so a lone entry with that id is kept only when
// its UUID is that subscription's UUID.
func (l *GetServerUserListLogic) withoutPlaceholder(users []dto.ServerUser) ([]dto.ServerUser, error) {
	if len(users) != 1 || users[0].Id != placeholderServerUserID {
		return users, nil
	}
	sub, err := l.deps.Store.UserSubscription().FindOneSubscribe(l.ctx, placeholderServerUserID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if sub.UUID != users[0].UUID {
		return nil, nil
	}
	return users, nil
}
