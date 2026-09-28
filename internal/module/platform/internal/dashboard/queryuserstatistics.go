package dashboard

import (
	"context"
	"time"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

const consoleUserStatisticsCacheKey = "console:user_statistics"
const consoleUserStatisticsCacheTTL = 60 * time.Second

// QueryUserStatistics counts registrations and paying users for today, this
// month (with its daily breakdown) and all time (with the last months'
// breakdown). Every figure is optional: one whose source fails is logged and
// left at zero. The summary is cached for a minute.
func (s *Service) QueryUserStatistics(ctx context.Context) (*dto.UserStatisticsResponse, error) {
	if demoMode() {
		return mockUserStatistics(), nil
	}
	if cached, ok := readSnapshot[dto.UserStatisticsResponse](ctx, s.deps.Cache, consoleUserStatisticsCacheKey); ok {
		return cached, nil
	}

	resp := &dto.UserStatisticsResponse{}
	now := timeutil.Now()
	failed := func(source string, err error) bool {
		if err == nil {
			return false
		}
		logger.WithContext(ctx).Errorw("[QueryUserStatistics] "+source, logger.Field("error", err.Error()))
		return true
	}

	if count, err := s.deps.Users.QueryRegisterUserTotalByDate(ctx, now); !failed("registrations today", err) {
		resp.Today.Register = count
	}
	if newUsers, renewals, err := s.deps.Orders.QueryDateUserCounts(ctx, now); !failed("paying users today", err) {
		resp.Today.NewOrderUsers, resp.Today.RenewalOrderUsers = newUsers, renewals
	}
	if count, err := s.deps.Users.QueryRegisterUserTotalByMonthly(ctx, now); !failed("registrations this month", err) {
		resp.Monthly.Register = count
	}
	if newUsers, renewals, err := s.deps.Orders.QueryMonthlyUserCounts(ctx, now); !failed("paying users this month", err) {
		resp.Monthly.NewOrderUsers, resp.Monthly.RenewalOrderUsers = newUsers, renewals
	}
	if days, err := s.deps.Users.QueryDailyUserStatisticsList(ctx, now); !failed("daily user breakdown", err) {
		resp.Monthly.List = userBreakdown(days)
	}
	if count, err := s.deps.Users.QueryRegisterUserTotal(ctx); !failed("registrations", err) {
		resp.All.Register = count
	}
	if newUsers, renewals, err := s.deps.Orders.QueryTotalUserCounts(ctx); !failed("paying users", err) {
		resp.All.NewOrderUsers, resp.All.RenewalOrderUsers = newUsers, renewals
	}
	if months, err := s.deps.Users.QueryMonthlyUserStatisticsList(ctx, now); !failed("monthly user breakdown", err) {
		resp.All.List = userBreakdown(months)
	}

	storeSnapshot(ctx, s.deps.Cache, consoleUserStatisticsCacheKey, resp, consoleUserStatisticsCacheTTL)
	return resp, nil
}

func userBreakdown(periods []user.UserStatisticsWithDate) []dto.UserStatistics {
	list := make([]dto.UserStatistics, len(periods))
	for i, period := range periods {
		list[i] = dto.UserStatistics{
			Date:              period.Date,
			Register:          period.Register,
			NewOrderUsers:     period.NewOrderUsers,
			RenewalOrderUsers: period.RenewalOrderUsers,
		}
	}
	return list
}

func mockUserStatistics() *dto.UserStatisticsResponse {
	now := timeutil.Now()

	// Generate daily user statistics for the current month (from 1st to current date)
	monthlyList := make([]dto.UserStatistics, 7)
	for i := 0; i < 7; i++ {
		dayDate := now.AddDate(0, 0, -(6 - i))
		baseRegister := int64(18 + ((6 - i) * 3) + ((6-i)%3)*8)
		monthlyList[i] = dto.UserStatistics{
			Date:              dayDate.Format("2006-01-02"),
			Register:          baseRegister,
			NewOrderUsers:     int64(float64(baseRegister) * 0.65),
			RenewalOrderUsers: int64(float64(baseRegister) * 0.35),
		}
	}

	// Generate monthly user statistics for the past 6 months (oldest first)
	allList := make([]dto.UserStatistics, 6)
	for i := 0; i < 6; i++ {
		monthDate := now.AddDate(0, -(5 - i), 0)
		baseRegister := int64(1800 + ((5 - i) * 200) + ((5-i)%2)*500)
		allList[i] = dto.UserStatistics{
			Date:              monthDate.Format("2006-01"),
			Register:          baseRegister,
			NewOrderUsers:     int64(float64(baseRegister) * 0.65),
			RenewalOrderUsers: int64(float64(baseRegister) * 0.35),
		}
	}

	return &dto.UserStatisticsResponse{
		Today: dto.UserStatistics{
			Register:          28,
			NewOrderUsers:     18,
			RenewalOrderUsers: 10,
		},
		Monthly: dto.UserStatistics{
			Register:          888,
			NewOrderUsers:     588,
			RenewalOrderUsers: 300,
			List:              monthlyList,
		},
		All: dto.UserStatistics{
			Register:          18888,
			NewOrderUsers:     0, // This field is not used in All statistics
			RenewalOrderUsers: 0, // This field is not used in All statistics
			List:              allList,
		},
	}
}
