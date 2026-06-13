# Codex Usage Tracker 使用文档

Codex Usage Tracker 用来统计多台设备上的 Codex token 使用量和估算成本。每台电脑运行一个无界面客户端，客户端定时扫描本机 Codex 会话日志，提取 token 记录后上报到服务端；服务端使用 SQLite 存储明细和聚合数据，并提供网页 dashboard 和 JSON API。

## 项目解决方案

本项目采用“本地客户端 + 中心服务端 + 网页展示”的结构。

```text
Windows/macOS 客户端
  -> 扫描本机 Codex JSONL 日志
  -> 按配置的 identity_key 标记身份
  -> 通过 HTTP 上报 usage events

服务端
  -> 校验 API key
  -> 写入 SQLite
  -> 按小时/天生成 rollup
  -> 根据 model_prices 估算成本

网页 dashboard
  -> 查看总 token、总成本、事件数、缺失价格
  -> 按 identity/device/model 分组
  -> 查看最近事件
```

核心身份规则：

- 每台设备会生成一个本地 `device_id`，用于区分设备。
- 每台设备配置一个 `identity_key`，用于统计时合并身份。
- 如果电脑 1 配置 `identity_key: a`，电脑 2 配置 `identity_key: b`，网页按 identity 展示时会分开统计。
- 如果电脑 1 和电脑 2 都配置 `identity_key: a`，网页按 identity 展示时会合并统计，但按 device 展示时仍然能看到两台设备各自的用量。

## 环境要求

开发和运行要求：

- Go 1.22 或更高版本。
- Windows 或 macOS 客户端。
- 服务端可运行在 Windows、macOS、Linux 或 VPS 上。
- SQLite，无需单独部署数据库服务。
- 服务端需要有一个客户端可以访问的 HTTP 地址。

可选工具：

- `sqlite3`：用于手动查看或维护 `model_prices` 表。
- `curl` 或 PowerShell `Invoke-WebRequest`：用于检查服务端健康状态。

当前限制：

- 客户端服务安装只支持 Windows Service 和 macOS LaunchAgent。
- Linux 客户端前台运行可用，但服务安装命令暂未实现。
- 成本是按服务端 `model_prices` 表估算的，不等于官方账单的最终结算值。

## 使用流程总览

第一次使用建议按下面顺序执行：

1. 在一台机器或 VPS 上部署服务端。
2. 设置服务端 `api_key` 和 SQLite 数据库路径。
3. 启动服务端，并确认 `/healthz` 正常。
4. 打开服务端首页，确认 dashboard 可以访问。
5. 在每台需要统计的电脑上初始化客户端配置。
6. 给每台客户端填写相同或不同的 `identity_key`。
7. 先以前台方式运行客户端，确认能扫描并上报数据。
8. 到网页 dashboard 查看 identity、device、model 统计是否符合预期。
9. 确认没问题后，把客户端安装成 Windows Service 或 macOS LaunchAgent。
10. 后续只需要维护模型价格、备份 SQLite 数据库、观察 dashboard。

身份配置示例：

```text
电脑 1：identity_key = a
电脑 2：identity_key = b
结果：a 和 b 分开统计

电脑 1：identity_key = a
电脑 2：identity_key = a
结果：两台电脑在 identity=a 下合并统计
```

推荐先在本机用 `localhost` 跑通完整链路，再部署到 VPS 或局域网服务器。跑通链路的判断标准是：

- 服务端 `/healthz` 返回 `{"ok":true}`。
- 客户端前台运行没有认证错误。
- dashboard 的 Recent Events 能看到新事件。
- Breakdown 选择 `Identity` 后，身份合并规则符合预期。

## 部署流程

### 部署方式选择

常见部署方式有三种：

| 场景 | 服务端位置 | 客户端位置 | 适合情况 |
| --- | --- | --- | --- |
| 本机试用 | 当前电脑 | 当前电脑 | 先验证功能是否可用 |
| 局域网部署 | 局域网服务器或一台常开电脑 | 多台办公电脑 | 团队内网统计 |
| 公网/VPS 部署 | VPS 或云服务器 | 任意能访问公网的设备 | 多地点、多设备统计 |

服务端只需要一个实例。客户端可以有很多个，每个设备运行一个客户端。

### 服务端部署步骤

#### 1. 获取代码并进入项目目录

```bash
cd /path/to/codex_project
```

Windows 示例：

```powershell
cd D:\code\stock\codex_project
```

#### 2. 检查 Go 版本

```bash
go version
```

建议 Go 版本为 1.22 或更高。

Windows 如果 PATH 上的 Go 版本不正确，可以直接使用 64 位 Go：

```powershell
& 'C:\Program Files\Go\bin\go.exe' version
```

#### 3. 准备服务端配置

可以直接复制示例配置：

```bash
cp examples/server.yaml server.yaml
```

Windows PowerShell：

```powershell
Copy-Item examples\server.yaml server.yaml
```

编辑 `server.yaml`：

```yaml
listen_addr: ":8080"
public_base_url: http://your-server:8080
api_key: replace-with-a-long-random-secret
sqlite_path: ./data/codex-usage.sqlite
```

字段填写建议：

- `listen_addr` 本机试用写 `127.0.0.1:8080`，允许外部访问写 `:8080`。
- `public_base_url` 写用户浏览器能访问到的地址，例如 `http://1.2.3.4:8080`。
- `api_key` 必须替换成强随机字符串，客户端也要填同一个值。
- `sqlite_path` 建议放在 `./data/` 或服务器专门的数据目录。

#### 4. 启动服务端

开发/试用方式：

```bash
go run ./cmd/codex-usage-server serve --config ./server.yaml
```

如果想先编译二进制：

```bash
go build -o ./bin/codex-usage-server ./cmd/codex-usage-server
./bin/codex-usage-server serve --config ./server.yaml
```

Windows PowerShell：

```powershell
go build -o .\bin\codex-usage-server.exe .\cmd\codex-usage-server
.\bin\codex-usage-server.exe serve --config .\server.yaml
```

#### 5. 验证服务端

健康检查：

```bash
curl http://localhost:8080/healthz
```

PowerShell：

```powershell
Invoke-WebRequest -UseBasicParsing http://localhost:8080/healthz
```

期望返回：

```json
{"ok":true}
```

打开 dashboard：

```text
http://localhost:8080/
```

如果部署在 VPS，则打开：

```text
http://服务器IP或域名:8080/
```

#### 6. 放行端口

如果服务端要被其他电脑访问，需要放行 `listen_addr` 对应端口。

Windows 防火墙需要允许入站 TCP 端口，例如 8080。

Linux/VPS 常见命令示例：

```bash
sudo ufw allow 8080/tcp
```

云服务器还需要在云厂商安全组里放行同一个端口。

#### 7. 生产部署建议

公网部署时建议：

- 使用 Nginx/Caddy 做 HTTPS 反向代理。
- 不要把 `api_key` 写成简单字符串。
- 定期备份 SQLite 文件。
- 使用 systemd、Windows Service、Supervisor 或 Docker 等方式托管服务端进程。

当前项目没有内置服务端 systemd 安装命令。如果需要长期运行，可以先用你熟悉的进程管理方式托管编译后的 `codex-usage-server`。

### 客户端部署步骤

每台需要统计的设备都要部署客户端。客户端没有界面，负责扫描本机 Codex 日志并上报。

#### 1. 初始化客户端配置

在客户端设备上执行：

```bash
go run ./cmd/codex-usage-client init --config ./client.config.yaml
```

Windows PowerShell：

```powershell
go run .\cmd\codex-usage-client init --config .\client.config.yaml
```

这个命令会做两件事：

- 如果配置文件不存在，创建 `client.config.yaml`。
- 创建 `state.json`，里面保存本机唯一的 `device_id`。

#### 2. 编辑客户端配置

```yaml
server_url: http://your-server:8080
api_key: replace-with-a-long-random-secret
identity_key: a
scan_interval: 5m
codex_home: ""
```

字段填写规则：

- `server_url` 写服务端地址，不要带 `/api/v1/ingest`，客户端会自动拼接。
- `api_key` 必须和服务端 `api_key` 完全一致。
- `identity_key` 是你要统计的身份名，可以是用户名、邮箱前缀、团队成员代号等。
- `scan_interval` 建议先用 `5m`。
- `codex_home` 留空时会读取默认 Codex 目录；如果日志不在默认目录，再手动填写。

多设备配置示例：

```yaml
# 电脑 1
identity_key: a

# 电脑 2
identity_key: a

# 电脑 3
identity_key: b
```

结果：

- 电脑 1 和电脑 2 会合并到 identity `a`。
- 电脑 3 会单独统计为 identity `b`。
- 按 device 分组时仍然能看到三台设备分别的数据。

#### 3. 确认 Codex 日志目录

默认扫描：

```text
~/.codex/sessions/**/*.jsonl
~/.codex/archived_sessions/*.jsonl
```

如果设置了环境变量 `CODEX_HOME`，客户端会优先使用它。

如果需要指定目录：

```yaml
codex_home: "D:/Users/you/.codex"
```

macOS 示例：

```yaml
codex_home: "/Users/you/.codex"
```

#### 4. 前台试运行

先不要急着安装后台服务，建议前台运行一次：

```bash
go run ./cmd/codex-usage-client run --config ./client.config.yaml
```

观察点：

- 没有 `401` 或 `403`，说明 `api_key` 正确。
- 没有连接失败，说明 `server_url` 可访问。
- 服务端 dashboard 的 Recent Events 出现数据。
- 按 Identity 分组结果符合预期。

如果当前没有新的 Codex 会话日志，客户端可能暂时没有可上报事件。这不是错误，可以先使用 Codex 产生一些 session 后再观察。

#### 5. 安装后台服务

前台验证正常后，再安装后台服务。

Windows：

```bash
go run ./cmd/codex-usage-client install-service --config ./client.config.yaml
```

卸载：

```bash
go run ./cmd/codex-usage-client uninstall-service --config ./client.config.yaml
```

macOS：

```bash
go run ./cmd/codex-usage-client install-service --config ./client.config.yaml
```

卸载：

```bash
go run ./cmd/codex-usage-client uninstall-service --config ./client.config.yaml
```

安装后：

- Windows 服务名是 `codex-usage-client`。
- macOS LaunchAgent 名称是 `com.codex-usage-client`。
- 客户端会持续按 `scan_interval` 扫描和上报。

#### 6. 验证后台运行

安装服务后建议再次验证：

1. 打开 dashboard。
2. 查看 Recent Events 是否持续增加。
3. 查看 Breakdown -> Device 是否出现这台设备。
4. 查看 Breakdown -> Identity 是否合并到了预期身份。

如果后台服务没有数据，先回到前台运行方式排查，确认配置和网络没问题后再安装服务。

## 快速开始

### 1. 准备服务端配置

复制或编辑 `examples/server.yaml`：

```yaml
listen_addr: ":8080"
public_base_url: http://localhost:8080
api_key: change-me
sqlite_path: ./data/codex-usage.sqlite
```

字段说明：

- `listen_addr`：服务端监听地址，VPS 上常用 `:8080`。
- `public_base_url`：外部访问地址，主要用于文档和部署记录。
- `api_key`：客户端上报时使用的密钥，生产环境必须改成强随机值。
- `sqlite_path`：SQLite 数据库路径。

启动服务端：

```bash
go run ./cmd/codex-usage-server serve --config ./examples/server.yaml
```

检查健康状态：

```bash
curl http://localhost:8080/healthz
```

返回如下内容表示服务端正常：

```json
{"ok":true}
```

网页地址：

```text
http://localhost:8080/
```

部署到 VPS 时，把 `localhost` 换成服务器 IP 或域名。

### 2. 初始化客户端

每台需要统计的电脑都执行一次：

```bash
go run ./cmd/codex-usage-client init --config ./client.config.yaml
```

如果配置文件不存在，命令会生成一个基础配置，并在同目录生成本机状态文件 `state.json`，其中包含本机 `device_id`。

客户端配置示例：

```yaml
server_url: http://your-server:8080
api_key: change-me
identity_key: a
scan_interval: 5m
codex_home: ""
```

字段说明：

- `server_url`：服务端地址，例如 `http://1.2.3.4:8080`。
- `api_key`：必须和服务端配置一致。
- `identity_key`：统计身份。相同值会合并统计，不同值会分开统计。
- `scan_interval`：扫描间隔，默认建议 `5m`。
- `codex_home`：Codex 数据目录。留空时优先使用 `CODEX_HOME`，否则使用默认 `~/.codex`。

### 3. 前台运行客户端

先用前台模式验证配置是否正确：

```bash
go run ./cmd/codex-usage-client run --config ./client.config.yaml
```

客户端会扫描：

```text
~/.codex/sessions/**/*.jsonl
~/.codex/archived_sessions/*.jsonl
```

如果你的 Codex 数据目录不在默认位置，请在配置里写明：

```yaml
codex_home: "D:/path/to/.codex"
```

或 macOS：

```yaml
codex_home: "/Users/your-name/.codex"
```

### 4. 安装为后台服务

确认前台运行正常后，可以安装为系统后台服务。

Windows：

```bash
go run ./cmd/codex-usage-client install-service --config ./client.config.yaml
```

卸载：

```bash
go run ./cmd/codex-usage-client uninstall-service --config ./client.config.yaml
```

macOS：

```bash
go run ./cmd/codex-usage-client install-service --config ./client.config.yaml
```

卸载：

```bash
go run ./cmd/codex-usage-client uninstall-service --config ./client.config.yaml
```

macOS 会创建 LaunchAgent：`com.codex-usage-client`。Windows 会创建服务：`codex-usage-client`。

## 网页使用

打开服务端首页：

```text
http://your-server:8080/
```

页面提供：

- Total tokens：总 token。
- Total cost：估算总成本。
- Events：已接收事件数。
- Missing prices：没有价格配置的模型数量。
- Summary Buckets：按小时或天展示趋势。
- Breakdown：按 identity、device 或 model 分组。
- Recent Events：最近上报的明细事件。

常见查看方式：

- 看某个账号身份：选择 Group 为 `Identity`。
- 看某台设备：选择 Group 为 `Device`。
- 看模型成本分布：选择 Group 为 `Model`。
- 看小时级趋势：Bucket 选择 `Hour`。

## HTTP API

查询汇总：

```text
GET /api/v1/summary?bucket=day
GET /api/v1/summary?bucket=hour
```

分组查询：

```text
GET /api/v1/breakdown?group_by=identity
GET /api/v1/breakdown?group_by=device
GET /api/v1/breakdown?group_by=model
```

最近事件：

```text
GET /api/v1/events?limit=100
```

可选过滤参数：

```text
identity_key=a
device_id=device-id
model=gpt-5.5
from=2026-06-13T00:00:00Z
to=2026-06-14T00:00:00Z
```

示例：

```text
GET /api/v1/summary?bucket=hour&identity_key=a&from=2026-06-13T00:00:00Z
```

客户端上报接口：

```text
POST /api/v1/ingest
Authorization: Bearer <api_key>
```

## 成本和模型价格

服务端启动时会初始化一些内置模型价格，但模型价格可能变化，也可能出现未内置的新模型。遇到没有价格的模型时：

- 明细事件仍然会入库。
- token 仍然会统计。
- `cost_usd` 会为空。
- dashboard 的 `Missing prices` 会增加。

模型价格表是 SQLite 的 `model_prices`：

```sql
CREATE TABLE model_prices (
  model TEXT PRIMARY KEY,
  input_per_million REAL NOT NULL,
  cache_read_per_million REAL NOT NULL DEFAULT 0,
  cache_creation_per_million REAL NOT NULL DEFAULT 0,
  output_per_million REAL NOT NULL,
  updated_at TEXT NOT NULL
);
```

新增或更新价格：

```bash
sqlite3 ./data/codex-usage.sqlite \
  "INSERT INTO model_prices (
     model,
     input_per_million,
     cache_read_per_million,
     cache_creation_per_million,
     output_per_million,
     updated_at
   )
   VALUES ('gpt-5.2-codex-low', 1, 0.1, 1, 3, 'manual')
   ON CONFLICT(model) DO UPDATE SET
     input_per_million = excluded.input_per_million,
     cache_read_per_million = excluded.cache_read_per_million,
     cache_creation_per_million = excluded.cache_creation_per_million,
     output_per_million = excluded.output_per_million,
     updated_at = excluded.updated_at;"
```

模型名会在入库前做规范化：

- 转小写。
- 去掉 provider 前缀，例如 `openai/gpt-5.5` -> `gpt-5.5`。
- `@` 会替换成 `-`。
- 日期后缀会去掉，例如 `gpt-5.5-2026-05-14` -> `gpt-5.5`。

维护价格时请使用规范化后的模型名。

## 数据文件说明

客户端配置同目录会产生：

- `state.json`：本机 `device_id`、扫描游标等状态。
- `pending.json`：上传失败时暂存的待上报事件。

服务端会产生：

- SQLite 主库，例如 `./data/codex-usage.sqlite`。
- SQLite WAL/SHM 文件，例如 `.sqlite-wal`、`.sqlite-shm`。

这些运行时数据不建议提交到 git。

## 注意事项

部署和安全：

- 生产环境必须修改 `api_key`，不要使用 `change-me`。
- 如果服务端暴露在公网，建议放在 HTTPS 反向代理后面。
- SQLite 文件需要定期备份。
- `sqlite_path` 所在目录需要服务端进程有写权限。

客户端：

- 同一台机器不要随意删除 `state.json`，否则会生成新的 `device_id`。
- 如果删除扫描状态，旧日志可能被重新扫描；服务端会按事件 ID 去重，但路径变化可能导致重新计入。
- 修改 `identity_key` 后，新上报数据会归到新身份，旧数据不会自动迁移。
- 多台设备想合并统计时，必须配置完全相同的 `identity_key`。

成本：

- 成本估算依赖 `model_prices`，价格不完整时只能统计 token，不能准确估算金额。
- 官方价格发生变化时，需要手动更新 `model_prices`。
- 本项目统计的是本地 Codex session 日志可见的数据，不保证覆盖所有官方计费口径。

网络：

- 客户端必须能访问 `server_url`。
- 服务端防火墙需要放行 `listen_addr` 对应端口。
- 如果上报失败，客户端会把事件放入本地 pending 队列，后续继续重试。

## 开发和测试

运行全部测试：

```bash
go test ./...
```

在 Windows 上如果遇到 Go 工具链或架构问题，可以显式使用 64 位 Go：

```powershell
$env:GOTOOLCHAIN='go1.22.0'
$env:GOARCH='amd64'
& 'C:\Program Files\Go\bin\go.exe' test ./...
```

项目包含端到端测试：

- 构造三台设备的 Codex JSONL fixture。
- 两台设备使用 `identity_key=a`。
- 一台设备使用 `identity_key=b`。
- 通过真实 scanner、uploader、server handler 验证 `a` 合并、`b` 分离。

## 常见问题

### 网页有 token 但成本为空

对应模型缺少价格。查看 dashboard 的 `Missing prices`，然后在 SQLite `model_prices` 表中补价格。

### 两台电脑没有合并统计

检查两台电脑的 `client.config.yaml`，确认 `identity_key` 完全一致。大小写、空格、拼写不同都会被当成不同身份。

### 客户端没有上报

检查：

- `server_url` 是否能访问。
- `api_key` 是否和服务端一致。
- `codex_home` 是否指向正确的 Codex 数据目录。
- 本机 Codex 是否已经产生 `sessions/**/*.jsonl` 日志。

### 服务端启动失败

检查：

- `listen_addr` 端口是否被占用。
- `sqlite_path` 目录是否可写。
- 配置文件 YAML 格式是否正确。
