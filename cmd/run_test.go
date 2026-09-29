package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/pkg/orm"
	"gopkg.in/yaml.v3"
)

// The environment-variable installation completes the default configuration
// file with the connections PPANEL_DB and PPANEL_REDIS name and writes it
// back. The database session zone follows the file's AppLocation, and every
// boot setting the file already had survives the rewrite.
func TestInitConfigInstallsFromTheEnvironment(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("etc", 0o755); err != nil {
		t.Fatal(err)
	}
	content := "AppLocation: Europe/Paris\n" +
		"Transport:\n  Driver: hertz\n" +
		"TLS:\n  Enable: true\n  CertFile: /etc/ppanel/tls.crt\n  KeyFile: /etc/ppanel/tls.key\n" +
		"EdgeSubscribe:\n  Enabled: true\n  MaxClockSkewSeconds: 60\n  Keys:\n    - ID: worker\n      Secret: worker-secret\n"
	if err := os.WriteFile(filepath.Join("etc", "ppanel.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PPANEL_DB", "root:db-secret@tcp(127.0.0.1:3306)/ppanel")
	t.Setenv("PPANEL_REDIS", "redis://:redis-secret@127.0.0.1:6379/2")

	var c config.Config
	if initConfig(&c) {
		t.Fatal("initConfig() asks for the wizard although the environment completes the configuration")
	}

	data, err := os.ReadFile(filepath.Join("etc", "ppanel.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var written config.File
	if err := yaml.Unmarshal(data, &written); err != nil {
		t.Fatalf("written file: %v\n%s", err, data)
	}
	if written.AppLocation != "Europe/Paris" {
		t.Fatalf("AppLocation = %q, want Europe/Paris", written.AppLocation)
	}
	if zone := (orm.Mysql{Config: written.Database}).SessionLocation(); zone != "Europe/Paris" {
		t.Fatalf("database session zone = %q, want the AppLocation (parameters %q)", zone, written.Database.Config)
	}
	if written.Database.Addr != "127.0.0.1:3306" || written.Database.Dbname != "ppanel" || written.Database.Username != "root" || written.Database.Password != "db-secret" {
		t.Fatalf("database = %+v, want the PPANEL_DB connection", written.Database)
	}
	if written.Redis.Host != "127.0.0.1:6379" || written.Redis.Pass != "redis-secret" || written.Redis.DB != 2 {
		t.Fatalf("redis = %+v, want the PPANEL_REDIS connection", written.Redis)
	}
	if written.JwtAuth.AccessSecret == "" {
		t.Fatal("the written file has no JWT secret")
	}
	if written.Transport.Driver != "hertz" {
		t.Fatalf("Transport = %+v, want the file's transport kept", written.Transport)
	}
	if !written.TLS.Enable || written.TLS.CertFile != "/etc/ppanel/tls.crt" || written.TLS.KeyFile != "/etc/ppanel/tls.key" {
		t.Fatalf("TLS = %+v, want the file's TLS settings kept", written.TLS)
	}
	if !written.EdgeSubscribe.Enabled || written.EdgeSubscribe.MaxClockSkewSeconds != 60 || len(written.EdgeSubscribe.Keys) != 1 || written.EdgeSubscribe.Keys[0].ID != "worker" {
		t.Fatalf("EdgeSubscribe = %+v, want the file's edge subscribe settings kept", written.EdgeSubscribe)
	}
	// The process starts on the configuration it wrote.
	if c.DatabaseConfig() != written.Database || c.Redis != written.Redis || c.JwtAuth != written.JwtAuth {
		t.Fatalf("loaded configuration %+v differs from the written file", c.Boot)
	}
}
