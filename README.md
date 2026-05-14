# admin_back_go

`admin_back_go` 是 `E:\admin_go` 后台系统的 Go 重写后端。它不是玩具 demo，也不是微服务实验；当前定位是 **Gin modular monolith**：一个 API 进程、一个 Worker 进程、共享 MySQL/Redis 状态，逐步承接原 admin 系统的认证、RBAC、系统管理、AI 管理、支付、通知、队列和实时 WebSocket 能力。

> 先说清楚：本仓库不负责前端静态文件托管，不负责宝塔证书申请。Go 后端只负责 HTTP API、Worker、队列、定时任务、WebSocket upgrade 和业务运行时。生产域名、SSL、反向代理放在宿主机 Nginx/OpenResty/宝塔里。

## 目录

- [当前定位](#当前定位)
- [技术栈](#技术栈)
- [进程模型](#进程模型)
- [目录结构](#目录结构)
- [核心能力](#核心能力)
- [配置说明](#配置说明)
- [本地开发](#本地开发)
- [数据库和迁移](#数据库和迁移)
- [Docker 部署](#docker-部署)
- [宝塔 / Nginx 反向代理](#宝塔--nginx-反向代理)
- [前端联动配置](#前端联动配置)
- [验证和运维](#验证和运维)
- [多节点部署注意事项](#多节点部署注意事项)
- [常见问题](#常见问题)
- [重要文档](#重要文档)

## 当前定位

本项目遵守 `E:\admin_go` 的 open-source-first 和 step-by-step 规则：先固定可运行、可验证的后端骨架，再按窄切片迁移旧 PHP admin 业务。

当前后端事实：

```text
HTTP API        Gin
进程入口        cmd/admin-api, cmd/admin-worker
数据库          MySQL
缓存/会话/队列   Redis + Asynq
定时任务        gocron/v2 + DB cron_task 注册
实时能力        gorilla/websocket + local/noop/redis publisher
日志            slog stdout + optional lumberjack file log
响应格式        { "code": 0, "data": {}, "msg": "ok" }
API 前缀        /api/admin/v1
健康检查        /health, /ready
```

## 技术栈

| 类型 | 选型 |
| --- | --- |
| Language | Go `1.26.1` |
| HTTP | Gin |
| ORM | GORM + MySQL driver |
| Redis | `redis/go-redis` |
| Queue | Asynq |
| Scheduler | `go-co-op/gocron/v2` |
| WebSocket | `gorilla/websocket` |
| Logger | `log/slog` + lumberjack |
| Excel export | `excelize` |
| Object storage | 腾讯云 COS STS / COS SDK |
| Captcha | `go-captcha` slide captcha |

## 进程模型

### `admin-api`

入口：`cmd/admin-api/main.go`

职责：

```text
1. 启动 Gin HTTP server
2. 注册 REST API
3. 注册 WebSocket upgrade 路由
4. 执行登录、RBAC、系统管理、AI 管理等同步请求
5. 持有 realtime session manager
6. 当 REALTIME_PUBLISHER=redis 时订阅 Redis Pub/Sub 并投递到本机 WebSocket session
```

默认监听：

```text
HTTP_ADDR=:8080
```

### `admin-worker`

入口：`cmd/admin-worker/main.go`

职责：

```text
1. 消费 Asynq 队列任务
2. 执行导出任务、通知任务、登录日志任务、支付定时任务、AI run timeout 等后台任务
3. 从 DB 中读取启用的 cron_task，注册到 scheduler
4. 使用 Redis 锁避免多 worker 重复执行同一定时任务
```

## 目录结构

```text
admin_back_go/
  cmd/
    admin-api/             # HTTP API 进程入口
    admin-worker/          # 队列 + 定时任务进程入口
  database/
    migrations/            # 当前增量迁移 SQL，不是完整建库脚本
  deploy/
    first-node/            # 单节点/首节点 Docker Compose 部署模板
  docs/                    # 后端架构、模块设计、迁移文档
  internal/
    bootstrap/             # 装配 config/db/redis/router/services/worker
    config/                # 环境变量读取和默认值
    enum/                  # 业务枚举
    dict/                  # 字典输出
    jobs/                  # 版本化队列任务注册
    middleware/            # RequestID/CORS/Auth/Permission/OperationLog
    module/                # 业务模块
    platform/              # DB/Redis/queue/storage/realtime/AI/payment 等基础设施
    readiness/             # /ready 依赖检查
    response/              # 统一响应
    server/                # Gin router 和 middleware 顺序
    validate/              # validator 扩展
    version/               # 版本信息
  runtime/                 # 本地运行日志/证书等运行时目录
  scripts/                 # smoke、contract、证书辅助脚本
  Dockerfile
  go.mod
  .env.example
```

固定调用链：

```text
route -> handler -> service -> repository -> model
```

不要为了“看起来规范”硬造空层。没有数据库就没有 repository；没有表就没有 model。

## 核心能力

当前已迁移/落地的主要能力包括：

```text
health / ready
登录 / refresh / logout / forgot-password
slide captcha
后台用户 / 个人资料 / 登录日志 / 会话管理
RBAC 权限定义 / 角色 / 菜单按钮权限
操作日志
系统设置
系统日志读取
上传配置 / 上传 token
导出任务
通知中心 / 通知任务
cron task 管理
支付基础任务和回调入口
客户端版本管理
AI provider / agent / chat / conversation / messages / runs / tools / knowledge
WebSocket realtime
Queue monitor
```

具体状态以这些文件为准，不要靠 README 猜：

```text
../docs/migration/current-status.md
../docs/contracts/admin-api-v1.md
../docs/contracts/admin-realtime-v1.md
docs/architecture.md
```

## 配置说明

### 配置来源

启动时会先读取 `.env`，再读取系统环境变量：

```go
_ = config.LoadDotEnv()
cfg := config.Load()
```

本地开发默认用：

```text
admin_back_go/.env
```

部署时 Docker Compose 默认用：

```text
/www/docker/admin-go/admin-go.env
```

### 必填核心配置

| 变量 | 说明 |
| --- | --- |
| `APP_ENV` | `local` / `production` 等运行环境 |
| `HTTP_ADDR` | HTTP 监听地址，Docker 内建议 `:8080` |
| `MYSQL_DSN` | 推荐使用的 MySQL DSN |
| `REDIS_ADDR` | Redis 地址 |
| `APP_SECRET` | 应用唯一根密钥，所有 API/Worker 节点必须一致；代码内部派生 JWT、refresh token pepper、secretbox 等用途 key |
| `CORS_ALLOW_ORIGINS` | 允许访问 API 的前端 origin |

### MySQL 配置

推荐直接配置完整 DSN：

```env
MYSQL_DSN=admin_user:CHANGE_ME@tcp(127.0.0.1:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local
MYSQL_MAX_OPEN_CONNS=20
MYSQL_MAX_IDLE_CONNS=10
MYSQL_CONN_MAX_LIFETIME=1h
```

兼容配置也存在，但不推荐新部署继续依赖：

```env
DB_HOST=127.0.0.1
DB_PORT=3306
DB_DATABASE=admin
DB_USERNAME=root
DB_PASSWORD=
```

### Redis 配置

```env
REDIS_ADDR=127.0.0.1:6379
REDIS_PASSWORD=
REDIS_DB=0

TOKEN_REDIS_DB=2
QUEUE_REDIS_DB=3
```

注意：主 Redis、token Redis、queue Redis 是同一 Redis 实例的不同 DB 逻辑隔离。

### Queue / Worker

```env
QUEUE_ENABLED=true
QUEUE_REDIS_DB=3
QUEUE_CONCURRENCY=10
QUEUE_DEFAULT_QUEUE=default
QUEUE_CRITICAL_WEIGHT=6
QUEUE_DEFAULT_WEIGHT=3
QUEUE_LOW_WEIGHT=1
QUEUE_SHUTDOWN_TIMEOUT=10s
QUEUE_DEFAULT_MAX_RETRY=3
QUEUE_DEFAULT_TIMEOUT=30s
```

如果 `QUEUE_ENABLED=true`，必须启动 `admin-worker`，否则导出、通知、部分异步任务只会入队，不会被消费。

### Realtime / WebSocket

```env
REALTIME_ENABLED=true
REALTIME_PUBLISHER=redis
REALTIME_REDIS_CHANNEL=admin_go:realtime:publish
REALTIME_HEARTBEAT_INTERVAL=25s
REALTIME_SEND_BUFFER=16
```

取值：

```text
REALTIME_PUBLISHER=local  # 单 API 进程，本机直接投递
REALTIME_PUBLISHER=noop   # 显式丢弃业务推送，不等于关闭 WebSocket
REALTIME_PUBLISHER=redis  # 多 API 进程推荐，Redis Pub/Sub fan-out
```

生产如果未来有多个 `admin-api`，推荐 `redis`。

### Scheduler

```env
SCHEDULER_ENABLED=true
SCHEDULER_TIMEZONE=Asia/Shanghai
SCHEDULER_LOCK_PREFIX=admin_go:scheduler:
SCHEDULER_LOCK_TTL=30s
```

多 worker 节点时，Redis 锁会降低重复执行风险。但不要无脑在多套环境里同时指向同一个数据库和同一个 Redis。

### 验证码

```env
VERIFY_CODE_TTL=5m
VERIFY_CODE_REDIS_PREFIX=auth:verify_code:
```

规则很简单：手机号验证码固定 `123456`，不接短信，也不受 `.env` 控制；邮箱验证码始终走腾讯云 SES，需要先在邮件管理里启用发信配置和审核通过的模板。生产如果不开放手机号登录，直接在 `auth_platforms.login_types` 里关闭 `phone`。

### CORS

本地开发：

```env
CORS_ALLOW_ORIGINS=http://localhost:5173,http://127.0.0.1:5173,http://localhost:5174,http://127.0.0.1:5174
```

线上演示：

```env
CORS_ALLOW_ORIGINS=https://zgm2003.cn
CORS_ALLOW_CREDENTIALS=true
```

## 本地开发

### 1. 准备依赖

需要：

```text
Go 1.26.1
MySQL
Redis
PowerShell 7 或 Windows PowerShell
```

### 2. 创建 `.env`

```powershell
cd E:/admin_go/admin_back_go
Copy-Item .env.example .env
```

然后至少改这些：

```env
MYSQL_DSN=你的 MySQL DSN
REDIS_ADDR=127.0.0.1:6379
# 至少 64 位随机字符串；修改会让旧登录态和已加密业务密钥失效
APP_SECRET=本地长随机字符串
CORS_ALLOW_ORIGINS=http://localhost:5173,http://127.0.0.1:5173
```

不要把真实 `.env` 提交到 Git。

### 3. 下载依赖

```powershell
go mod download
```

### 4. 启动 API

```powershell
go run ./cmd/admin-api
```

验证：

```powershell
curl.exe http://127.0.0.1:8080/health
curl.exe http://127.0.0.1:8080/ready
curl.exe http://127.0.0.1:8080/api/admin/v1/auth/login-config
```

### 5. 启动 Worker

另开一个终端：

```powershell
cd E:/admin_go/admin_back_go
go run ./cmd/admin-worker
```

### 6. 常用本地检查

```powershell
# 单元测试
go test ./...

# 合同检查
powershell -ExecutionPolicy Bypass -File ./scripts/check-contract.ps1

# 基础 smoke，需要传真实测试账号
powershell -ExecutionPolicy Bypass -File ./scripts/basic-admin-smoke.ps1 -Account <account> -Password <password>

# 完整 smoke，覆盖更多读写链路
powershell -ExecutionPolicy Bypass -File ./scripts/full-admin-smoke.ps1 -Account <account> -Password <password>
```

## 数据库和迁移

这点别搞错：

```text
database/migrations 现在是增量迁移，不是完整从 0 建库脚本。
```

首次部署需要有一份可用的 `admin` 数据库基线。来源可以是：

```text
1. 当前已整理好的线上/演示库备份
2. legacy admin 库迁移后的基线库
3. 后续补齐的完整 schema dump
```

然后再按顺序执行：

```text
database/migrations/*.sql
```

生产执行迁移前必须：

```text
1. 备份数据库
2. 确认 SQL 是否 destructive
3. 在测试库跑一遍
4. 再上生产
```

## Docker 部署

当前推荐演示部署：

```text
宿主机宝塔 / OpenResty / Nginx 负责域名、HTTPS、反向代理
Docker Compose 只跑 admin-api 和 admin-worker
MySQL / Redis 可以同机，也可以独立状态机
```

### 1. 服务器目录建议

```bash
mkdir -p /www/project
mkdir -p /www/docker/admin-go/runtime/logs
mkdir -p /www/docker/admin-go/runtime/cert/alipay
mkdir -p /www/docker/admin-go/exports
mkdir -p /www/backup/admin-go
```

推荐代码目录：

```bash
/www/project/admin_back_go
```

如果你已经把后端代码放在宝塔站点目录，也可以用：

```bash
/www/wwwroot/www.zgm2003.cn
```

关键是 Docker Compose 里的 `ADMIN_BACK_GO_DIR` 必须指向真实 Go 后端代码目录。

### 2. 拉代码

```bash
cd /www/project
git clone <your-repo-url> admin_back_go
cd /www/project/admin_back_go
```

已有目录则：

```bash
cd /www/project/admin_back_go
git pull
```

### 3. 准备 Compose 工作目录

```bash
cp /www/project/admin_back_go/deploy/first-node/docker-compose.yml /www/docker/admin-go/docker-compose.yml
cp /www/project/admin_back_go/deploy/first-node/admin-go.env.example /www/docker/admin-go/admin-go.env
```

创建 `/www/docker/admin-go/.env`，这是给 Docker Compose 自己读取的变量，不是容器业务配置：

```env
ADMIN_BACK_GO_DIR=/www/project/admin_back_go
ADMIN_GO_ENV_FILE=/www/docker/admin-go/admin-go.env
ADMIN_GO_RUNTIME_DIR=/www/docker/admin-go/runtime
ADMIN_GO_EXPORTS_DIR=/www/docker/admin-go/exports
ADMIN_API_HOST_BIND=127.0.0.1
ADMIN_API_HOST_PORT=8080
```

如果代码在 `/www/wwwroot/www.zgm2003.cn`，就改成：

```env
ADMIN_BACK_GO_DIR=/www/wwwroot/www.zgm2003.cn
```

### 4. 修改业务环境变量

编辑：

```bash
vim /www/docker/admin-go/admin-go.env
```

最少要改：

```env
APP_ENV=production
HTTP_ADDR=:8080

MYSQL_DSN=admin_user:CHANGE_ME@tcp(127.0.0.1:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local
REDIS_ADDR=127.0.0.1:6379
REDIS_PASSWORD=

# 所有 admin-api/admin-worker 节点必须一致；至少 64 位随机字符串
APP_SECRET=CHANGE_ME_AT_LEAST_64_RANDOM_CHARS

QUEUE_ENABLED=true
REALTIME_ENABLED=true
REALTIME_PUBLISHER=redis
SCHEDULER_ENABLED=true

CORS_ALLOW_ORIGINS=https://zgm2003.cn
CORS_ALLOW_CREDENTIALS=true
```

如果 MySQL/Redis 在另一台机器，把 `127.0.0.1` 改成内网 IP。别把 MySQL/Redis 裸奔到公网；必须用安全组/防火墙只放行后端机器。

### 5. 启动

```bash
cd /www/docker/admin-go
docker compose up -d --build
```

查看状态：

```bash
docker compose ps
docker compose logs -f admin-api
docker compose logs -f admin-worker
```

验证宿主机本地端口：

```bash
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
curl -fsS http://127.0.0.1:8080/api/admin/v1/auth/login-config
```

### 6. 更新部署

更新时分清两个目录：

```text
代码目录：负责 git pull
Compose 目录：负责 docker compose up -d --build
```

如果后端代码放在推荐目录：

```bash
cd /www/project/admin_back_go
git pull

cd /www/docker/admin-go
docker compose up -d --build
```

如果你把后端代码放在宝塔站点目录，也就是当前单体演示常用方式：

```bash
cd /www/wwwroot/www.zgm2003.cn
git pull

cd /www/docker/admin-go
docker compose up -d --build
```

重点是：`docker compose up -d --build` 在 `/www/docker/admin-go` 执行，因为这里才有 `docker-compose.yml`；Compose 会通过 `ADMIN_BACK_GO_DIR` 去读取真正的 Go 后端代码和 `Dockerfile`。

### 7. 停止 / 重启

```bash
cd /www/docker/admin-go

# 重启
docker compose restart

# 停止
docker compose down

# 重新构建并启动
docker compose up -d --build
```

## 宝塔 / Nginx 反向代理

### 原则

```text
Docker 只跑 Go 后端。
宝塔 Nginx 负责域名、SSL、反向代理、前端 SPA 伪静态。
```

后端站点建议：

```text
www.zgm2003.cn  -> http://127.0.0.1:8080
```

前端站点建议：

```text
zgm2003.cn      -> /www/wwwroot/zgm2003.cn 静态 dist
```

### 后端站点 `www.zgm2003.cn`

宝塔路径：

```text
宝塔 -> 网站 -> www.zgm2003.cn -> 设置 -> 配置文件
```

核心反代：

```nginx
location ^~ / {
    proxy_pass http://127.0.0.1:8080;

    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Request-Id $request_id;

    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $connection_upgrade;

    proxy_connect_timeout 30s;
    proxy_send_timeout 3600s;
    proxy_read_timeout 3600s;
}
```

如果宝塔已经生成了 `location ^~ /`，不要再新建第二个 `location /`；直接改已有块。

### 前端站点 `zgm2003.cn` 的 SPA 伪静态

前端 Vue/Vite 刷新页面需要：

```nginx
location / {
    try_files $uri $uri/ /index.html;
}
```

这个只解决前端路由刷新 404，不解决 WebSocket 认证。

### 前端同域 WebSocket 反代

浏览器原生 WebSocket 不能稳定附带自定义 `Authorization` header，本项目生产 WebSocket 依赖路径限定 cookie token。为了避免 `zgm2003.cn` 页面连 `www.zgm2003.cn` 时 cookie 丢失，生产推荐让 WebSocket 走前端同域：

```text
wss://zgm2003.cn/api/admin/v1/realtime/ws
```

所以前端站点 `zgm2003.cn` 还需要加一条精确反代，放在 `location /` 前面：

```nginx
location = /api/admin/v1/realtime/ws {
    proxy_pass http://127.0.0.1:8080;

    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";

    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

检查并重载 Nginx：

```bash
/www/server/nginx/sbin/nginx -t
/www/server/nginx/sbin/nginx -s reload
```

## 前端联动配置

前端项目在：

```text
../admin_front_ts
```

生产环境建议：

```env
VITE_GO_API_BASE_URL=https://www.zgm2003.cn
VITE_WEB_SOCKET_URL=wss://zgm2003.cn/api/admin/v1/realtime/ws
VITE_PLATFORM=admin
```

解释：

```text
普通 REST API 走 www.zgm2003.cn 后端域名。
WebSocket 走 zgm2003.cn 前端同域，避免浏览器 cookie 跨子域丢失导致 401。
```

前端 GitHub Actions 当前会把 `dist` 上传到：

```text
/www/wwwroot/zgm2003.cn
```

## 验证和运维

### 健康检查

```bash
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
curl -fsS https://www.zgm2003.cn/health
curl -fsS https://www.zgm2003.cn/ready
```

`/health` 只表示进程活着。

`/ready` 会检查：

```text
database
redis
token_redis
queue_redis
realtime
```

### 日志

Docker stdout：

```bash
cd /www/docker/admin-go
docker compose logs -f admin-api
docker compose logs -f admin-worker
```

文件日志：

```text
/www/docker/admin-go/runtime/logs/admin-api.log
/www/docker/admin-go/runtime/logs/admin-worker.log
```

宝塔 Nginx 日志一般在：

```text
/www/wwwlogs/www.zgm2003.cn.log
/www/wwwlogs/www.zgm2003.cn.error.log
/www/wwwlogs/zgm2003.cn.log
/www/wwwlogs/zgm2003.cn.error.log
```

### Smoke

在本机或服务器代码目录执行：

```powershell
cd E:/admin_go/admin_back_go
powershell -ExecutionPolicy Bypass -File ./scripts/basic-admin-smoke.ps1 -Account <account> -Password <password>
powershell -ExecutionPolicy Bypass -File ./scripts/full-admin-smoke.ps1 -Account <account> -Password <password>
```

如果在 Linux 服务器上跑这些 PowerShell 脚本，需要安装 `pwsh`；否则可以先用 `curl` 验证 `/health`、`/ready` 和登录配置。

## 多节点部署注意事项

演示环境推荐单机，简单、可控、少踩坑。未来要拆多节点时，至少遵守这些规则：

```text
1. 所有 admin-api / admin-worker 节点必须使用同一套 MySQL/Redis。
2. 所有节点的 APP_SECRET 必须一致，否则 access/refresh token、Redis session cache、AI/upload/payment 已加密 secret 都会失效。
3. 变更 APP_SECRET 前先按 `E:/admin_go/docs/deployment/auth-foundation-v2-reset-runbook.md` 撤销会话、清 Redis token cache，并重新录入业务密钥。
4. REALTIME_PUBLISHER 多 API 节点建议使用 redis。
5. 支付证书、运行时 cert 目录必须部署到需要处理支付的后端节点。
6. SCHEDULER_ENABLED 不要在多套独立环境里同时指向同一库；同一集群内依靠 Redis lock，但仍要监控重复执行。
7. MySQL/Redis 放独立机器时优先走内网 IP，公网必须安全组白名单。
8. 8080 只绑定 127.0.0.1，由 Nginx 对外暴露 80/443。
```

一个合理的三机/四机演进方向：

```text
A: 前端静态站 + Nginx + admin-api/admin-worker
B: admin-api/admin-worker
C: MySQL + Redis
D: 备用后端或后续对象存储/监控，不要为了“分布式”硬拆
```

## 常见问题

### 1. `GET /ready` 返回 not ready

先看 `data.checks` 哪项失败：

```text
database    MySQL DSN、账号、库名、网络、防火墙
redis       REDIS_ADDR、密码、网络
queue_redis QUEUE_ENABLED=true 时 Redis 必须可用
token_redis TOKEN_REDIS_DB 对应 Redis 必须可用
realtime    REALTIME_ENABLED/REALTIME_PUBLISHER 配置错误
```

### 2. 前端普通接口能登录，但 WebSocket 401

这通常不是后端“白名单”问题，而是 WebSocket 这条链路没带到 cookie。

推荐配置：

```env
VITE_WEB_SOCKET_URL=wss://zgm2003.cn/api/admin/v1/realtime/ws
```

并在 `zgm2003.cn` 前端站加精确反代：

```nginx
location = /api/admin/v1/realtime/ws { ... proxy_pass http://127.0.0.1:8080; ... }
```

### 3. 前端刷新 `/login` 或后台页面 404

这是前端 SPA 伪静态问题，不是 Go 后端问题。前端站点加：

```nginx
location / {
    try_files $uri $uri/ /index.html;
}
```

### 4. 导出任务一直 pending

检查：

```bash
docker compose ps
docker compose logs -f admin-worker
```

`admin-worker` 没跑，队列任务就不会消费。

### 5. 手机号验证码总是固定 `123456`

这是当前业务规则，不是配置遗漏：手机号短信未接入，手机号验证码固定 `123456`，不受 env 控制。邮箱验证码才走腾讯云 SES；生产如果不开放手机号登录，去 `auth_platforms.login_types` 关闭 `phone`。

### 6. 不要把 Nginx 配置放进 Docker

这个项目的 Docker 镜像只包含：

```text
/app/admin-api
/app/admin-worker
/app/runtime
/app/exports
```

Nginx、SSL、伪静态、反代都在宿主机宝塔里。

## 重要文档

```text
../AGENTS.md
../docs/architecture/00-open-source-first.md
../docs/architecture/01-step-by-step-roadmap.md
../docs/architecture/04-go-backend-framework.md
../docs/architecture/05-development-quality-rules.md
../docs/migration/current-status.md
../docs/contracts/admin-api-v1.md
../docs/contracts/admin-realtime-v1.md
docs/architecture.md
internal/middleware/README.md
deploy/first-node/docker-compose.yml
deploy/first-node/admin-go.env.example
```

## 底线

```text
不要提交真实 .env。
不要让 8080 直接暴露公网。
不要把 database/migrations 当完整建库脚本。
不要在 README 里承诺未实现能力。
不要为了“分布式”增加复杂度；能单机稳定演示，就先单机。
```
