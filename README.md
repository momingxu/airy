# Airy

一个使用 Go、Gin、Vue 3、GORM、PostgreSQL/SQLite 构建的相册 Web 应用。

当前已实现功能、业务边界和验收场景见
[Airy 功能需求说明书](REQUIREMENTS.md)。

## 目录结构

```text
airy/
├── cmd/       # Go 服务入口
├── internal/  # 后端业务代码
├── tools/     # 本地 ffmpeg、证书等工具（不提交）
└── web/       # Vue 3 + Vite 前端
```

## 本地开发

### 启动后端

需要 Go 1.22 或更高版本。Go 服务始终内嵌前端，因此首次启动以及每次修改前端后，
先生成供 Go 编译的静态文件：

```bash
cd web
npm ci --no-audit --no-fund
npm run build -- --outDir ../cmd/server/web --emptyOutDir
cd ..
```

未配置任何数据库环境变量时，直接运行 `go run ./cmd/server` 默认使用当前目录的
`airy.db`（SQLite）。Docker 中默认为 `/data/airy.db`，请挂载 `/data` 持久化。

显式的 `DB_DRIVER` 优先；未填写时，已有 `DB_HOST`、`DB_PASSWORD`、`DB_DSN` 等
数据库配置继续按 PostgreSQL 处理。自定义 SQLite 连接字符串需设置 `DB_DRIVER=sqlite`。

也可以使用 PostgreSQL：

```bash
DB_PASSWORD=postgres go run ./cmd/server
```

SQLite 示例：

```bash
DB_DRIVER=sqlite \
DB_DSN='file:airy.db?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)' \
go run ./cmd/server
```

后端默认监听 `http://localhost:8096`，健康检查地址为
`http://localhost:8096/api/health`。启动时会自动创建或更新所需数据表。所有运行方式均
直接提供编译时内嵌的前端；修改页面后需要重新构建前端并重新编译、重启 Go 服务。

首次打开网页时需要创建唯一的管理员账户，之后必须登录才能访问图库、媒体、扫描和
管理接口。登录会话默认保持 30 天；用户名和密码可以在“设置”页面修改。

可通过环境变量 `PORT` 修改监听端口：

```bash
PORT=9000 go run ./cmd/server
```

### HTTPS

Go 服务可以直接提供 HTTPS。在根目录 `.env` 中同时配置证书链和私钥：

```dotenv
PORT=8443
TLS_CERT_FILE=/etc/airy/server.crt
TLS_KEY_FILE=/etc/airy/server.key
```

配置后，`PORT` 会同时识别 HTTP 与 HTTPS。访问 `http://localhost:8443` 时，服务会
返回 `308 Permanent Redirect`，并保留域名、端口、路径和查询参数跳转到对应的
`https://localhost:8443`；未配置这两项时仍使用纯 HTTP。证书必须包含浏览器实际访问
的域名或 IP，且证书链需要受客户端信任。
`TLS_CERT_FILE` 和 `TLS_KEY_FILE` 必须同时配置，否则服务会拒绝启动。也可以不设置这
两项，继续由 Nginx、Caddy 等反向代理终止 TLS。

可以直接提供完整连接字符串：

```bash
DB_DSN="host=localhost port=5432 user=postgres password=postgres dbname=airy sslmode=disable" \
go run ./cmd/server
```

也可以分别设置 `DB_HOST`、`DB_PORT`、`DB_USER`、`DB_PASSWORD`、`DB_NAME`
和 `DB_SSLMODE`。完整配置示例见根目录 `.env.example`。

程序会自动读取根目录 `.env`；该文件存在时，其中的配置优先。

### 扫描照片目录

在“资源”页面添加服务器上的资源库目录。扫描工具路径可通过环境变量配置：

```dotenv
FFMPEG_PATH=/absolute/path/to/ffmpeg
FFPROBE_PATH=/absolute/path/to/ffprobe
```

资源库默认将一级子目录作为相册，也可以在资源页面为每个资源库独立设置 1–10 级
相册目录深度。相册内部可以包含图片或视频；
不支持的文件会被忽略。扫描器不会修改、移动或删除原文件，但会在每个相册内
写入缩略图目录：

目录监听只注册到相册层级的父目录，并且只处理新建文件夹事件；文件新增、写入、删除
和重命名不会触发扫描。发现新相册目录后等待 5 分钟，再扫描对应资源库。

```text
相册目录/
└── thumbnails/    # 高度固定为 400px 的 WebP 缩略图
```

扫描器会跳过该目录，不会把生成文件再次收录为照片。只有缩略图缺失或原
文件发生变化时才会重新生成。

每次扫描先读取资源根目录并确定全部相册，然后逐个相册完成媒体信息扫描和缩略图
生成。当前相册完成后会立即对图库可见，再继续处理下一个相册；尚未轮到的已有相册
保持之前的可访问状态。

支持的图片格式：JPEG、PNG、WebP、GIF。支持的视频容器：MP4、M4V、MOV、
WebM、MKV、AVI。视频尺寸和时长通过 `ffprobe` 读取；缺少 `ffprobe` 时仍会登记
视频文件，但对应元数据为空。

图片和视频的缩略图在目录扫描时使用 `ffmpeg` 生成，格式统一为 WebP。图片预览
直接读取原文件。仓库忽略根目录 `tools/`，可以把下载的静态 `ffmpeg` 和 `ffprobe`
放在该目录，并在 `.env` 中填写绝对路径。

扫描接口：

```bash
# 启动一次后台扫描
curl -X POST http://localhost:8096/api/scan

# 查看扫描进度和结果
curl http://localhost:8096/api/scan
```

前端“资源”页面支持添加多个服务器资源目录，并可分别手动扫描或移除。移除资源
文件夹只会清理 Airy 数据库中的相簿与媒体记录，不会删除目录中的原图、视频或
缩略图。

每个资源文件夹默认监听相册父层级的新建目录事件，发现新相册目录后延迟加入扫描队列；
生成的 `thumbnails` 目录不会触发监听扫描。
可为资源文件夹开启“关闭监听和扫描”，将其排除在监听、手动全局扫描和夜间定时扫描之外；
该资源文件夹仍可单独手动扫描。所有手动、监听和定时任务都使用同一个串行
队列，避免多个目录同时生成缩略图造成磁盘压力。运行中的扫描可以取消，取消还会
清空尚未执行的队列。

资源文件夹接口：

```text
GET    /api/directories              # 列出目录及相簿、媒体统计
POST   /api/directories              # 添加目录，JSON: {"path":"/absolute/path"}
PATCH  /api/directories/:id/settings # 设置是否排除监听和全局扫描
POST   /api/directories/:id/scan     # 扫描指定目录
DELETE /api/directories/:id          # 移除目录的数据库记录
POST   /api/scan/all                  # 将所有未排除的资源文件夹加入扫描队列
POST   /api/scan/cancel               # 取消当前扫描并清空队列
GET    /api/scan/schedule             # 读取全局定时扫描设置
PATCH  /api/scan/schedule             # 修改全局定时扫描设置
```

设置示例。

关闭资源文件夹的监听和全局扫描：

```json
{"automation_disabled": true}
```

全局定时扫描：

```json
{"enabled": true}
```

前端使用的读取接口：

```text
GET /api/gallery                       # 每批默认 10 个相册，包含每册全部媒体
GET /api/albums/:id/media              # 分页读取单个相册的照片/视频
GET /api/media/:id/thumbnail           # 读取 WebP 缩略图
GET /api/media/:id/original            # 读取原文件（支持 Range 请求）
```

模特与系列均以相册为单位组织内容。模特继续使用兼容接口 `/api/people`；系列接口如下：

一个系列可以包含多个相册，但一个相册最多属于一个系列；重新关联时会替换该相册原有的系列。模特头像和系列封面保存为媒体 ID，列表接口直接读取该关联，不会在每次请求时重新遍历关联相册。

```text
GET    /api/series                            # 系列列表
POST   /api/series                            # 创建系列并批量关联相册
PATCH  /api/series/:id                        # 修改名称或收藏状态
DELETE /api/series/:id                        # 删除系列（不删除相册和媒体）
GET    /api/series/:id/gallery                # 系列相册页
GET    /api/series/:id/album-options          # 搜索可关联相册
PUT    /api/series/:id/albums/:album_id       # 关联相册，并替换其原系列
DELETE /api/series/:id/albums/:album_id       # 取消关联
```

重复扫描不会产生重复记录。磁盘中消失的媒体会被标记为 `missing`，不再出现在图库中。
文件重新出现时，missing 记录会恢复为 active。

### 启动前端

需要 Node.js 20 或更高版本。

```bash
cd web
npm install
npm run dev
```

前端默认运行在 `http://localhost:5173`。开发服务器会把 `/api` 请求代理到
`http://localhost:8096`。

生产构建包含 PWA Manifest 和自动更新的 Service Worker，可在支持的浏览器中
安装到桌面或主屏幕。离线缓存只包含页面外壳、CSS、JavaScript 和应用图标；
`/api` 下的图库数据、缩略图、预览图和原文件始终通过网络读取，不占用 PWA
运行时缓存。

照片流使用按行虚拟列表：PC 端先按宽高比生成接近 290px 的齐行布局，手机端
在同一相册接下来的五张内允许小幅调整展示顺序：优先组合三张竖图、两张横图或一竖一横；方图按横图参与两张一行的组合，末尾无法配齐时保留剩余图片，不修改相册数据中的顺序。缩略图按原比例
组成等高行，不裁成正方形；末行不足时保留空位。仅渲染当前视口及少量缓冲行。相簿包含大量照片时不会一次
创建全部图片 DOM 节点。

生产构建：

```bash
cd web
npm ci --no-audit --no-fund
npm run build -- --outDir ../cmd/server/web --emptyOutDir
cd ..
go build -trimpath -o bin/airy ./cmd/server
./bin/airy
```

`cmd/server/web` 是构建时生成目录，已被 Git 忽略。生成的 `bin/airy` 已内嵌前端，
不依赖外部 `web/dist`。修改前端后，重新执行上述前端构建和 Go 编译并重启服务即可。
启动后直接访问 `http://localhost:8096`。

## Docker

项目根目录提供多阶段 `Dockerfile`。Node 和 Go 编译环境不会进入最终镜像；前端会
内嵌到 Go 程序，最终层仅包含单个 Airy 可执行文件、CA 证书、时区数据以及扫描媒体
所需的 `ffmpeg/ffprobe`。

仓库提供 [`compose.yaml`](compose.yaml)，当前配置使用外部 PostgreSQL 和 HTTPS，
以 UID/GID `1000:1001` 运行。部署前按实际环境修改数据库地址、数据库名、端口映射、
证书目录和照片目录，并创建被 Git 忽略的 `.env.compose`：

```dotenv
AIRY_POSTGRES_PASSWORD=填写实际数据库密码
```

```bash
docker compose --env-file .env.compose pull
docker compose --env-file .env.compose up -d
```

访问 `https://服务器IP:8096`，首次访问创建管理员账户。证书目录需包含
`server.crt` 和 `server.key`，且证书必须适用于访问地址。挂载目录必须预先存在，
运行用户需能读取证书、原图，并在相册目录创建缩略图。
数据库保存在外部 PostgreSQL，Compose 不创建数据库服务或 SQLite 数据卷。

如果宿主机已有服务占用 8096，请修改 `ports` 的宿主机端口。多个资源库可通过增加
指向 `/mnt/<资源库名称>` 的挂载实现。开发环境 `.env` 不会注入容器。

```bash
# 查看日志
docker compose --env-file .env.compose logs -f airy
# 更新镜像与容器
docker compose --env-file .env.compose pull
docker compose --env-file .env.compose up -d
```

```bash
docker build -t airy:latest .
```

### GitHub Actions 发布镜像

支持在 GitHub 在线构建，无需本地 Docker。将 `Dockerfile` 和工作流随源码提交到仓库后，
进入 **Actions → Build Docker image and programs → Run workflow**，选择分支并运行。
手动入口需要工作流先存在于默认分支。`main` 分支发布 `latest`；其他分支手动构建使用
分支名标签，避免覆盖正式镜像。构建日志和镜像摘要可在 Actions 运行记录中查看。

工作流 `.github/workflows/container.yml` 会构建 `linux/amd64` 和 `linux/arm64`
镜像并发布到 GitHub Container Registry：

```bash
docker pull ghcr.io/momingxu/airy:latest
```

推送到 `main` 时发布 `latest`、`main` 和 `sha-*` 标签；推送 `v1.2.3` 形式的 Git
标签时额外发布 `1.2.3` 和 `1.2` 标签。Pull Request 只执行多架构构建验证，不会
登录或发布镜像。工作流使用仓库自动提供的 `GITHUB_TOKEN`，不需要额外配置仓库
密钥，但仓库的 Actions 权限需要允许工作流写入 Packages。

使用 SQLite 运行：

```bash
docker run -d --name airy \
  -p 8096:8096 \
  -e DB_DRIVER=sqlite \
  -v airy-data:/data \
  -v /宿主机/写真目录:/mnt/写真 \
  -v /宿主机/家庭照片:/mnt/家庭照片 \
  airy:latest
```

Docker 镜像会自动识别容器环境，启动时把 `/mnt` 下每个非隐藏的一级目录分别登记为
资源库，因此可以将多个宿主机目录统一挂载到 `/mnt/<资源库名称>`。登记不会触发媒体
扫描；只有后台手动扫描、新相册目录监听或后台定时任务才会触发扫描。

容器默认以 UID/GID `10001` 的非 root 用户运行。照片目录需要允许该用户创建和更新
`thumbnails`；应用不会修改或删除原媒体。PostgreSQL 环境可
通过 `DB_HOST`、`DB_PORT`、`DB_USER`、`DB_PASSWORD` 和 `DB_NAME` 传入。

启用 HTTPS 时挂载证书并传入容器内路径：

```bash
docker run -d --name airy \
  -p 8443:8096 \
  -e DB_DRIVER=sqlite \
  -e TLS_CERT_FILE=/certs/server.crt \
  -e TLS_KEY_FILE=/certs/server.key \
  -v airy-data:/data \
  -v /宿主机/证书目录:/certs:ro \
  airy:latest
```

## 常用命令

```bash
# 后端测试
go test ./...

# 前端类型检查和生产构建
cd web && npm run build
```


### 独立程序发布

同一工作流同时构建内嵌前端的 Linux amd64、arm64 和 Windows amd64 程序。每次构建可在 Actions
运行页面的 `airy-programs` 下载产物，保留 30 天。推送 `v*` 版本标签时，
程序构建和测试成功后会自动创建 GitHub Release（不受 GHCR 镜像推送失败影响），附带：

- `airy-linux-amd64.tar.gz`
- `airy-linux-arm64.tar.gz`
- `airy-windows-amd64.zip`（含 `start.bat`）
- `SHA256SUMS` 校验文件

带连字符的版本标签（例如 `v1.2.3-beta.1`）标记为预发布。重跑版本构建会更新同名附件。
发布使用 `GITHUB_TOKEN`，Release 作业需要仓库内容写入权限。

解压适合 CPU 架构的包，按需将 `env.example` 复制为 `.env` 并填写数据库配置，
运行 `./airy`。独立程序已包含网页，但宿主机需自行安装 `ffmpeg/ffprobe`，或在
配置中指定工具路径；它们不包含在程序压缩包中。

Windows 用户解压 ZIP 后双击 `start.bat`，访问 `http://localhost:8096`。未设置数据库
配置时默认使用程序目录中的 SQLite 数据库。Windows 需要配置对应的 `ffmpeg.exe`
和 `ffprobe.exe`，可通过 `.env` 指定其绝对路径。

## 使用 CSV 批量修改相册

在“资源”面板顶部的“全局相册 CSV”区域使用“导出 CSV”和“导入 CSV”按钮。
导出包含全部资源库的相册，用 Excel 或文本编辑器修改后，保存为 UTF-8 CSV 并导入。
文件包含五列，不包含路径，相册匹配仅使用相册 ID：

```csv
相册ID,相册标题,发布日期,系列,模特
12,修改后的相册标题,2026-02-05,城市写真,小雨;小雪
13,另一个标题,,,
```

保留表头和相册 ID，可修改标题、发布日期、系列和模特。日期须为有效的 `YYYY-MM-DD`，
留空会保留现有日期。标题不能为空，最多 255 个字符。每次最多导入 5 MB、20000 行。
导入后先预览修改前后内容；重复 ID、不存在的 ID 或格式错误须全部修正后重新导入。
点击“应用修改”才会保存，任一更新失败则全部回滚；预览后数据有变动时需要重新预览。

修改仅写入数据库，不改动磁盘目录名称，后续扫描保留手动修改的标题和发布日期。
导出包含 UTF-8 BOM，并对可能被电子表格当作公式的标题加保护前缀；这些标题未经
编辑直接导入时会还原原值。

系列列填写一个名称；模特列可填写多个名称，以英文或中文分号分隔。不存在的系列和
模特会在“应用修改”时自动创建，同一文件内重复出现的新名称只创建一次。
模特按主名称精确匹配，多个同名模特时会报错，不按别名猜测；需先在模特页区分名称。
名称含分号时，模特列也支持 JSON 名称数组（例如 `["小雨;别名","小雪"]`），
导出会自动选择能完整保留名称的格式。

系列或模特列留空保留原关联，填写名称替换原关联，填写 `[清空]` 解除关联。
预览展示关联前后内容和待创建记录；新建与关联修改一起提交，失败全部回滚。
旧版三列 CSV 仍可导入，并保留已有系列和模特关联。
