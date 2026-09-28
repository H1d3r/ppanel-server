package bootstrap

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/mapping"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

var errAuthMethodMissing = errors.New("no such auth method")

// findAuthMethod reads the stored configuration of one authentication method.
func findAuthMethod(ctx *Dependencies, method string) (*auth.Auth, error) {
	found, err := ctx.Store.Auth().FindOneByMethod(context.Background(), method)
	if err == nil && found == nil {
		err = errAuthMethodMissing
	}
	if err != nil {
		return nil, wrapf(err, xerr.DatabaseQueryError, "read the %s auth method", method)
	}
	return found, nil
}

// authMethodEnabled treats a method without a stored flag as disabled
// instead of dereferencing nil.
func authMethodEnabled(method *auth.Auth) bool {
	return method.Enabled != nil && *method.Enabled
}

// Email get email smtp config
func Email(ctx *Dependencies) error {
	logger.Debug("Email config initialization")
	method, err := findAuthMethod(ctx, "email")
	if err != nil {
		return err
	}
	var cfg config.EmailConfig
	var emailConfig = new(auth.EmailAuthConfig)
	if err := emailConfig.Unmarshal(method.Config); err != nil {
		// The stored config falls back to the defaults on purpose; the
		// load goes on, but the operator has to see the rejected value.
		logger.Errorw("[Email] stored auth method config is invalid, using the defaults",
			logger.Field("method", "email"), logger.Field("error", err.Error()))
	}
	mapping.DeepCopy(&cfg, emailConfig)
	cfg.Enable = authMethodEnabled(method)
	value, err := json.Marshal(emailConfig.PlatformConfig)
	if err != nil {
		return wrapf(err, xerr.ERROR, "encode the email platform config")
	}
	cfg.PlatformConfig = string(value)
	ctx.updateConfig(func(current *config.Config) { current.Email = cfg })
	return nil
}

func Mobile(ctx *Dependencies) error {
	logger.Debug("Mobile config initialization")
	method, err := findAuthMethod(ctx, "mobile")
	if err != nil {
		return err
	}
	var cfg config.MobileConfig
	var mobileConfig auth.MobileAuthConfig
	if err := mobileConfig.Unmarshal(method.Config); err != nil {
		// The stored config falls back to the defaults on purpose; the
		// load goes on, but the operator has to see the rejected value.
		logger.Errorw("[Mobile] stored auth method config is invalid, using the defaults",
			logger.Field("method", "mobile"), logger.Field("error", err.Error()))
	}
	mapping.DeepCopy(&cfg, mobileConfig)
	cfg.Enable = authMethodEnabled(method)
	value, err := json.Marshal(mobileConfig.PlatformConfig)
	if err != nil {
		return wrapf(err, xerr.ERROR, "encode the mobile platform config")
	}
	cfg.PlatformConfig = string(value)
	ctx.updateConfig(func(current *config.Config) { current.Mobile = cfg })
	return nil
}

// Device loads the device login settings. A stored configuration that does
// not decode fails the load: applying the zero value instead would silently
// switch device request signing off.
func Device(ctx *Dependencies) error {
	logger.Debug("device config initialization")
	method, err := findAuthMethod(ctx, "device")
	if err != nil {
		return err
	}
	var cfg config.DeviceConfig
	var deviceConfig auth.DeviceConfig
	if err := deviceConfig.Unmarshal(method.Config); err != nil {
		return wrapf(err, xerr.ERROR, "decode the device auth method config")
	}
	mapping.DeepCopy(&cfg, deviceConfig)
	cfg.Enable = authMethodEnabled(method)
	ctx.updateConfig(func(current *config.Config) { current.Device = cfg })
	return nil
}
