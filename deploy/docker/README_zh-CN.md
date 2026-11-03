# uniTerm Web（Docker 多用户部署）

把 uniTerm 部署为浏览器可访问的多用户服务：每个用户登录后获得独立的隔离工作区
（独立的连接配置、SSH 会话、密钥与数据），互不可见。

## 获取镜像

三种方式任选其一。

### Docker Hub

```bash
docker pull uniterm/uniterm:latest
```

### 离线导入镜像

内网/离线环境可以从 Release 页下载 `uniterm-docker-<架构>-<版本>.tar.gz`
（amd64 / arm64 按需选择）：

```bash
docker load -i uniterm-docker-amd64-<版本>.tar.gz
```

### 自己构建镜像

在仓库根目录执行：

```bash
docker build -t uniterm/uniterm:latest .
```

## 启动容器

推荐使用 compose（把 `deploy/docker/docker-compose.yml` 复制到你的部署目录，在该目录执行）：

```bash
# 设置管理员初始密码（仅首次启动生效；留空则自动生成随机密码并打印在容器日志里）
export UNITERM_ADMIN_PASSWORD='YourStrongPassword_2026!'

docker compose up -d
```

或者用 `docker run`：

```bash
docker run -d --name uniterm-web \
  -p 8080:8080 \
  -v uniterm-data:/data \
  -e UNITERM_ADMIN_PASSWORD='YourStrongPassword_2026!' \
  --restart unless-stopped \
  uniterm/uniterm:latest
```

浏览器打开 `http://localhost:8080`，用管理员账号登录。

## 数据持久化

所有数据（账号、每个用户的配置、密钥与个人数据）都在数据目录中，容器升级/重建
只要把数据目录挂回去即可保留：

- compose 部署：默认落在 `deploy/docker/uniterm-data/`，可通过 `UNITERM_DATA_HOST`
  指向其他位置；
- `docker run` 部署：通过 `-v <宿主机目录>:/data` 指定。

## 环境变量

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `UNITERM_ADMIN_PASSWORD` | 随机 | 初始管理员密码（仅首次启动生效，之后改密码请登录后在界面操作） |
| `UNITERM_ADMIN_USER` | `admin` | 初始管理员用户名 |
| `UNITERM_DATA_HOST` | `./uniterm-data` | 宿主机数据目录 |
| `UNITERM_WORKER_IDLE_MINUTES` | `60` | 用户空闲多久后回收工作区（分钟），回收后下次访问自动恢复 |
| `UNITERM_COOKIE_SECURE` | `false` | 通过 HTTPS 反代部署时置 `true` |
| `UNITERM_ALLOWED_ORIGINS` | 空 | 通过反向代理部署时的公网访问来源（逗号分隔） |

## 用户管理（管理员）

登录后点击右上角头像菜单进入"用户管理"：

- **新增用户**：设置用户名和初始密码（至少 6 位）。
- **重置密码**：为用户设置新密码。注意：登录密码同时是该用户数据的加密主密码，
  重置密码会**清空该用户的全部密钥数据**，工作区从空白重新开始；用户本人通过
  头像菜单"修改密码"则不受影响（数据平滑迁移）。
- **删除用户**：删除账号并同时清除其全部数据。

## 升级

1. 备份数据目录（compose 部署默认是 `deploy/docker/uniterm-data/`）。
2. 获取新镜像（pull / 离线导入 / 重新构建均可），重建容器，数据目录挂载保持不变。

## 安全建议

- **公网部署务必通过 HTTPS 反向代理**，并设置 `UNITERM_COOKIE_SECURE=true`。
- 登录接口有防暴力破解：连续失败 5 次锁定 10 分钟。
- 容器本地终端不可用：本地 shell 等于容器 root 权限，属于多用户部署的安全设计。
