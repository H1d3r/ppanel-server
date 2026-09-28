package telegram

import (
	"strings"

	"github.com/go-telegram/bot/models"
)

// commandEntity returns the bot_command entity when the message starts with
// one. The bounds check guards against malformed entities in untrusted
// webhook payloads.
func commandEntity(msg *models.Message) (models.MessageEntity, bool) {
	if msg == nil || len(msg.Entities) == 0 {
		return models.MessageEntity{}, false
	}
	entity := msg.Entities[0]
	if entity.Type != models.MessageEntityTypeBotCommand || entity.Offset != 0 ||
		entity.Length <= 0 || entity.Length > len(msg.Text) {
		return models.MessageEntity{}, false
	}
	return entity, true
}

// messageCommand extracts the command name without the leading slash or the
// @botname mention suffix. It returns "" when the message is not a command.
func messageCommand(msg *models.Message) string {
	entity, ok := commandEntity(msg)
	if !ok {
		return ""
	}
	command := msg.Text[1:entity.Length]
	if at := strings.Index(command, "@"); at != -1 {
		command = command[:at]
	}
	return command
}

// commandArguments returns the text following the command, or "" when the
// command has no arguments.
func commandArguments(msg *models.Message) string {
	entity, ok := commandEntity(msg)
	if !ok || entity.Length >= len(msg.Text) {
		return ""
	}
	return msg.Text[entity.Length+1:]
}

// shortcutCommands are the ticket commands the listings offer as
// "/<command>_<id>" shortcuts: a Telegram command cannot contain a space, so
// the id rides in the command name to make it a single tap.
var shortcutCommands = map[string]bool{"tk": true, "rp": true, "close": true, "reopen": true}

// expandShortcut turns the shortcut "/tk_12" into the command "tk" with the
// arguments "12"; arguments after the shortcut ("/rp_12 text") follow the id.
// Anything else is returned unchanged.
func expandShortcut(command, args string) (string, string) {
	name, id, ok := strings.Cut(command, "_")
	if !ok || !shortcutCommands[name] || id == "" || strings.Trim(id, "0123456789") != "" {
		return command, args
	}
	if args == "" {
		return name, id
	}
	return name, id + " " + args
}
