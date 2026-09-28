package mail

import (
	"github.com/perfect-panel/server/internal/infra/integration"
)

type Platform int

const (
	SMTP Platform = iota
	unsupported
)

var platforms = integration.NewPlatforms(unsupported, map[string]Platform{
	"smtp": SMTP,
})

func (p Platform) String() string {
	return platforms.Name(p)
}

func parsePlatform(s string) Platform {
	return platforms.Parse(s)
}

func GetSupportedPlatforms() []integration.Info {
	return []integration.Info{
		{
			Platform:    SMTP.String(),
			PlatformURL: "",
			PlatformFieldDescription: map[string]string{
				"host":     "host",
				"port":     "port",
				"user":     "user",
				"pass":     "pass",
				"from":     "from",
				"reply_to": "reply_to",
				"ssl":      "ssl",
			},
		},
	}
}
