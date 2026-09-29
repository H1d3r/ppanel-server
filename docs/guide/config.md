# PPanel Configuration Guide

This document provides a comprehensive guide to the configuration file for the PPanel application. The configuration
file is in YAML format and defines settings for the server, logging, database, Redis, and admin access.

## 1. Configuration File Overview

- **Default Path**: `./etc/ppanel.yaml`
- **Custom Path**: Specify a custom path using the `--config` startup parameter.
- **Format**: YAML, supports comments, and must be named with a `.yaml` extension.

## 2. Configuration File Structure

Below is an example of the configuration file with default values and explanations:

```yaml
# PPanel Configuration
Host: "0.0.0.0"                     # Server listening address
Port: 8080                          # Server listening port
Debug: false                        # Enable debug mode (disables background logging)
JwtAuth: # JWT authentication settings
  AccessSecret: ""                  # Access token secret (required, see 3.2)
  AccessExpire: 604800              # Access token expiration (seconds)
Logger: # Logging configuration
  ServiceName: "PPanel"             # Service name for log identification
  Mode: "file"                      # Log output mode (console, file, volume)
  Encoding: "json"                  # Log format (json, plain)
  TimeFormat: "2006-01-02 15:04:05.000"  # Custom time format
  Path: "logs"                      # Log file directory
  Level: "info"                     # Log level (debug, info, error, severe)
  Compress: false                   # Enable log compression
  KeepDays: 30                      # Log retention period (days)
  StackCooldownMillis: 100          # Stack trace cooldown (milliseconds)
  MaxBackups: 30                    # Maximum number of log backups
  MaxSize: 100                      # Maximum log file size (MB)
  Rotation: "daily"                 # Log rotation strategy (daily, size)
Database: # MySQL, MariaDB, or PostgreSQL database configuration
  Driver: "mysql"                   # mysql or postgres
  Addr: ""                          # Database address (required)
  Username: ""                      # Database username (required)
  Password: ""                      # Database password (required)
  Dbname: ""                        # Database name (required)
  Config: "charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai&interpolateParams=true"  # Dialect connection parameters
  MaxIdleConns: 10                  # Maximum idle connections
  MaxOpenConns: 10                  # Maximum open connections
  ConnMaxLifetime: 1800             # Maximum connection lifetime (seconds)
  ConnMaxIdleTime: 300              # Maximum connection idle time (seconds)
  SlowThreshold: 1000               # Slow query threshold (milliseconds)
Redis: # Redis configuration
  Host: "localhost:6379"            # Redis address
  Pass: ""                          # Redis password
  DB: 0                             # Redis database index
Administrator: # First administrator, created on the first start
  Email: "admin@ppanel.dev"         # Admin login email
  Password: ""                      # Admin login password; empty = generate one
```

## 3. Configuration Details

### 3.1 Server Settings

- **`Host`**: Address the server listens on.
  - Default: `0.0.0.0` (all network interfaces).
  - It is only a bind address. Public links, such as payment callback URLs, are built from the site host
    (site settings) or a payment method's own domain.
- **`Port`**: Port the server listens on.
  - Default: `8080`.
- **`Debug`**: Enables debug mode, disabling background logging.
  - Default: `false`.
- **`AppLocation`**: IANA time zone of the application: "today", expiry reminders, reset cycles and the daily
  statistics are computed in it.
  - Default: `Asia/Shanghai`.
  - Any IANA zone name works: the binary embeds the time zone database (`time/tzdata`), so neither the host
    nor the container image needs zoneinfo files.
  - The server also makes it the process time zone at startup, whatever `TZ` says, so every time it writes
    (automatic `created_at` / `updated_at` included) is on the same clock.
  - It must match the database time zone (`Database.Config`: MySQL `loc`, PostgreSQL `TimeZone`), in which
    timestamps are stored and statistics are grouped by day. The setup page, and the `PPANEL_DB` /
    `PPANEL_REDIS` environment-variable install, write the zone of `AppLocation` into a new database's
    parameters; for an existing database the server logs an error at startup when they differ. Change the database zone only on an empty database or after converting the stored times: changing
    it reinterprets every stored time.

### 3.2 JWT Authentication (`JwtAuth`)

- **`AccessSecret`**: Secret key for access tokens. Sessions, order event tickets and guest-checkout signatures
  are all keyed by it, so an empty secret would let anyone forge them.
  - Required: the server exits at startup when it is empty, and logs a warning when it is shorter than 16
    characters. Use a long random value.
  - Only a new installation gets one generated: when the default `etc/ppanel.yaml` has no secret, the setup
    wizard (or the `PPANEL_DB` / `PPANEL_REDIS` environment variables completing the file) writes a generated
    secret into it. A file given with `--config` must already contain one.
- **`AccessExpire`**: Token expiration time in seconds.
  - Default: `604800` (7 days).

### 3.3 Logging (`Logger`)

- **`ServiceName`**: Identifier for logs; in `volume` mode it names the log directory.
  - Default: `PPanel`.
- **`Mode`**: Log output destination.
  - Options: `console` (stdout/stderr), `file` (to a directory), `volume` (Docker volume).
  - Default: `file`.
- **`Encoding`**: Log format.
  - Options: `json` (structured JSON), `plain` (plain text with colors).
  - Default: `json`.
- **`TimeFormat`**: Custom time format for logs.
  - Default: `2006-01-02 15:04:05.000`.
- **`Path`**: Directory for log files.
  - Default: `logs`.
- **`Level`**: Log filtering level.
  - Options: `debug` (everything), `info` (everything but debug), `error` (errors, slow queries and stack traces
    only), `severe` (nothing: the server writes no entry above `error`, so this level silences the log).
  - Default: `info`.
- **`Compress`**: Enable compression for log files (only in `file` mode).
  - Default: `false`.
- **`KeepDays`**: Retention period for log files (in days).
  - Default: `30`.
- **`StackCooldownMillis`**: Cooldown for stack trace logging to prevent log flooding.
  - Default: `100`.
- **`MaxBackups`**: Maximum number of log backups (for `size` rotation).
  - Default: `30`.
- **`MaxSize`**: Maximum log file size in MB (for `size` rotation).
  - Default: `100`.
- **`Rotation`**: Log rotation strategy.
  - Options: `daily` (rotate daily), `size` (rotate by size).
  - Default: `daily`.

### 3.4 Database (`Database`)

- **`Driver`**: Database dialect: `mysql` or `postgres`. Use `mysql` for both MySQL 8 and MariaDB 11.8.
  - Default: `mysql`.
- **`Addr`**: Database server address.
  - Required.
- **`Username`**: Database username.
  - Required.
- **`Password`**: Database password.
  - Required.
- **`Dbname`**: Database name.
  - Required.
- **`Config`**: Dialect-specific connection parameters.
  - MySQL default: `charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai&interpolateParams=true`.
  - PostgreSQL default: `sslmode=disable&TimeZone=Asia/Shanghai&application_name=perfect-panel`.
  - PostgreSQL: always set `sslmode` explicitly. The driver (pgx) treats a missing `sslmode` as `prefer`, which
    silently falls back to plaintext when the server offers no TLS (lib/pq used to default to `require`). Use
    `sslmode=require` or `sslmode=verify-full` for a database reached over a network; `disable`, as in the
    default parameters, only for one on the same host or private network.
  - The zone in these parameters is the database time zone; see `AppLocation`.
- **`MaxIdleConns`**: Maximum idle connections.
  - Default: `10`.
- **`MaxOpenConns`**: Maximum open connections.
  - Default: `10`.
- **`ConnMaxLifetime`**: Maximum time a pooled connection may be reused, in seconds.
  - Default: `1800`.
- **`ConnMaxIdleTime`**: Maximum time an idle pooled connection is retained, in seconds.
  - Default: `300`.
- **`SlowThreshold`**: Threshold for slow query logging (in milliseconds).
  - Default: `1000`.

### 3.5 Redis (`Redis`)

- **`Host`**: Redis server address.
  - Default: `localhost:6379`.
- **`Pass`**: Redis password.
  - Default: `""` (no password).
- **`DB`**: Redis database index.
  - Default: `0`.

### 3.6 Admin Login (`Administrator`)

Used once, to create the first administrator when the database has no users.

- **`Email`**: Admin login email.
  - Default: `admin@ppanel.dev`.
- **`Password`**: Admin login password.
  - Default: empty. When it is empty, a random password is generated and printed once in the startup log
    (`docker logs`). Sign in with it and change it. A configured value is used as given.

### 3.7 Email Delivery (SMTP)

The SMTP relay is not part of this file: administrators configure it in the panel (system settings, email),
and the settings are stored in the database. The fields are:

| Field | Meaning |
|---|---|
| `host`, `port` | Relay address. Port 465 is implicit TLS (SMTPS); 25, 587 and 2525 start in plaintext and upgrade with STARTTLS. |
| `user`, `pass` | Relay credentials; empty to send without authentication. |
| `from`, `reply_to` | Sender address and, optionally, the reply address. The display name is the site name. |
| `ssl` | Encryption required. On port 465 (or with `implicit_tls`) the connection starts with TLS; on any other port the session must upgrade with STARTTLS, and a relay that does not offer it is refused before the credentials are sent. This is the setting to use with the STARTTLS ports of Mailgun, SendGrid, Postmark or Brevo (587, 2525). Off, STARTTLS is used when the relay offers it and the session stays in plaintext otherwise. |
| `implicit_tls` | Start the connection with a TLS handshake on a port other than 465. Only for relays that serve SMTPS on a non-standard port; never for a STARTTLS port, where the handshake would meet a plaintext greeting and no mail would go out. |
| `insecure_skip_verify` | Accept any relay certificate. Only for relays with a self-signed certificate; certificates are verified by default. |

## 4. Environment Variables

The following environment variables can be used to override configuration settings:

| Environment Variable | Configuration Section | Example Value                                |
|----------------------|-----------------------|----------------------------------------------|
| `PPANEL_DB`          | MySQL/MariaDB          | `root:password@tcp(localhost:3306)/vpnboard` |
| `PPANEL_REDIS`       | Redis                 | `redis://localhost:6379`                     |

When both complete a new default configuration file (no `JwtAuth.AccessSecret` yet), the server writes the
file back with a generated secret, the database parameters carrying the `AppLocation` zone and the Redis
connection; the rewritten file keeps the `Transport`, `TLS` and `EdgeSubscribe` sections.

## 5. Best Practices

- **Security**: Change the first administrator's password after the first sign-in. The server logs an error at
  startup while any administrator still uses the old default password `password`.
- **Logging**: Use `file` or `volume` mode for production to persist logs. Set `Level` to `error` to reduce log
  volume; `severe` writes nothing at all.
- **Database**: Ensure `Database` and `Redis` credentials are secure and not exposed in version control.
- **JWT**: Specify a strong `AccessSecret` for `JwtAuth` to enhance security.

For further assistance, refer to the official PPanel documentation or contact support.
