package orderLogic

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/tool"
)

// DailyOrderReportLogic pushes the previous day's settlement summary to the
// administrators who bound Telegram. It reports yesterday because it runs
// after midnight, when the day it summarises is complete.
type DailyOrderReportLogic struct {
	svc *svc.ServiceContext
}

func NewDailyOrderReportLogic(svc *svc.ServiceContext) *DailyOrderReportLogic {
	return &DailyOrderReportLogic{svc: svc}
}

func (l *DailyOrderReportLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	log := logger.WithContext(ctx)
	if !l.svc.Config.Telegram.EnableNotify {
		return nil
	}

	date := timeutil.Now().AddDate(0, 0, -1)
	report, err := l.svc.Billing.DailyOrderReport(ctx, date)
	if err != nil {
		return err
	}

	text, err := tool.RenderTemplateToString(notification.AdminOrderDaily, map[string]string{
		"Date":      report.Date.Format(time.DateOnly),
		"Orders":    fmt.Sprintf("%d", report.Orders),
		"Amount":    formatReportAmount(report.Amount),
		"Subscribe": formatReportLines(report.ByPlan),
		"Payment":   formatReportLines(report.ByMethod),
	})
	if err != nil {
		log.Errorw("[DailyOrderReport] render template failed", logger.Field("error", err.Error()))
		return err
	}

	admins, err := l.svc.Store.User().QueryAdminUsers(ctx)
	if err != nil {
		log.Errorw("[DailyOrderReport] query admin users failed", logger.Field("error", err.Error()))
		return err
	}
	bot := l.svc.TelegramBot
	if bot == nil {
		log.Info("[DailyOrderReport] telegram bot is not configured")
		return nil
	}

	sent := 0
	for _, admin := range admins {
		chatID, ok := findTelegram(admin)
		if !ok {
			continue
		}
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ParseMode = "markdown"
		if _, err := bot.Send(msg); err != nil {
			// One unreachable administrator must not stop the others.
			log.Errorw("[DailyOrderReport] send failed",
				logger.Field("error", err.Error()),
				logger.Field("user_id", admin.Id),
			)
			continue
		}
		sent++
	}
	log.Infow("[DailyOrderReport] report delivered",
		logger.Field("date", report.Date.Format(time.DateOnly)),
		logger.Field("orders", report.Orders),
		logger.Field("recipients", sent),
	)
	return nil
}

// formatReportLines renders a breakdown as Markdown list items. An empty
// breakdown still needs a line, otherwise the section reads as broken.
func formatReportLines(lines []billing.DailyOrderReportLine) string {
	if len(lines) == 0 {
		return "· 无"
	}
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		rendered = append(rendered, fmt.Sprintf("· %s：%d 单，%s",
			line.Name, line.Orders, formatReportAmount(line.Amount)))
	}
	return strings.Join(rendered, "\n")
}

// formatReportAmount converts minor units to the display amount.
func formatReportAmount(amount int64) string {
	return fmt.Sprintf("%.2f", float64(amount)/100)
}
