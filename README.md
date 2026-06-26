# drawnext 部署指南

## 环境准备

部署前需要准备以下内容：

- 一台 Linux 服务器，已安装 Docker（含 Compose v2，可用 `docker compose version` 验证）。
- 授权码 `LICENSE_KEY`，由供应商提供。未填写时服务仍可启动，但所有接口都会被拦截。
- 一个对象存储 Bucket（S3、阿里云 OSS 等均可），用于存放生成的图片，未配置则无法出图。
- 至少一个模型厂商的 API Key，OpenAI、Gemini、通义万相 DashScope 任选其一。

## 一键部署

推荐使用部署脚本：

```bash
curl -fsSL https://raw.githubusercontent.com/arxuan09/draw-deploy/main/deploy.sh -o deploy.sh
bash deploy.sh
```

完成后：

- 浏览器访问 `http://<服务器IP>:16789`。
- 首个管理员账号见启动日志，默认为 `admin@example.com` / `Admin123456!`。登录后请立即修改密码，并到后台「用户管理」将该账号的邮箱与用户名一并改为自己的，不建议沿用默认值。
- 进入「系统设置」，配置对象存储与 AI 供应商/模型后即可出图。

若目标目录已存在，脚本会拒绝执行，以免覆盖其中已有的 `.env` 密钥和 `./data` 数据。需要全新部署时，请更换目录名或先移走原目录。

## 手动部署

```bash
git clone https://github.com/arxuan09/draw-deploy.git drawnext && cd drawnext
cp .env.example .env
# 编辑 .env，填入 JWT_SECRET、SETTINGS_ENCRYPTION_KEY 两个密钥与 LICENSE_KEY 授权码
docker compose up -d
```

## 配置 .env

`.env` 仅涉及以下几项：

| 变量 | 说明 | 必填 |
|---|---|---|
| `JWT_SECRET` | 登录态签名密钥 | 是 |
| `SETTINGS_ENCRYPTION_KEY` | 后台配置（OSS、AI、支付、邮件）落库时的加密密钥，32 字节 base64/hex。设定后请勿更改，否则已存配置将无法解密 | 是 |
| `LICENSE_KEY` | 授权码，由供应商提供 | 是 |
| `APP_PORT` | 对外端口，默认 16789 | 否 |
| `JWT_EXPIRES_DAYS` | 登录态有效期（天），默认 30 | 否 |

前两个密钥在使用 `deploy.sh` 时会自动生成，仅手动部署需要自行填写；两项为空时服务无法启动，并会给出提示。

## 部署后的必要配置（后台 → 系统设置）

以下几项直接影响出图与邮件、支付，建议优先完成：

- 「存储」：对象存储的 Endpoint、Bucket、AccessKey 及 `oss_public_base_url`。未配置将无法出图。
- 「通用」：`site_public_url`，即站点对外地址，邮件链接与支付回调均依赖该项。
- 「AI」：先在供应商中填写 Base URL 与 API Key（两者均为必填），再配置模型。

邮件、支付、注册开关、敏感词等其余配置可按需补充。

配置模型的流程为：在「供应商管理」中填写 Base URL 与 API Key，随后在「模型管理」中选择供应商与端点，并设置计费、质量与尺寸。各模型的质量、分辨率、选项栏具体如何填写，可参见速查表 [`model-config-reference.md`](./model-config-reference.md)。

## 升级

```bash
cd 你的部署目录 && ./update.sh
```

该命令仅更新 drawnext 容器，数据库与缓存保持不变，新版本启动时会自动增量建表。数据位于 `./data`，升级不会丢失。

如只需重启而不拉取新镜像，使用 `./restart.sh`。注意修改 `.env` 后须执行 `./update.sh` 才能使新配置生效，仅重启不行。

## 数据与备份

数据库数据在部署目录的 `./data` 下，删除部署目录会一并丢失，请定期备份：

```bash
docker compose exec mysql mysqldump -u root -pdrawnext_root_pwd drawnext > backup-$(date +%F).sql
```

## 更换服务器

数据与密钥都在部署目录里（`.env` 与 `./data`），整目录迁移即可，无需重装。迁移前先联系供应商更新授权绑定的 IP——授权可能绑定服务器出口 IP，不更新会在新机被拦。

```bash
# 旧服务器：停服并打包整个目录
docker compose stop
cd .. && tar czf drawnext.tar.gz drawnext

# 传到新服务器解压后，在目录内启动
docker compose pull && docker compose up -d
```

打包务必是整个目录：缺了 `.env` 里的 `SETTINGS_ENCRYPTION_KEY`，后台已存配置就解不开了。

## 常见问题

| 现象 | 处理 |
|---|---|
| 前台显示「服务暂不可用」或接口报授权错误 | 授权未生效——未填写、已过期，或出口 IP 变更。检查 `.env` 的 `LICENSE_KEY`，并查看日志 `docker compose logs drawnext \| grep 授权` |
| 点击生成报错、无法出图 | 多为后台「存储」未配置对象存储，或模型、供应商的 Base URL、API Key 未填全 |
| 启动时提示需设置 `JWT_SECRET` / `SETTINGS_ENCRYPTION_KEY` | `.env` 中这两项为空。可用 `deploy.sh` 自动生成，或手动填入随机密钥 |
| 注册收不到验证码、接口报 503 | 未配置 SMTP 或 `site_public_url`，请到后台「邮件」「通用」补全 |
| 端口被占用 | 修改 `.env` 的 `APP_PORT` 后执行 `./update.sh` |
| 删除部署目录后数据丢失 | 数据位于 `./data`，删目录即丢失，请定期使用 `mysqldump` 备份 |

排查时常用以下命令：

```bash
docker compose logs -f drawnext     # 应用日志
docker compose ps                   # 容器状态
```
