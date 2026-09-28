package bootstrap

import (
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/pkg/logger"
)

// currencySettings mirrors the stored currency keys. The seeded Currency key
// has no reader: the unit is what the runtime uses.
type currencySettings struct {
	CurrencyUnit   string
	CurrencySymbol string
	AccessKey      string
}

func Currency(ctx *Dependencies) error {
	// Retrieve system currency configuration
	var configs currencySettings
	if err := readSettings(categoryCurrency, ctx.Store.System().GetCurrencyConfig, &configs); err != nil {
		logger.Errorf("[INIT] Failed to get currency configuration: %v", err.Error())
		return err
	}
	ctx.ExchangeRate.Set(0) // Default exchange rate to 0
	currencyConfig := config.Currency{
		Unit:      configs.CurrencyUnit,
		Symbol:    configs.CurrencySymbol,
		AccessKey: configs.AccessKey,
	}
	ctx.updateConfig(func(current *config.Config) { current.Currency = currencyConfig })
	logger.Info("[INIT] Currency configuration loaded",
		logger.Field("unit", currencyConfig.Unit),
		logger.Field("symbol", currencyConfig.Symbol),
		logger.Field("provider_configured", currencyConfig.AccessKey != ""),
	)
	return nil
}
