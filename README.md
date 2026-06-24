# drawnext · 部署指南

## 准备

- 一台 Linux 服务器，已装 **Docker**（含 Docker Compose v2，`docker compose version` 可用）。
- **软件授权码 `LICENSE_KEY`**：向供应商获取。没有它服务能启动但所有功能被拦截。
- **对象存储 Bucket**（S3 / 阿里云 OSS 等）：生成的图片必须存这里，不配无法出图。
- **至少一个模型厂商 API Key**（OpenAI / Gemini / 通义万相 DashScope）。

## 一键部署（推荐）

```bash
curl -fsSL https://raw.githubusercontent.com/arxuan09/draw-deploy/main/deploy.sh -o deploy.sh
bash deploy.sh          # 默认部署到 ./drawnext；也可 bash deploy.sh 自定义目录名
```

脚本会：创建部署目录 → 下载配置 → **自动生成密钥** → **提示输入授权码** → 拉镜像启动。
完成后：

- 访问 `http://<服务器IP>:16789`；
- 首个管理员见启动日志（默认 `admin@example.com` / `Admin123456!`）。**登录后立即改密码，并到后台「用户管理」把管理员账号（邮箱 / 用户名）也改成你自己的**；
- 进入后台「系统设置」配置**对象存储**与 **AI 供应商 / 模型**后即可出图。

> 目标目录已存在时脚本会**拒绝执行**（保护既有 `.env` 密钥与 `./data` 数据）。要全新部署请换目录名或先移走旧目录。

## 手动部署

```bash
git clone https://github.com/arxuan09/draw-deploy.git drawnext && cd drawnext
cp .env.example .env
# 编辑 .env：填好 JWT_SECRET、SETTINGS_ENCRYPTION_KEY 两个密钥和 LICENSE_KEY 授权码
docker compose up -d
```

## 配置（`.env`）

数据库 / 缓存（MySQL、Redis）已写死在 `docker-compose.yml`，无需配置。`.env` 只放这几项：

| 变量 | 说明 | 必填 |
|---|---|---|
| `JWT_SECRET` | 登录态签名密钥 | ✅ |
| `SETTINGS_ENCRYPTION_KEY` | 后台密钥（OSS/AI/支付/邮件）落库加密密钥，32 字节 base64/hex。**设置后切勿更改**，否则已存配置无法解密 | ✅ |
| `LICENSE_KEY` | 软件授权码，向供应商获取 | ✅ |
| `APP_PORT` | 对外端口，默认 `16789` | 可选 |
| `JWT_EXPIRES_DAYS` | 登录态有效期（天），默认 `30` | 可选 |

> 两个密钥用 `deploy.sh` 会自动生成。没填这两项服务会直接启动失败并给出中文提示。
> 站点信息、对象存储、邮件、支付、AI 等业务配置都在**后台「系统设置」**里填，不进 `.env`。

## 上线后必做（后台 → 系统设置）

| 分页 | 配置 | 说明 |
|---|---|---|
| **存储** | 对象存储 Endpoint / Bucket / AccessKey + `oss_public_base_url` | 🔴 不配无法出图 |
| **通用** | `site_public_url`（站点对外地址） | 🔴 邮件 / 支付链接依赖它 |
| **AI** | 供应商（Base URL + API Key，均必填）+ 模型 | 见下 |
| 邮件 / 支付 / 访问 / 内容安全 | SMTP、易支付、注册开关、敏感词 | 按需 |

**配置模型**：先在「供应商管理」填 Base URL + API Key（路径自动补全），再在「模型管理」选
供应商 + 端点并设置计费 / 质量 / 尺寸。各模型「质量 / 分辨率 / 选项栏怎么填」的速查表见
**[`model-config-reference.md`](./model-config-reference.md)**。

## 升级

```bash
cd 你的部署目录 && ./update.sh
```

只更新 `drawnext` 容器（数据库 / 缓存不动）；启动时自动增量建表。数据在 `./data`，升级不丢。

需要重启服务用 `./restart.sh`（仅重启，不拉新镜像；改了 `.env` 请用 `./update.sh` 让新配置生效）。

## 数据与备份

数据 **bind 挂载在部署目录的 `./data`**（`mysql` / `redis`），容器重建 / 升级都不丢；生成的图在对象存储。
**删除部署目录会一并删 `./data`**，请勿误删，并定期备份：

```bash
docker compose exec mysql mysqldump -u root -pdrawnext_root_pwd drawnext > backup-$(date +%F).sql
```

## 更换服务器

数据与密钥都在部署目录里（`.env` + `./data`），整目录搬走即可，无需重装：

1. **先找管理员 / 供应商更换授权绑定的 IP**：授权可能绑定服务器出口 IP，先把新服务器的公网出口 IP 告知对方更新授权，否则到新机会因 IP 不匹配被拦截。
2. **停服并打包整个部署目录**（含 `.env` 与 `./data`；停服可保证数据库文件一致）：

   ```bash
   docker compose stop
   cd .. && tar czf drawnext.tar.gz drawnext
   ```

3. **上传到新服务器并解压**（scp / rsync 等），进入该目录。
4. **启动**：

   ```bash
   docker compose pull && docker compose up -d
   ```

5. 确认新机正常后，旧服务器可 `docker compose down` 下线。

> 必须打包**整个目录**：`.env` 里的 `SETTINGS_ENCRYPTION_KEY` 随之搬走，后台已存配置才能继续解密；`./data` 里是数据库与缓存。

## HTTPS

生产环境用 Nginx / Caddy 反代到 `16789`，配好 HTTPS 与域名，并让域名与后台「通用 → `site_public_url`」一致。
MySQL / Redis 仅容器内网可达，不要对公网开放。

## 常见问题

| 现象 | 处理 |
|---|---|
| 前台「服务暂不可用」/ 接口报授权错误 | 授权无效 / 未填 / 过期或出口 IP 变了；查 `.env` 的 `LICENSE_KEY`，看日志 `docker compose logs drawnext \| grep 授权` |
| 点生成报错 / 出不了图 | 后台「存储」未配对象存储，或模型 / 供应商 Base URL、API Key 未填 |
| 启动失败提示要设置 `JWT_SECRET` / `SETTINGS_ENCRYPTION_KEY` | `.env` 这两项为空，用 `deploy.sh` 自动生成，或手动填入随机密钥 |
| 注册收不到验证码 / 接口报 503 | 未配 SMTP 或 `site_public_url`（后台「邮件」「通用」） |
| 端口被占用 | 改 `.env` 的 `APP_PORT` 后 `./update.sh` |
| 删了部署目录后数据没了 | 数据在 `./data`，删目录即丢；请定期 `mysqldump` 备份 |

```bash
docker compose logs -f drawnext     # 应用日志
docker compose ps                   # 容器状态
```
