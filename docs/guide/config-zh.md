# PPanel 配置指南

本文件为 PPanel 应用程序的配置文件提供全面指南。配置文件采用 YAML 格式，定义了服务器、日志、数据库、Redis 和管理员访问的相关设置。

## 1. 配置文件概述

- **默认路径**：`./etc/ppanel.yaml`
- **自定义路径**：通过启动参数 `--config` 指定配置文件路径。
- **格式**：YAML 格式，支持注释，文件名需以 `.yaml` 结尾。

## 2. 配置文件结构

以下是配置文件示例，包含默认值和说明：

```yaml
# PPanel 配置文件
Host: "0.0.0.0"                     # 服务监听地址
Port: 8080                          # 服务监听端口
Debug: false                        # 是否开启调试模式（禁用后台日志）
JwtAuth: # JWT 认证配置
  AccessSecret: ""                  # 访问令牌密钥（必填，见 3.2）
  AccessExpire: 604800              # 访问令牌过期时间（秒）
Logger: # 日志配置
  ServiceName: "PPanel"             # 日志服务标识名称
  Mode: "file"                      # 日志输出模式（console、file、volume）
  Encoding: "json"                  # 日志格式（json、plain）
  TimeFormat: "2006-01-02 15:04:05.000"  # 自定义时间格式
  Path: "logs"                      # 日志文件目录
  Level: "info"                     # 日志级别（debug、info、error、severe）
  Compress: false                   # 是否压缩日志文件
  KeepDays: 30                      # 日志保留天数
  StackCooldownMillis: 100          # 堆栈日志冷却时间（毫秒）
  MaxBackups: 30                    # 最大日志备份数
  MaxSize: 100                      # 最大日志文件大小（MB）
  Rotation: "daily"                 # 日志轮转策略（daily、size）
Database: # MySQL、MariaDB 或 PostgreSQL 数据库配置
  Driver: "mysql"                   # mysql 或 postgres
  Addr: ""                          # 数据库地址（必填）
  Username: ""                      # 数据库用户名（必填）
  Password: ""                      # 数据库密码（必填）
  Dbname: ""                        # 数据库名（必填）
  Config: "charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai&interpolateParams=true"  # 对应数据库的连接参数
  MaxIdleConns: 10                  # 最大空闲连接数
  MaxOpenConns: 10                  # 最大打开连接数
  ConnMaxLifetime: 1800             # 连接最大生命周期（秒）
  ConnMaxIdleTime: 300              # 空闲连接最大保留时间（秒）
  SlowThreshold: 1000               # 慢查询阈值（毫秒）
Redis: # Redis 配置
  Host: "localhost:6379"            # Redis 地址
  Pass: ""                          # Redis 密码
  DB: 0                             # Redis 数据库索引
Administrator: # 首位管理员，仅在首次启动时创建
  Email: "admin@ppanel.dev"         # 管理员登录邮箱
  Password: ""                      # 管理员登录密码，留空则自动生成
```

## 3. 配置项说明

### 3.1 服务器设置

- **`Host`**：服务监听的地址。
  - 默认：`0.0.0.0`（监听所有网络接口）。
  - 它只是绑定地址。支付回调等对外链接由站点设置中的站点地址或支付方式自己的域名生成。
- **`Port`**：服务监听的端口。
  - 默认：`8080`。
- **`Debug`**：是否开启调试模式，开启后禁用后台日志功能。
  - 默认：`false`。
- **`AppLocation`**：应用使用的 IANA 时区，"今天"、到期提醒、流量重置周期和每日统计都按它计算。
  - 默认：`Asia/Shanghai`。
  - 任何 IANA 时区名都可用：二进制内嵌了时区数据库（`time/tzdata`），宿主机和容器镜像都不需要 zoneinfo 文件。
  - 服务启动时还会把它设为进程时区（不论 `TZ` 如何设置），因此写入的所有时间（包括自动填写的 `created_at` /
    `updated_at`）都在同一个时钟上。
  - 必须与数据库时区（`Database.Config` 中 MySQL 的 `loc`、PostgreSQL 的 `TimeZone`）一致：时间按数据库时区存储，
    统计也按数据库时区分天。安装页以及通过 `PPANEL_DB` / `PPANEL_REDIS` 环境变量完成的安装都会把 `AppLocation`
    的时区写进新数据库的连接参数；对已有数据库，两者不一致时服务启动会记录错误日志。只能在空库上，或把已存储的时间换算之后再修改数据库时区，否则所有已存时间都会被重新解读。

### 3.2 JWT 认证 (`JwtAuth`)

- **`AccessSecret`**：访问令牌的密钥。会话、订单事件凭据和游客结账签名都由它派生，为空则任何人都能伪造。
  - 必填：为空时服务启动即退出；短于 16 个字符时会记录警告。请使用足够长的随机值。
  - 只有全新安装会自动生成：默认的 `etc/ppanel.yaml` 没有密钥时，安装向导（或用 `PPANEL_DB` / `PPANEL_REDIS`
    环境变量补全配置的流程）会生成一个并写入文件；通过 `--config` 指定的文件必须自带密钥。
- **`AccessExpire`**：令牌过期时间（秒）。
  - 默认：`604800`（7天）。

### 3.3 日志配置 (`Logger`)

- **`ServiceName`**：日志的服务标识名称，在 `volume` 模式下用作日志目录名。
  - 默认：`PPanel`。
- **`Mode`**：日志输出方式。
  - 选项：`console`（标准输出/错误输出）、`file`（写入指定目录）、`volume`（Docker 卷）。
  - 默认：`file`。
- **`Encoding`**：日志格式。
  - 选项：`json`（结构化 JSON）、`plain`（纯文本，带颜色）。
  - 默认：`json`。
- **`TimeFormat`**：日志时间格式。
  - 默认：`2006-01-02 15:04:05.000`。
- **`Path`**：日志文件存储目录。
  - 默认：`logs`。
- **`Level`**：日志过滤级别。
  - 选项：`debug`（全部）、`info`（除 debug 外的全部）、`error`（仅错误、慢查询和堆栈）、`severe`（不输出任何日志：
    服务没有高于 `error` 的日志，这一级别相当于关闭日志）。
  - 默认：`info`。
- **`Compress`**：是否压缩日志文件（仅在 `file` 模式下生效）。
  - 默认：`false`。
- **`KeepDays`**：日志文件保留天数。
  - 默认：`30`。
- **`StackCooldownMillis`**：堆栈日志冷却时间（毫秒），防止日志过多。
  - 默认：`100`。
- **`MaxBackups`**：最大日志备份数量（仅在 `size` 轮转时生效）。
  - 默认：`30`。
- **`MaxSize`**：日志文件最大大小（MB，仅在 `size` 轮转时生效）。
  - 默认：`100`。
- **`Rotation`**：日志轮转策略。
  - 选项：`daily`（按天轮转）、`size`（按大小轮转）。
  - 默认：`daily`。

### 3.4 数据库 (`Database`)

- **`Driver`**：数据库类型，可选 `mysql` 或 `postgres`。MySQL 8 与 MariaDB 11.8 均使用 `mysql`。
  - 默认：`mysql`。
- **`Addr`**：数据库服务器地址。
  - 必填。
- **`Username`**：数据库用户名。
  - 必填。
- **`Password`**：数据库密码。
  - 必填。
- **`Dbname`**：数据库名。
  - 必填。
- **`Config`**：对应数据库的连接参数。
  - MySQL 默认：`charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai&interpolateParams=true`。
  - PostgreSQL 默认：`sslmode=disable&TimeZone=Asia/Shanghai&application_name=perfect-panel`。
  - PostgreSQL 请始终显式设置 `sslmode`：驱动（pgx）把缺失的 `sslmode` 当作 `prefer`，服务器不提供 TLS 时会静默
    回退到明文（lib/pq 过去默认 `require`）。经网络访问的数据库请用 `sslmode=require` 或 `sslmode=verify-full`；
    默认参数中的 `disable` 只适合同一主机或内网中的数据库。
  - 这些参数中的时区即数据库时区，见 `AppLocation`。
- **`MaxIdleConns`**：最大空闲连接数。
  - 默认：`10`。
- **`MaxOpenConns`**：最大打开连接数。
  - 默认：`10`。
- **`ConnMaxLifetime`**：连接池中单个连接的最大复用时间（秒）。
  - 默认：`1800`。
- **`ConnMaxIdleTime`**：空闲连接在连接池中的最大保留时间（秒）。
  - 默认：`300`。
- **`SlowThreshold`**：慢查询阈值（毫秒）。
  - 默认：`1000`。

### 3.5 Redis 配置 (`Redis`)

- **`Host`**：Redis 服务器地址。
  - 默认：`localhost:6379`。
- **`Pass`**：Redis 密码。
  - 默认：`""`（无密码）。
- **`DB`**：Redis 数据库索引。
  - 默认：`0`。

### 3.6 管理员登录 (`Administrator`)

仅在数据库中还没有任何用户时使用一次，用来创建首位管理员。

- **`Email`**：管理员登录邮箱。
  - 默认：`admin@ppanel.dev`。
- **`Password`**：管理员登录密码。
  - 默认：空。留空时会生成随机密码并在启动日志（`docker logs`）中打印一次，请用它登录后立即修改；
    填写了则按填写的值使用。

### 3.7 邮件发送（SMTP）

SMTP 中继不在本文件中配置：管理员在面板的系统设置（邮件）里填写，配置保存在数据库中。字段如下：

| 字段 | 含义 |
|---|---|
| `host`、`port` | 中继地址。465 端口为隐式 TLS（SMTPS）；25、587、2525 以明文开始，再通过 STARTTLS 升级加密。 |
| `user`、`pass` | 中继凭据；留空则不认证。 |
| `from`、`reply_to` | 发件地址与可选的回复地址。显示名称取站点名称。 |
| `ssl` | 要求加密。465 端口（或开启 `implicit_tls`）时连接一开始就是 TLS；其他端口必须通过 STARTTLS 升级，中继不提供 STARTTLS 时会在发送凭据之前拒绝。Mailgun、SendGrid、Postmark、Brevo 的 STARTTLS 端口（587、2525）就用这个设置。关闭时，中继提供 STARTTLS 就用，否则保持明文。 |
| `implicit_tls` | 在 465 之外的端口上以 TLS 握手开始连接。仅用于在非标准端口提供 SMTPS 的中继；不要用于 STARTTLS 端口，否则握手会撞上明文问候，邮件发不出去。 |
| `insecure_skip_verify` | 接受任何中继证书。仅用于自签名证书的中继；默认会校验证书。 |

## 4. 环境变量

以下环境变量可用于覆盖配置文件中的设置：

| 环境变量           | 配置项      | 示例值                                          |
|----------------|----------|----------------------------------------------|
| `PPANEL_DB`    | MySQL/MariaDB 配置 | `root:password@tcp(localhost:3306)/vpnboard` |
| `PPANEL_REDIS` | Redis 配置 | `redis://localhost:6379`                     |

二者共同补全一份新的默认配置文件（尚无 `JwtAuth.AccessSecret`）时，服务会把文件写回：包含生成的密钥、带有
`AppLocation` 时区的数据库连接参数和 Redis 连接；写回的文件保留 `Transport`、`TLS` 与 `EdgeSubscribe` 段。

## 5. 最佳实践

- **安全性**：首次登录后请修改首位管理员的密码。只要还有管理员在使用旧默认密码 `password`，服务启动时都会记录错误日志。
- **日志**：生产环境中建议使用 `file` 或 `volume` 模式持久化日志，将 `Level` 设置为 `error` 以减少日志量；`severe`
  会关闭全部日志输出。
- **数据库**：确保 `Database` 和 `Redis` 凭据安全，避免在版本控制中暴露。
- **JWT**：为 `JwtAuth` 的 `AccessSecret` 设置强密钥以增强安全性。

如需进一步帮助，请参考 PPanel 官方文档或联系支持团队。
