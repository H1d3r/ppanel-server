package serverapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// scopeStore backs the user-list rebuild: one server with one vless node,
// one plan on it and two subscriptions, only one of whose owners is enabled.
type scopeStore struct {
	repository.Store
	redis    *redis.Client
	server   *node.Server
	lookups  int
	cacheSet string
	// placeholderOwner is the stored subscription #1, if any.
	placeholderOwner *usersub.Subscribe
}

func (s *scopeStore) Node() repository.NodeRepo                         { return scopeNodes{s: s} }
func (s *scopeStore) Subscribe() repository.SubscribeRepo               { return scopePlans{s: s} }
func (s *scopeStore) UserSubscription() repository.UserSubscriptionRepo { return scopeSubs{s: s} }
func (s *scopeStore) User() repository.UserRepo                         { return scopeUsers{s: s} }

type scopeNodes struct {
	repository.NodeRepo
	s *scopeStore
}

func (r scopeNodes) FindOneServer(context.Context, int64) (*node.Server, error) {
	server := *r.s.server
	return &server, nil
}
func (r scopeNodes) ServerCacheGeneration(context.Context, int64) (int64, error) { return 0, nil }
func (r scopeNodes) ListNodes(_ context.Context, params *node.FilterNodeParams) ([]*node.Node, error) {
	r.s.lookups++
	if params.Protocol != "vless" {
		return nil, nil
	}
	return []*node.Node{{Id: 11, ServerId: r.s.server.Id, Protocol: "vless"}}, nil
}
func (r scopeNodes) SetServerCache(ctx context.Context, _ int64, key string, value interface{}, _ int64) error {
	r.s.cacheSet = key
	return r.s.redis.Set(ctx, key, value, time.Minute).Err()
}

type scopePlans struct {
	repository.SubscribeRepo
	s *scopeStore
}

func (r scopePlans) FindByNodeScope(_ context.Context, nodeIDs []int64, _ []string) ([]*subscribe.Subscribe, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	return []*subscribe.Subscribe{{Id: 7}}, nil
}

type scopeSubs struct {
	repository.UserSubscriptionRepo
	s *scopeStore
}

func (r scopeSubs) ActivatePendingSubscribesBySubscribeIds(context.Context, []int64) error {
	return nil
}
func (r scopeSubs) FindUsersSubscribeBySubscribeIds(context.Context, []int64) ([]*usersub.Subscribe, error) {
	return []*usersub.Subscribe{
		{Id: 21, UserId: 100, SubscribeId: 7, UUID: "uuid-21"},
		{Id: 22, UserId: 101, SubscribeId: 7, UUID: "uuid-22"},
	}, nil
}
func (r scopeSubs) FindOneSubscribe(_ context.Context, id int64) (*usersub.Subscribe, error) {
	if r.s.placeholderOwner == nil || r.s.placeholderOwner.Id != id {
		return nil, gorm.ErrRecordNotFound
	}
	sub := *r.s.placeholderOwner
	return &sub, nil
}

type scopeUsers struct {
	repository.UserRepo
	s *scopeStore
}

func (r scopeUsers) FindEnabledUserIDs(context.Context, []int64) ([]int64, error) {
	return []int64{100}, nil
}

func newScopeDeps(t *testing.T) (Deps, *scopeStore, *redis.Client) {
	t.Helper()
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := &scopeStore{redis: client, server: &node.Server{Id: 4, Protocols: `[{"type":"vless","ratio":1}]`}}
	return Deps{Store: store, Redis: client, Config: func() Snapshot { return Snapshot{} }}, store, client
}

func servedIDs(t *testing.T, deps Deps) string {
	t.Helper()
	ids, err := newGetServerUserListLogic(context.Background(), deps, RequestMeta{}).servedSubscriptionIDs(4, "vless")
	if err != nil {
		t.Fatalf("servedSubscriptionIDs: %v", err)
	}
	list := make([]string, 0, len(ids))
	for _, id := range []int64{1, 21, 22, 99} {
		if _, ok := ids[id]; ok {
			list = append(list, fmt.Sprint(id))
		}
	}
	if len(list) != len(ids) {
		t.Fatalf("unexpected served ids: %v", ids)
	}
	return strings.Join(list, ",")
}

// The served scope is the node's own user list: a miss rebuilds and caches
// it like GET /v1/server/user, and later reports read the cache.
func TestServedSubscriptionIDsSharesTheUserListCache(t *testing.T) {
	deps, store, client := newScopeDeps(t)
	if got := servedIDs(t, deps); got != "21" {
		t.Fatalf("served = %q, want 21 (disabled owner excluded)", got)
	}
	if store.cacheSet != serverUserListCacheKey(4, "vless") || store.lookups != 1 {
		t.Fatalf("rebuild did not populate the user-list cache: key=%q lookups=%d", store.cacheSet, store.lookups)
	}
	// The node's own pull now hits the list the report rebuilt.
	resp, err := newGetServerUserListLogic(context.Background(), deps, RequestMeta{}).GetServerUserList(&dto.GetServerUserListRequest{ServerCommon: dto.ServerCommon{ServerId: 4, Protocol: "vless"}})
	if err != nil || len(resp.Users) != 1 || resp.Users[0].Id != 21 {
		t.Fatalf("user list = %+v, %v", resp, err)
	}
	if got := servedIDs(t, deps); got != "21" || store.lookups != 1 {
		t.Fatalf("cached scope = %q after %d lookups, want 21 from the cache", got, store.lookups)
	}

	// A protocol without nodes rebuilds to the placeholder, which serves
	// nobody, and caches it for the node.
	got, err := newGetServerUserListLogic(context.Background(), deps, RequestMeta{}).servedSubscriptionIDs(4, "trojan")
	if err != nil || len(got) != 0 {
		t.Fatalf("placeholder list served %v, %v", got, err)
	}
	if cached, err := client.Get(context.Background(), serverUserListCacheKey(4, "trojan")).Result(); err != nil || !strings.Contains(cached, `"id":1`) {
		t.Fatalf("placeholder list not cached: %q, %v", cached, err)
	}
}

// The placeholder shares its id with subscription #1: a cached lone entry
// with that id counts only when it carries that subscription's UUID.
func TestServedSubscriptionIDsIgnoresCachedPlaceholder(t *testing.T) {
	deps, store, client := newScopeDeps(t)
	cache := func(users ...dto.ServerUser) {
		payload, err := json.Marshal(dto.GetServerUserListResponse{Users: users})
		if err != nil {
			t.Fatal(err)
		}
		if err := client.Set(context.Background(), serverUserListCacheKey(4, "vless"), payload, time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
	}
	placeholder := placeholderServerUser()
	cache(placeholder)
	if got := servedIDs(t, deps); got != "" {
		t.Fatalf("placeholder without subscription #1 served %q", got)
	}
	store.placeholderOwner = &usersub.Subscribe{Id: 1, UUID: "uuid-1"}
	if got := servedIDs(t, deps); got != "" {
		t.Fatalf("placeholder served subscription #1: %q", got)
	}
	cache(dto.ServerUser{Id: 1, UUID: "uuid-1"})
	if got := servedIDs(t, deps); got != "1" {
		t.Fatalf("real subscription #1 served = %q, want 1", got)
	}
	cache(dto.ServerUser{Id: 1, UUID: "uuid-1"}, dto.ServerUser{Id: 22, UUID: "uuid-22"})
	if got := servedIDs(t, deps); got != "1,22" {
		t.Fatalf("served = %q, want 1,22", got)
	}
	if store.lookups != 0 {
		t.Fatalf("cached scope rebuilt the list %d times", store.lookups)
	}
}

// The push endpoint only bills subscriptions the reporting server serves.
func TestServerPushUserTrafficDropsSubscriptionsTheServerDoesNotServe(t *testing.T) {
	deps, _, client := newScopeDeps(t)
	err := newServerPushUserTrafficLogic(context.Background(), deps).ServerPushUserTraffic(&dto.ServerPushUserTrafficRequest{
		ServerCommon: dto.ServerCommon{ServerId: 4, Protocol: "vless"},
		Traffic: []dto.UserTraffic{
			{SID: 21, Upload: 5, Download: 7},
			{SID: 22, Upload: 1 << 30, Download: 1 << 30},
			{SID: 99, Upload: 1 << 30},
		},
	})
	if err != nil {
		t.Fatalf("ServerPushUserTraffic: %v", err)
	}
	buckets, err := client.Keys(context.Background(), "traffic:agg:2*").Result()
	if err != nil || len(buckets) != 1 {
		t.Fatalf("traffic buckets = %v, %v", buckets, err)
	}
	fields, err := client.HGetAll(context.Background(), buckets[0]).Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields["4|21|u"] != "5" || fields["4|21|d"] != "7" {
		t.Fatalf("bucket = %v, want only subscription 21", fields)
	}
}
