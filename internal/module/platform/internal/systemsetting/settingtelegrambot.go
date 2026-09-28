package systemsetting

import "context"

// SettingTelegramBot reloads the Telegram bot from its stored settings.
func (s *Service) SettingTelegramBot(_ context.Context) error {
	return s.deps.reinit("telegram")
}
