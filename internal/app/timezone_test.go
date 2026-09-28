package app

import (
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/pkg/logger/logtest"
	"github.com/perfect-panel/server/pkg/orm"
)

func TestWarnDatabaseTimeZone(t *testing.T) {
	for _, tc := range []struct {
		name     string
		location string
		params   string
		warns    bool
	}{
		{"defaults match the default location", "Asia/Shanghai", "", false},
		{"defaults differ from another location", "Europe/Paris", "", true},
		{"explicit parameters match", "Europe/Paris", "charset=utf8mb4&parseTime=true&loc=Europe%2FParis", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := logtest.NewCollector(t)
			var c config.Config
			c.AppLocation = tc.location
			c.SetDatabaseConfig(orm.Config{Driver: orm.DriverMySQL, Config: tc.params})
			warnDatabaseTimeZone(c)
			if got := strings.Contains(logs.String(), "database time zone differs"); got != tc.warns {
				t.Fatalf("warned = %v, want %v: %s", got, tc.warns, logs.String())
			}
		})
	}
}
