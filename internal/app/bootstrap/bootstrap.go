package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/perfect-panel/server/internal/app/migration/schema"
	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/orm"
	"github.com/perfect-panel/server/pkg/random"
	"github.com/perfect-panel/server/pkg/xerr"
)

// Dependencies is the startup/reconfiguration boundary. It owns only mutable
// runtime configuration and the services needed to load or publish it.
type Dependencies struct {
	Config                   func() config.Config
	UpdateConfig             func(func(*config.Config))
	Store                    repository.Store
	ExchangeRate             *billing.CurrencyRateCache
	Notification             notification.Service
	SetTelegramBot           func(*tgbot.Bot)
	SetNodeMultiplierManager func(*network.MultiplierManager)
}

func (d *Dependencies) currentConfig() config.Config {
	if d == nil || d.Config == nil {
		return config.Config{}
	}
	return d.Config()
}

func (d *Dependencies) updateConfig(update func(*config.Config)) {
	if d != nil && d.UpdateConfig != nil {
		d.UpdateConfig(update)
	}
}

// Subsystem names a runtime configuration subsystem an administrator can
// reload. The values are the names the admin settings handlers pass through
// their reinitialize callback.
type Subsystem string

const (
	SubsystemSite      Subsystem = "site"
	SubsystemNode      Subsystem = "node"
	SubsystemEmail     Subsystem = "email"
	SubsystemDevice    Subsystem = "device"
	SubsystemInvite    Subsystem = "invite"
	SubsystemVerify    Subsystem = "verify"
	SubsystemSubscribe Subsystem = "subscribe"
	SubsystemRegister  Subsystem = "register"
	SubsystemMobile    Subsystem = "mobile"
	SubsystemCurrency  Subsystem = "currency"
	SubsystemTelegram  Subsystem = "telegram"
)

// ErrUnknownSubsystem is returned by Reload for a name no subsystem answers
// to, so a misspelt reload request fails loudly instead of doing nothing.
var ErrUnknownSubsystem = errors.New("unknown runtime subsystem")

// loaders maps every reloadable subsystem to its loader. Each loader publishes
// its configuration only after everything it needs was read, so a failed load
// leaves the previous configuration in place.
var loaders = map[Subsystem]func(*Dependencies) error{
	SubsystemSite:      Site,
	SubsystemNode:      Node,
	SubsystemEmail:     Email,
	SubsystemDevice:    Device,
	SubsystemInvite:    Invite,
	SubsystemVerify:    Verify,
	SubsystemSubscribe: Subscribe,
	SubsystemRegister:  Register,
	SubsystemMobile:    Mobile,
	SubsystemCurrency:  Currency,
	SubsystemTelegram:  Telegram,
}

// startupOrder is the order Start loads the subsystems in. Node reads the
// secret NodeSecret provisions, so NodeSecret runs right before it.
var startupOrder = []Subsystem{
	SubsystemSite, SubsystemNode, SubsystemEmail, SubsystemDevice, SubsystemInvite, SubsystemVerify,
	SubsystemSubscribe, SubsystemRegister, SubsystemMobile, SubsystemCurrency, SubsystemTelegram,
}

// Start loads startup state in dependency order. Migration and node-secret
// provisioning must precede every node configuration read. The first failure
// stops startup and is returned, so the caller fails fast instead of serving
// with a partially loaded configuration.
func Start(deps *Dependencies) error {
	if err := Migrate(deps); err != nil {
		return err
	}
	WarnDefaultAdminPassword(deps)
	return loadSubsystems(deps, startupOrder)
}

func loadSubsystems(deps *Dependencies, order []Subsystem) error {
	for _, subsystem := range order {
		if subsystem == SubsystemNode {
			if err := NodeSecret(deps); err != nil {
				return wrapf(err, xerr.ERROR, "provision the node secret")
			}
		}
		if err := loaders[subsystem](deps); err != nil {
			return wrapf(err, xerr.ERROR, "load the %s configuration", subsystem)
		}
	}
	return nil
}

// Reload refreshes the subsystem changed by an administrator. Startup-only
// migration and node-secret provisioning are deliberately excluded. A failure
// is logged and returned, and the subsystem keeps its previous configuration.
func Reload(deps *Dependencies, subsystem Subsystem) error {
	load, ok := loaders[subsystem]
	if !ok {
		logger.Errorw("[Reload] unknown subsystem, nothing reloaded", logger.Field("subsystem", string(subsystem)))
		return fmt.Errorf("reload %q: %w", string(subsystem), ErrUnknownSubsystem)
	}
	if err := load(deps); err != nil {
		logger.Errorw("[Reload] reload failed, keeping the previous configuration",
			logger.Field("subsystem", string(subsystem)), logger.Field("error", err.Error()))
		return wrapf(err, xerr.ERROR, "reload the %s configuration", subsystem)
	}
	return nil
}

// wrapf adds context to err with xerr.Wrapf and keeps the cause's text in the
// message: Wrapf prints the cause only of an error that already carries a
// code.
func wrapf(err error, code uint32, format string, args ...any) error {
	var coded *xerr.CodeError
	if err != nil && !errors.As(err, &coded) {
		format += ": %v"
		args = append(args, err)
	}
	return xerr.Wrapf(err, code, format, args...)
}

func Migrate(ctx *Dependencies) error {
	current := ctx.currentConfig()
	mc := orm.Mysql{
		Config: current.DatabaseConfig(),
	}
	now := time.Now()
	if err := schema.Up(mc.Driver(), mc.MigrationDsn()); err != nil {
		if errors.Is(err, schema.NoChange) {
			logger.Info("[Migrate] database not change")
			return nil
		}
		logger.Errorf("[Migrate] Up error: %v", err.Error())
		return wrapf(err, xerr.ERROR, "migrate the database")
	}
	logger.Info("[Migrate] Database change, took " + time.Since(now).String())
	// if not found admin user
	err := ctx.Store.InTx(context.Background(), func(store repository.Store) error {
		count, err := store.User().QueryRegisterUserTotal(context.Background())
		if err != nil {
			return err
		}
		if count == 0 {
			enable := true
			adminPassword := initialAdminPassword(current.Administrator.Email, current.Administrator.Password)
			admin := &user.User{
				Password:  password.EncodePassWord(adminPassword),
				Algo:      password.PasswordAlgoArgon2id,
				IsAdmin:   &enable,
				ReferCode: user.GenerateInviteCode(time.Now().Unix()),
			}
			if err := store.User().Insert(context.Background(), admin); err != nil {
				logger.Errorf("[Migrate] CreateAdminUser error: %v", err.Error())
				return err
			}
			if err := store.UserAuth().InsertUserAuthMethods(context.Background(), &user.AuthMethods{
				UserId:         admin.Id,
				AuthType:       "email",
				AuthIdentifier: current.Administrator.Email,
				Verified:       true,
			}); err != nil {
				logger.Errorf("[Migrate] CreateAdminUser error: %v", err.Error())
				return err
			}
			logger.Info("[Migrate] Create admin user success")
		}
		return nil
	})
	return wrapf(err, xerr.DatabaseInsertError, "seed the first administrator")
}

// defaultAdminPassword is the value older releases seeded the first
// administrator with when none was configured.
const defaultAdminPassword = "password"

// initialAdminPassword returns the configured password for the first
// administrator, or a generated one when none was configured. The documented
// Docker and environment-variable installs configure none, and falling back to
// the published default would open the panel to anyone.
func initialAdminPassword(email, configured string) string {
	if configured != "" {
		return configured
	}
	generated := random.KeyNew(20, 1)
	// Printed once, outside the structured logger's redaction, so the
	// operator can sign in; it is not stored anywhere else.
	log.Printf("[Migrate] Created administrator %s with generated password %s; sign in and change it now", email, generated)
	return generated
}

// WarnDefaultAdminPassword flags administrators that still sign in with the
// password older releases seeded, which anyone can look up.
func WarnDefaultAdminPassword(ctx *Dependencies) {
	admins, err := ctx.Store.User().QueryAdminUsers(context.Background())
	if err != nil {
		logger.Errorf("[Migrate] Query admin users error: %v", err.Error())
		return
	}
	for _, admin := range admins {
		if !password.MultiPasswordVerify(admin.Algo, admin.Salt, defaultAdminPassword, admin.Password) {
			continue
		}
		email := ""
		for _, method := range admin.AuthMethods {
			if method.AuthType == "email" {
				email = method.AuthIdentifier
			}
		}
		logger.Errorw("[Security] An administrator still uses the default password; change it immediately",
			logger.Field("user_id", admin.Id), logger.Field("email", email))
	}
}
