# 安装与首次连接

本指南将帮助您下载安装 uniTerm 并建立第一个连接。


## 下载与安装

从 [GitHub Releases](https://github.com/ys-ll/uniterm/releases/latest) 或 [Gitee Releases](https://gitee.com/ys-l/uniterm/releases) 下载对应平台的安装包：

### Windows

- **安装版** — 双击运行 `uniterm-windows-amd64-installer-<版本>.exe`，按提示完成安装（另有 arm64 安装包）
- **便携版** — 下载 `uniterm-windows-amd64-portable-<版本>.zip`，解压到任意目录后运行其中的 `uniterm.exe`，无需安装

### macOS

- 根据芯片选择安装包：Apple Silicon（M 系列）选 `arm64`，Intel 选 `amd64`
- 打开 `.dmg` 镜像，将 uniTerm 拖入 Applications 文件夹，然后从启动台或应用程序目录运行

### Linux

- **.deb（Debian / Ubuntu 等）** — `sudo dpkg -i uniterm-linux-amd64-<版本>.deb` 或双击安装，安装后可从应用菜单启动
- **.rpm（Fedora / openSUSE 等）** — `sudo rpm -i uniterm-linux-amd64-<版本>.rpm` 或用包管理器安装，安装后可从应用菜单启动
- **.tar.gz（通用）** — 解压后直接运行其中的 `uniterm`：

  ```bash
  tar xzf uniterm-linux-amd64-<版本>.tar.gz
  ./uniterm
  ```

### Android

- 下载 `uniterm-android-arm64-<版本>.apk`，在手机上直接安装

### Docker（Web 多用户模式）

除桌面客户端外，uniTerm 还提供 Docker 部署的 Web 多用户模式：部署为浏览器可访问的服务，每位用户拥有隔离的工作区（各自的连接、会话、密钥与数据）。

```bash
# 拉取镜像
docker pull uniterm/uniterm:latest

# 或使用 docker compose（推荐，compose 文件见仓库 deploy/docker/ 目录）
export UNITERM_ADMIN_PASSWORD='你的管理员密码'
docker compose up -d
```

- 启动后浏览器打开 `http://<服务器IP>:8080`，使用管理员账号登录
- 首次启动若未设置 `UNITERM_ADMIN_PASSWORD`，会自动生成随机密码并打印在容器日志中
- 离线导入镜像、自建镜像等更多部署方式见 [Docker 部署指南](https://github.com/ys-ll/uniterm/blob/main/deploy/docker/README_zh-CN.md)

### 运行依赖

- **Windows**：WebView2 运行时（Windows 10+ 已内置，更老的系统需安装）
- **macOS**：无需额外依赖（使用系统自带 WebKit）
- **Linux**：`libgtk-3-0` 与 `libwebkit2gtk-4.1-0`（多数桌面发行版已自带）
- **Android**：Android 5.0+（使用系统自带 WebView，无需额外依赖）


## 创建第一个连接

1. 打开 uniTerm，点击左侧边栏的 **+** 按钮（「新建连接」），或在开始页搜索框输入主机地址后选择「快速连接」。

   ![新建连接](/imgs/new_connection_light.webp)

2. 在弹出的新建连接对话框中，选择协议类型（例如 **SSH**），填写连接信息：
   - **名称**：给连接起一个易识别的名字
   - **主机**：服务器 IP 或域名
   - **端口**：协议默认端口会自动填充
   - **用户名**：登录用户名
   - **密码**：登录密码，其他认证方式详见[终端](/zh/connections/terminal)

3. 点击 **仅连接**（打开但不保存）或 **保存并连接**，保存后连接将出现在左侧列表中。

4. 双击连接，即可打开终端/会话。


## 界面概览

uniTerm 的主界面主要分为以下区域：

- **左侧边栏** — 连接列表、文件、监控、隧道、快捷命令等标签页，详见[边栏](/zh/features/sidebar)
- **中央终端区** — 终端 Tab 页，支持拖拽分栏组成工作区
- **开始页** — 启动后的快速访问入口，支持搜索、收藏与最近连接


## 下一步

- 查看 [开始页](/zh/start-page) 了解开始页的使用方法
- 查看 [支持协议](/zh/protocols) 了解全部连接类型
- 查看 [终端](/zh/connections/terminal) 了解 SSH/Telnet/Mosh 的详细用法
- 查看 [AI 助理](/zh/features/ai-assistant) 配置 AI Agent
- 查看 [个性化](/zh/features/personalization) 调整主题、界面字号、快捷键和语言
- 查看 [常见问题](/zh/faq) 排查安装与连接问题
