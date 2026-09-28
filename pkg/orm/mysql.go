package orm

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/perfect-panel/server/pkg/logger"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const (
	DriverMySQL     = "mysql"
	DriverPostgres  = "postgres"
	DriverPostgres2 = "postgresql"

	// DefaultLocation is the time zone the default connection parameters
	// use when the caller names none.
	DefaultLocation = "Asia/Shanghai"
	// DefaultMySQLConfig and DefaultPostgresConfig are the default
	// connection parameters for DefaultLocation, the values
	// DefaultMySQLQuery and DefaultPostgresQuery return for it.
	DefaultMySQLConfig             = "charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai&interpolateParams=true"
	legacyDefaultMySQLConfig       = "charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai"
	DefaultPostgresConfig          = "sslmode=disable&TimeZone=Asia/Shanghai&application_name=perfect-panel"
	DefaultSlowThresholdMs         = 1000
	DefaultConnMaxLifetimeSeconds  = 1800
	DefaultConnMaxIdleTimeSeconds  = 300
	defaultPostgresApplicationName = "perfect-panel"
)

func locationOrDefault(location string) string {
	if location == "" {
		return DefaultLocation
	}
	return location
}

// DefaultMySQLQuery returns the default MySQL connection parameters, reading
// DATETIME values in location (DefaultLocation when empty).
func DefaultMySQLQuery(location string) string {
	return legacyMySQLQuery(location) + "&interpolateParams=true"
}

// legacyMySQLQuery is the default MySQL parameters before interpolation was
// enabled by default.
func legacyMySQLQuery(location string) string {
	return "charset=utf8mb4&parseTime=true&loc=" + url.QueryEscape(locationOrDefault(location))
}

// DefaultPostgresQuery returns the default PostgreSQL connection parameters,
// with the session time zone set to location (DefaultLocation when empty).
// The zone's slashes stay visible; see encodePostgresParams.
func DefaultPostgresQuery(location string) string {
	zone := strings.ReplaceAll(url.QueryEscape(locationOrDefault(location)), "%2F", "/")
	return "sslmode=disable&TimeZone=" + zone + "&application_name=" + defaultPostgresApplicationName
}

// isDefaultMySQLQuery reports whether query is a default MySQL parameter set
// written for location or for DefaultLocation, which a PostgreSQL
// connection replaces with its own defaults.
func isDefaultMySQLQuery(query, location string) bool {
	switch query {
	case DefaultMySQLConfig, legacyDefaultMySQLConfig, DefaultMySQLQuery(location), legacyMySQLQuery(location):
		return true
	}
	return false
}

type Config struct {
	Driver          string `yaml:"Driver" default:"mysql"`
	Addr            string `yaml:"Addr"`
	Username        string `yaml:"Username"`
	Password        string `yaml:"Password"`
	Dbname          string `yaml:"Dbname"`
	Config          string `yaml:"Config" default:"charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai&interpolateParams=true"`
	MaxIdleConns    int    `yaml:"MaxIdleConns" default:"10"`
	MaxOpenConns    int    `yaml:"MaxOpenConns" default:"10"`
	ConnMaxLifetime int64  `yaml:"ConnMaxLifetime" default:"1800"`
	ConnMaxIdleTime int64  `yaml:"ConnMaxIdleTime" default:"300"`
	SlowThreshold   int64  `yaml:"SlowThreshold" default:"1000"`
}

type Mysql struct {
	Config Config
	// Location is the IANA time zone of the default connection parameters,
	// used when Config.Config is empty or, for PostgreSQL, still a MySQL
	// default. Empty keeps DefaultLocation.
	Location string
}

func NormalizeDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "", DriverMySQL:
		return DriverMySQL
	case DriverPostgres, DriverPostgres2, "pgsql":
		return DriverPostgres
	default:
		return strings.ToLower(strings.TrimSpace(driver))
	}
}

func (m Mysql) Driver() string {
	return NormalizeDriver(m.Config.Driver)
}

func (m Mysql) Dsn() string {
	switch m.Driver() {
	case DriverPostgres:
		return m.postgresDsn()
	default:
		return m.mysqlDsn()
	}
}

func (m Mysql) MigrationDsn() string {
	return m.Dsn()
}

func (m Mysql) mysqlDsn() string {
	query := m.Config.Config
	if query == "" {
		query = DefaultMySQLQuery(m.Location)
	} else {
		query = withDefaultMySQLParams(query)
	}
	return m.Config.Username + ":" + m.Config.Password + "@tcp(" + m.Config.Addr + ")/" + m.Config.Dbname + "?" + query
}

// withDefaultMySQLParams enables client-side placeholder interpolation only
// for the UTF-8 connection settings the application supports by default. It
// saves a prepare/execute/close round trip for ordinary GORM queries, while an
// explicit false value and custom legacy multibyte character sets are left
// untouched.
func withDefaultMySQLParams(query string) string {
	params, err := url.ParseQuery(query)
	if err != nil {
		return query
	}
	if _, configured := params["interpolateParams"]; configured {
		return query
	}
	charset := strings.ToLower(params.Get("charset"))
	collation := strings.ToLower(params.Get("collation"))
	if charset != "utf8mb4" && charset != "utf8mb4,utf8" {
		return query
	}
	if collation != "" && !strings.HasPrefix(collation, "utf8mb4_") && !strings.HasPrefix(collation, "utf8_") {
		return query
	}
	params.Set("interpolateParams", "true")
	return params.Encode()
}

func (m Mysql) postgresDsn() string {
	query := m.Config.Config
	if query == "" || isDefaultMySQLQuery(query, m.Location) {
		query = DefaultPostgresQuery(m.Location)
	}
	u := url.URL{
		Scheme: DriverPostgres,
		Host:   m.Config.Addr,
		Path:   "/" + m.Config.Dbname,
	}
	if m.Config.Username != "" {
		u.User = url.UserPassword(m.Config.Username, m.Config.Password)
	}
	params, err := url.ParseQuery(query)
	if err != nil {
		u.RawQuery = query
	} else {
		if params.Get("application_name") == "" {
			params.Set("application_name", defaultPostgresApplicationName)
		}
		u.RawQuery = encodePostgresParams(params)
	}
	return u.String()
}

// encodePostgresParams keeps IANA time-zone separators visible in the final
// DSN. pgx decodes ordinary URL query values correctly, but GORM's PostgreSQL
// dialector also extracts TimeZone directly from the raw DSN with a regular
// expression. Leaving the slash as %2F therefore makes GORM ask both Go and
// PostgreSQL for a literal zone such as "Asia%2FShanghai".
//
// Only the slash in supported time-zone parameters is unescaped; all other
// parameter values remain URL encoded. This preserves credentials and custom
// settings while accepting both legacy Asia%2FShanghai configuration and the
// clearer Asia/Shanghai form.
func encodePostgresParams(params url.Values) string {
	query := params.Encode()
	for _, key := range []string{"TimeZone", "timezone", "time_zone"} {
		for _, value := range params[key] {
			encodedKey := url.QueryEscape(key)
			encodedValue := url.QueryEscape(value)
			visibleValue := strings.ReplaceAll(encodedValue, "%2F", "/")
			if visibleValue == encodedValue {
				continue
			}
			query = strings.Replace(query, encodedKey+"="+encodedValue, encodedKey+"="+visibleValue, 1)
		}
	}
	return query
}

// gormConfig is the GORM configuration of the application's connection.
// TranslateError makes the dialector report driver errors such as a
// unique-key violation as GORM's portable errors (gorm.ErrDuplicatedKey),
// which callers test with errors.Is: without it those checks never match on
// MySQL or PostgreSQL.
func (m *Mysql) gormConfig() *gorm.Config {
	return &gorm.Config{
		Logger: &logger.GormLogger{SlowThreshold: m.GetSlowThreshold()},
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		TranslateError: true,
	}
}

func (m *Mysql) GetSlowThreshold() time.Duration {
	return time.Duration(m.Config.SlowThreshold) * time.Millisecond
}
func (m *Mysql) GetColorful() bool {
	return true
}

func ConnectMysql(m Mysql) (*gorm.DB, error) {
	return ConnectDatabase(m)
}

func ConnectDatabase(m Mysql) (*gorm.DB, error) {
	if m.Config.Dbname == "" {
		return nil, errors.New("database name is empty")
	}
	var dialector gorm.Dialector
	switch m.Driver() {
	case DriverMySQL:
		dialector = mysql.New(mysql.Config{DSN: m.Dsn()})
	case DriverPostgres:
		dialector = postgres.Open(m.Dsn())
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", m.Config.Driver)
	}
	db, err := gorm.Open(dialector, m.gormConfig())
	if err != nil {
		return nil, err
	}
	sqldb, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqldb.SetMaxIdleConns(m.Config.MaxIdleConns)
	sqldb.SetMaxOpenConns(m.Config.MaxOpenConns)
	if m.Config.ConnMaxLifetime > 0 {
		sqldb.SetConnMaxLifetime(time.Duration(m.Config.ConnMaxLifetime) * time.Second)
	}
	if m.Config.ConnMaxIdleTime > 0 {
		sqldb.SetConnMaxIdleTime(time.Duration(m.Config.ConnMaxIdleTime) * time.Second)
	}
	return db, nil
}
