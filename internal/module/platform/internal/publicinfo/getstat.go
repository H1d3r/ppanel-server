package publicinfo

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oschwald/geoip2-golang"
	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/redis/go-redis/v9"
)

const (
	statCacheTTL = time.Hour
	// statRefreshTimeout bounds one refresh of the statistics, which runs
	// on behalf of every caller waiting for it rather than of one request.
	statRefreshTimeout = 30 * time.Second
	// statLookupTimeout bounds the DNS lookup of one node hostname.
	statLookupTimeout = 5 * time.Second
	statLookupWorkers = 8
)

// GetStat returns the public site statistics: the enabled users (rounded
// down), the enabled nodes, the number of countries the nodes are in and the
// protocols they offer. The statistics are cached for an hour. Concurrent
// cache misses share one refresh, and a caller may stop waiting for it
// without cancelling it for the others.
func (s *Service) GetStat(ctx context.Context) (*dto.GetStatResponse, error) {
	if cached := s.cachedStat(ctx); cached != nil {
		return cached, nil
	}
	refresh := s.statRefresh.DoChan(config.CommonStatCacheKey, func() (any, error) {
		refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statRefreshTimeout)
		defer cancel()
		// A refresh that finished while this one was being scheduled has
		// already done the work.
		if cached := s.cachedStat(refreshCtx); cached != nil {
			return cached, nil
		}
		return s.refreshStat(refreshCtx)
	})
	select {
	case result := <-refresh:
		if result.Err != nil {
			return nil, result.Err
		}
		// Every waiting caller gets its own copy of the shared result.
		stat := *result.Val.(*dto.GetStatResponse)
		stat.Protocol = slices.Clone(stat.Protocol)
		return &stat, nil
	case <-ctx.Done():
		return nil, xerr.Wrapf(ctx.Err(), xerr.ERROR, "wait for the site statistics: %v", ctx.Err())
	}
}

func (s *Service) cachedStat(ctx context.Context) *dto.GetStatResponse {
	data, err := s.deps.Redis.Get(ctx, config.CommonStatCacheKey).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) && ctx.Err() == nil {
			logger.WithContext(ctx).Errorw("[GetStat] read the cached statistics", logger.Field("error", err.Error()))
		}
		return nil
	}
	var cached dto.GetStatResponse
	if json.Unmarshal([]byte(data), &cached) != nil {
		return nil
	}
	return &cached
}

func (s *Service) refreshStat(ctx context.Context) (*dto.GetStatResponse, error) {
	nodes := s.deps.Store.Node()
	users, err := s.deps.Store.User().CountEnabledUsers(ctx)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "count enabled users: %v", err)
	}
	nodeCount, err := nodes.CountEnabledNodes(ctx)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "count enabled nodes: %v", err)
	}
	addresses, err := nodes.QueryServerAddresses(ctx)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "list node addresses: %v", err)
	}
	protocols, err := nodes.QueryEnabledNodeProtocols(ctx)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "list node protocols: %v", err)
	}

	stat := &dto.GetStatResponse{
		User:     roundUserCount(users),
		Node:     nodeCount,
		Country:  int64(s.countCountries(ctx, addresses)),
		Protocol: distinctProtocols(protocols),
	}
	data, err := json.Marshal(stat)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.ERROR, "encode the site statistics: %v", err)
	}
	if err := s.deps.Redis.Set(ctx, config.CommonStatCacheKey, string(data), statCacheTTL).Err(); err != nil {
		logger.WithContext(ctx).Errorw("[GetStat] cache the statistics", logger.Field("error", err.Error()))
	}
	return stat, nil
}

// roundUserCount publishes the user count rounded down to a multiple of 100
// above 100 users and of 10 above 10; smaller sites show 1.
func roundUserCount(users int64) int64 {
	switch {
	case users > 100:
		return users - users%100
	case users > 10:
		return users - users%10
	default:
		return 1
	}
}

func distinctProtocols(protocols []string) []string {
	var distinct []string
	for _, protocol := range protocols {
		if protocol != "" && !slices.Contains(distinct, protocol) {
			distinct = append(distinct, protocol)
		}
	}
	slices.Sort(distinct)
	return distinct
}

// countCountries counts the countries the local GeoIP database places the
// node addresses in. An address that does not resolve, or that the database
// has no country for, is left out.
func (s *Service) countCountries(ctx context.Context, addresses []string) int {
	if len(addresses) == 0 {
		return 0
	}
	var db *geoip2.Reader
	if s.deps.GeoIP != nil {
		db = s.deps.GeoIP()
	}
	if db == nil {
		logger.WithContext(ctx).Infow("[GetStat] no GeoIP database: the node countries are not counted")
		return 0
	}
	countries := map[string]struct{}{}
	failed := 0
	for _, ip := range s.resolve(ctx, addresses) {
		record, err := db.Country(ip)
		if err != nil {
			failed++
			continue
		}
		if code := record.Country.IsoCode; code != "" {
			countries[code] = struct{}{}
		}
	}
	if failed > 0 {
		logger.WithContext(ctx).Errorw("[GetStat] locate node addresses",
			logger.Field("failed", failed), logger.Field("addresses", len(addresses)))
	}
	return len(countries)
}

// resolve returns the IPs of the node addresses, looking hostnames up with
// bounded concurrency; the addresses that do not resolve are left out.
func (s *Service) resolve(ctx context.Context, addresses []string) []net.IP {
	resolver := s.deps.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	ips := make([]net.IP, len(addresses))
	slots := make(chan struct{}, statLookupWorkers)
	var (
		wg     sync.WaitGroup
		failed atomic.Int64
	)
	for i, address := range addresses {
		if ip := net.ParseIP(address); ip != nil {
			ips[i] = ip
			continue
		}
		wg.Go(func() {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				failed.Add(1)
				return
			}
			lookupCtx, cancel := context.WithTimeout(ctx, statLookupTimeout)
			defer cancel()
			resolved, err := resolver.LookupIPAddr(lookupCtx, address)
			if err != nil || len(resolved) == 0 {
				failed.Add(1)
				return
			}
			ips[i] = resolved[0].IP
		})
	}
	wg.Wait()
	if n := failed.Load(); n > 0 {
		logger.WithContext(ctx).Errorw("[GetStat] resolve node hostnames", logger.Field("failed", n))
	}
	return slices.DeleteFunc(ips, func(ip net.IP) bool { return ip == nil })
}
