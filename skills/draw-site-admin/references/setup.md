# 从零建站

按 `draw_admin.py doctor` 的顺序逐项处理。每一项：先问运营方要信息 → 写入 → 验证 → 下一项。一次只问当前这一项要的东西，不要一上来甩一张大表。

开始前跑一次 `whoami` 和 `doctor`，把「必须项 x/y 完成」告诉运营方，说明接下来按什么顺序走。

## license 软件授权

授权无效时全站接口都会被拒绝，CLI 也会报错。这一项你修不了：把 `doctor` 里的原因转述给运营方，请他到后台「系统 → 软件授权」处理或联系授权方。

## admin-password 默认管理员密码

密钥不能改管理员密码。请运营方用该管理员账号登录，在个人中心改密码。改完再跑 `doctor` 确认。

## site-identity 站点名称与公开地址

问：站名（界面上显示的名字）、用户访问的地址（带 https://，不带路径）。

```bash
draw_admin.py settings set site_name=某某绘图 site_public_url=https://example.com --dry-run
draw_admin.py settings set site_name=某某绘图 site_public_url=https://example.com
```

公开地址决定支付回调、邮件链接、OAuth 回调，必须是用户真实访问的域名。密钥只能在它空着时填一次，填错了要运营方去网页后台改，所以保存前先让运营方确认。

## storage 对象存储

生成的图片全部存这里，没有它不能出图。支持任何 S3 兼容存储（Cloudflare R2、Backblaze B2、阿里云 OSS、腾讯云 COS、MinIO 等）。

问运营方要：Endpoint、Region、Bucket、公开访问域名（用户浏览器加载图片的地址，通常是绑定在桶上的 CDN 域名）、Access Key ID、Secret Access Key；是否需要 Path-Style（MinIO、部分自建存储需要）。

```bash
draw_admin.py settings get oss_endpoint oss_region oss_public_base_url oss_force_path_style   # 先看说明
draw_admin.py settings set oss_endpoint=https://… oss_region=auto oss_bucket=… oss_public_base_url=https://img.example.com oss_access_key_id=…
draw_admin.py settings set oss_secret_access_key --secret        # 请运营方在自己终端运行
draw_admin.py settings test-storage
```

`oss_access_key_id` 不是密钥类，可以直接写；`oss_secret_access_key` 是。

测试失败时：
- `AccessDenied` / 403：密钥没有该桶的读写权限。
- `NoSuchBucket`：桶名或 Region 错。
- 连接超时：Endpoint 错，或服务器访问不到该地址。
- 测试通过但用户看不到图：公开访问域名没指向这个桶，或桶没开公共读。请运营方在浏览器里打开一张已生成图片的地址确认。

**已经在用的站不要改桶和服务商**，换存储走后台「存储迁移」（`danger` 档，迁移期间站点进入维护）。只换访问域名可以直接改 `oss_public_base_url`，但新域名必须指向同一个桶。

## provider AI 供应商 / image-model 图片模型

见 `references/models.md`。至少接一个供应商、启用一个图片模型，并且 `models ping` 成功。

## packages 积分套餐

用户在「套餐」页买积分。问运营方：想卖几档、每档价格和积分数、积分有效期（天，不填 = 永久）、要不要月卡式订阅。不知道怎么定价的，先 `models list` 看各模型每张图扣多少积分，按「一档能出多少张图」给出 2–3 档建议，由运营方拍板。

先看现有套餐和字段：

```bash
draw_admin.py packages list --json
```

新建（价格单位是**分**，按站点币种；`kind` 为 `credits` 或 `subscription`）：

```bash
draw_admin.py call packages/create --data '{"name":"入门包","credits":500,"priceCents":990,"validityDays":365,"kind":"credits","enabled":true,"sortOrder":1,"features":["约 50 张高清图"]}'
```

订阅类另需 `subscriptionPeriodDays`、`subscriptionTotalPeriods`、`subscriptionCreditsPerPeriod`；Stripe / Waffo 自动续费再加 `billingInterval`（`month` / `year`）。先读一个现有订阅套餐的 JSON 照着写。改价用 `packages/update`（带 `id`），下架用 `packages/toggle`。

## payment 支付通道

密钥写不了任何支付设置，也动不了支付通道：币种、收款账户、密钥和支付开关都由运营方在网页后台填（`doctor` 这一项给出的「后台」页面）。你告诉他要准备什么、Webhook 怎么配，他填完你来验证。逐家怎么做见 `references/payments.md`。

## mail 发信

没有发信，用户收不到验证码、找回密码邮件。问：SMTP 主机、端口、是否 TLS、用户名、密码、发件人邮箱和名称。

**整组（含密码）必须在同一条命令里保存**：SMTP 主机一旦填上，发信这组设置密钥就不能再改了（防止有人把找回密码邮件导走），分两条命令先填主机、再设密码，第二条会被拒（`errors.apiKey.channelLocked`）。密码用 `--from-env`（或运营方在终端用 `--secret`），和其它 `键=值` 写在一起：

```bash
export SMTP_PW=…        # 运营方在自己终端；或运营方把密码贴进对话时由你 export
draw_admin.py settings set mail_smtp_host=smtp.example.com mail_smtp_port=465 mail_smtp_secure=true mail_smtp_user=no-reply@example.com mail_from_email=no-reply@example.com mail_from_name=某某绘图 mail_smtp_password --from-env SMTP_PW --dry-run
draw_admin.py settings set mail_smtp_host=smtp.example.com mail_smtp_port=465 mail_smtp_secure=true mail_smtp_user=no-reply@example.com mail_from_email=no-reply@example.com mail_from_name=某某绘图 mail_smtp_password --from-env SMTP_PW
draw_admin.py settings test-mail 运营方邮箱
```

保存前先 `--dry-run` 给运营方核对，一次填对；拿不准密码（比如末尾带不带标点）就先用 Python `smtplib` 登录一次验证。之后要改请运营方去网页后台。短信、微信、QQ 这几组同理，整组一条命令。

请运营方确认收到（含垃圾箱）。常见问题：465 端口要 `mail_smtp_secure=true`，587 通常为 `false`（STARTTLS）；QQ / 163 邮箱的密码是「授权码」不是登录密码；发件人邮箱要和 SMTP 账号一致。

## legal 用户协议与隐私政策

内置模板可以直接用（站名会自动替换），但运营主体、联系方式、适用法律要运营方核对。运营方给了自己的版本时：

```bash
draw_admin.py settings set user_agreement_content --from-file agreement.md
draw_admin.py settings set privacy_policy_content --from-file privacy.md
```

改了协议内容后建议同步改 `user_agreement_version`，改版本号会要求所有用户重新同意——先问运营方要不要。

## icp 备案号（人民币站点）

问：ICP 备案号；有公安备案的话，备案号和链接。

```bash
draw_admin.py settings set "site_icp_record=京ICP备00000000号-1"
```

## branding Logo、图标、分享图

```bash
draw_admin.py settings set site_logo_svg --from-file logo.svg      # SVG 源码
draw_admin.py settings upload-favicon favicon.png                   # 上传并自动写入 site_favicon_url
draw_admin.py settings upload-share-image share.png                 # 1200×630 左右，用于社交分享卡片
```

运营方没有素材就先跳过，不要自己画一个塞进去。

## seo 站点标语与搜索描述

`site_description` 是首页标题里「站名丨标语」的标语，20 字以内；`site_seo_description` 是搜索结果下的描述，一两句话讲清站点能做什么。先写草稿给运营方看，同意后再保存。

## moderation 提示词审查

建议开启。`sensitive_words` 是敏感词（每行一个），`prompt_review_enabled` + `prompt_review_model_id` 是用一个文本模型做语义审查。审查模型要从已启用的文本模型里选：

```bash
draw_admin.py models list --kind text
draw_admin.py settings set prompt_review_enabled=true prompt_review_model_id=<文本模型ID>
```

## trusted-proxies 反向代理地址

只能改部署文件：在 `docker-compose.yml` 的服务环境变量里加 `TRUSTED_PROXIES=<反代的内网地址或网段>`，然后重启。告诉运营方怎么改，不要说你改好了。

## admin-notify 管理员接收邮箱

```bash
draw_admin.py settings set admin_notification_email=ops@example.com
draw_admin.py settings list --group mail        # 各类提醒开关 admin_notify_*_enabled
```

## login 第三方登录与短信、video-model 视频模型

按需。短信、微信公众号、QQ 和发信一样，密钥只能在空着时填。OAuth 渠道用 `call oauth-providers/create` 新建（先 `call oauth-providers/list` 看结构），回调地址统一为「站点公开 URL + /auth/oauth2/callback」；密钥不能修改已有渠道，也不能开 `linkByEmail`，需要的话请运营方在网页后台打开。视频模型见 `references/models.md`。
