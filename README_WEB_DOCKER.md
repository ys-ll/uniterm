# uniTerm Web（Docker 多用户版）

把桌面端 uniTerm 无头化（Wails v3 `-tags server`）后放在浏览器里用，并按用户隔离。
本仓库已经内置全部改动，直接构建即可，无需再打补丁。

## 组成

| 路径 | 作用 |
| --- | --- |
| `cmd/uniterm-gateway/main.go` | 登录页、用户管理、按用户拉起独立 `uniterm-server` 进程并反向代理 |
| `server_web_extension.go` | `-tags server` 下的 `/api/web/*`：浏览器流式 SFTP 上传/下载、目录打包下载、本地文件暂存 |
| `server_keychain.go` | 容器内没有 D-Bus，用文件版 keychain 替代系统 keychain |
| `server_desktop_stub.go` | 桌面构建的空实现（`BuildServerOptions`、`credentialKeychain` 等） |
| `backend/session/sftp_web_stream.go` | 把 `SFTPSession` 适配成 `io.Reader`/`io.Writer` 流 |
| `frontend/src/components/WebConsoleOverlay.vue` | 浏览器右下角的 Web 控制台浮窗（仅 server 模式显示） |
| `frontend/src/services/webSftpBridge.ts` | 前端调用 `/api/web/*` 的桥接层 |

## 构建与启动

```bash
# 首次启动必须设置管理员初始密码；留空会生成随机密码并打印在 docker logs 里
export UNITERM_ADMIN_PASSWORD='YourStrongPassword_2026!'
docker compose up -d --build

# 浏览器打开 http://localhost:8080
```

默认数据目录是仓库下的 `./uniterm-data`。升级已有部署时，用 `UNITERM_DATA_HOST`
指向旧数据目录即可保留账号与用户数据：

```bash
export UNITERM_DATA_HOST=/path/to/existing/uniterm-data
docker compose up -d --force-recreate
```

数据全部落在该目录：`gateway/users.json`、`gateway/jwt_secret.key`，
以及每个用户独立的 `users/<name>/{home,config,data,cache}`（`HOME`、`XDG_*` 都指向这里）。

## 环境变量

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `UNITERM_DATA_DIR` | `/data` | 数据根目录 |
| `UNITERM_GATEWAY_ADDR` | `0.0.0.0:8080` | 网关监听地址 |
| `UNITERM_SERVER_BIN` | `/app/uniterm-server` | 每个用户的 worker 二进制 |
| `UNITERM_ADMIN_USER` | `admin` | 初始管理员用户名 |
| `UNITERM_ADMIN_PASSWORD` | 随机 | 初始管理员密码（仅首次启动写入 `users.json`） |
| `UNITERM_WORKER_IDLE_MINUTES` | `60` | 用户空闲多久后回收 worker 进程 |
| `UNITERM_ALLOWED_ORIGINS` | 空 | 反向代理部署时允许的 WebSocket 来源，逗号分隔 |

## 用户管理 API（需要 admin 会话 Cookie）

```bash
# 登录并保存 cookie
curl -c cookie.txt -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"YourStrongPassword_2026!"}'

# 查看 / 新建 / 删除
curl -b cookie.txt http://localhost:8080/api/auth/users
curl -b cookie.txt -X POST http://localhost:8080/api/auth/users \
  -H 'Content-Type: application/json' \
  -d '{"username":"dev_alice","password":"AlicePassword123","role":"user"}'
curl -b cookie.txt -X DELETE http://localhost:8080/api/auth/users/dev_alice
```

## 浏览器流式文件传输

| 接口 | 方法 | 说明 |
| --- | --- | --- |
| `/api/web/mode` | GET | 是否 server 模式、当前用户名/角色/家目录 |
| `/api/web/sftp/download?sessionId=&path=` | GET | 远程文件直接流式下载（64 KiB 分块，不落盘） |
| `/api/web/sftp/download-archive?sessionId=&paths=[...]` | GET | 多个文件/目录实时打包 `.tar.gz` 流 |
| `/api/web/sftp/upload?sessionId=&remoteDir=` | POST | multipart 流式上传，支持拖拽整个文件夹 |
| `/api/web/sftp/cancel?transferId=` | POST | 中断传输 |
| `/api/web/dialog/stage-file` | POST | 浏览器选文件后暂存到容器 `$HOME/.cache/uniterm-web-staging/` |

`sessionId` 是当前活动的 SFTP 会话 ID。当前只有 SFTP 会话支持浏览器流式传输；
SCP/FTP 仍走桌面端原有的传输管线。

## 桌面端不受影响

`serverMode` 是编译期常量：普通 `wails3 build` / `wails3 dev` 走原有桌面逻辑，
`-tags server` 才注册 `/api/web` 路由并跳过系统通知服务。