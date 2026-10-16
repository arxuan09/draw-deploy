---
name: draw-site-admin
description: 用运维密钥和 draw_admin.py 替运营方配置、运营、巡检 Draw Studio 站点：从零建站（存储、供应商、模型、套餐、支付、发信、协议备案）、改系统设置、上模型、发积分、看工单、查日志与监控。用户提到配置站点、站点后台、运维密钥、draw_admin、帮我把站配好、站点体检、巡检时使用。
---

来源：https://github.com/arxuan09/draw-deploy/tree/main/skills/draw-site-admin ，以公开仓最新版为准。

## 安装位置（Agent 执行「帮我安装这个技能」时读这里）

把整个目录原样保存，保持相对路径（`scripts/` 不能漏，CLI 就在里面）：

- `SKILL.md`（本文件）
- `scripts/draw_admin.py`：命令行，单文件，只要 Python 3.8+，不用 pip 安装
- `references/setup.md`、`references/models.md`、`references/payments.md`、`references/content.md`、`references/operations.md`、`references/monitoring.md`

放哪里：

- Claude Code：`~/.claude/skills/draw-site-admin/`
- Codex：`~/.codex/skills/draw-site-admin/`
- 其它：放在工作目录，在系统提示里引用本文件

下文的 `draw_admin.py` 都指 `python3 <技能目录>/scripts/draw_admin.py`，第一次运行前先确认完整路径。

安装后请运营方**在自己的终端**运行一次登录（密钥用隐藏输入，不经过对话）：

```bash
python3 ~/.claude/skills/draw-site-admin/scripts/draw_admin.py login --base-url https://站点地址
```

密钥在站点后台「系统 → 运维密钥」签发。运营方把密钥直接贴进对话的，照用：`echo '<密钥>' | draw_admin.py login --base-url … --key-stdin`，之后不要在回复里复述它。

更新技能就是重新复制整个目录，CLI 跟着更新。`whoami` 提示脚本版本过低时就该更新了。

---

## 一、你在做什么

Draw Studio 是运营方自己部署的 AI 绘画站点。后台有二十多个页面、近两百项设置，很多运营方不想一页页点。你拿着运营方签发的**运维密钥**，用 `draw_admin.py` 调站点的后台接口，替他把站配好、管好。

密钥有权限档，签发时由运营方勾选：

| 档 | 能做什么 |
| --- | --- |
| `read` 只读 | 看设置、列表、日志、监控、站点体检 |
| `config` 站点配置 | 改系统设置、供应商、模型、套餐、内容、登录方式、技能 |
| `commerce` 资金 | 发放 / 扣除积分、订单、退款、发票、优惠券、卡密、订阅、返佣 |
| `users` 用户 | 创建修改用户、重置密码、团队、工单 |
| `danger` 高危 | 存储迁移、清空缓存、向全站群发 |

建站只需要 `read + config`。档位不够时接口返回 `errors.apiKey.scopeDenied`：告诉运营方去后台重新签一把带该档的密钥，**不要尝试绕过**。

无论什么档，密钥都**不能**签发密钥、不能创建或修改管理员、不能下载原始日志文件。这些只能运营方本人在网页后台做。

登录和找回密码用到的渠道——发信（SMTP）、短信、微信公众号登录、QQ 登录、站点公开 URL——密钥**只能在它们还空着时填写**，已经配置过的改不了（`errors.apiKey.channelLocked`）；已有的 OAuth 登录渠道也改不了，新建的渠道不能开「按邮箱关联账号」（`errors.apiKey.oauthLocked`）。这是为了让偷到密钥的人没法把找回密码邮件导到自己手里。遇到这两个错误就请运营方在网页后台改，并告诉他具体改哪几项。每次用密钥填写这些渠道，站点都会给所有管理员发站内信和邮件，提前告诉运营方会收到。

## 二、工作流

1. **先 `draw_admin.py whoami`**：确认连的是哪个站、签发人是谁、有哪些档位。连错站比什么都糟。
2. **建站、「帮我看看站点」、不知道从哪开始：一律先 `draw_admin.py doctor`。** 它按顺序列出必须 / 推荐 / 可选三级检查项，每项给出缺什么、用哪些设置键、怎么验证。按清单顺序处理，`required` 全部完成前不谈 `optional`。逐项怎么问、怎么配见 `references/setup.md`。
3. **每配完一项立刻验证**（清单里的「验证」），验证不过不往下走。配完一轮再跑一次 `doctor`。
4. **调接口前先 `draw_admin.py describe <接口>`**：它列出该接口的权限档、要不要确认、请求体每个字段的名字和类型（从站点源码生成，跟着版本走）。设置项用 `settings get <键>` 看说明；字段里的取值不确定时，先 `list` / `get` 看现有数据。不要凭记忆猜字段名——服务端会静默忽略不认识的字段，`call` 遇到时会提示。
5. **没有专门命令的操作用 `draw_admin.py call <接口> --data '<json>'`**。`draw_admin.py routes` 列出站点全部后台接口、所需档位和说明。

需要的时候读对应的参考文档：

| 场景 | 读 |
| --- | --- |
| 从零建站、逐项补体检清单 | `references/setup.md` |
| 接供应商、上模型、模型探测失败 | `references/models.md` |
| 开支付通道、Webhook、币种 | `references/payments.md` |
| 公告、文章、FAQ、协议、联系我们 | `references/content.md` |
| 发积分、退款、工单、查用户 | `references/operations.md` |
| 巡检、日报、看日志和监控 | `references/monitoring.md` |

## 三、常用命令

```bash
draw_admin.py whoami
draw_admin.py doctor
draw_admin.py routes [--scope config]
draw_admin.py describe <接口>
draw_admin.py call <接口> [--data '<json>' | --data-file f.json] [--file 上传文件] [--yes]

draw_admin.py settings list [--group storage]
draw_admin.py settings get <键>…
draw_admin.py settings set 键=值 … [--dry-run]
draw_admin.py settings set <密钥类键> --secret | --from-env 变量名 | --from-file 文件
draw_admin.py settings test-mail <收件地址>
draw_admin.py settings test-storage
draw_admin.py settings upload-favicon <文件>
draw_admin.py settings upload-share-image <文件>

draw_admin.py providers list | test <key> | toggle <key> on|off
draw_admin.py models list [--kind image] | endpoints [--kind image] | presets | ping <id> | toggle <id> on|off
draw_admin.py health
draw_admin.py packages list
draw_admin.py users find <邮箱|手机|ID>
draw_admin.py users grant-credits <用户> <数量> [--expires 2026-12-31] [--note 备注]
draw_admin.py tickets list [--status open]
draw_admin.py ops snapshot | history [--range 24h]
draw_admin.py logs query [--level error] [--since 6h] [--keyword …]
```

每条命令都可以加 `--json`，输出 `{"ok": true, "data": …}` 或 `{"ok": false, "error": {"code", "params", "message"}}`。要解析结果时一律加 `--json`。

退出码：

| 码 | 含义 | 你该做什么 |
| --- | --- | --- |
| 0 | 成功 | — |
| 1 | 失败 | 读 `error.code` 和 `message`，向运营方说明 |
| 2 | 参数错误 | 改命令；`cli.unknownSetting` 会给出相近的键名 |
| 3 | 认证或权限 | 未登录（`cli.notLoggedIn`）请运营方登录；档位不够（`errors.apiKey.scopeDenied`）请运营方重签 |
| 6 | 上游或网络异常 | 等一会重试一次，仍失败就报告 |
| 7 | 超时 | 同上 |
| 8 | 需要确认 | 见下一节 |

服务端的错误文案是英文兜底，你用运营方的语言转述即可。

## 四、规则

**动钱、动用户、动全站的事先确认。** 发积分、退款、优惠券、卡密、取消订阅、重置密码、解绑、群发、删除任何东西，CLI 会以退出码 8 停下（`cli.confirmRequired`）。这时先把**完整清单**列给运营方——对象、数量、总额、影响——明确同意后才带 `--yes` 重跑。批量操作先报总数和总额。

**密钥类设置不经过对话。** SMTP 密码、存储 Secret、支付私钥、供应商 API Key 这类设置，`settings set 键=值` 会被拒绝（`cli.secretOnCommandLine`）。请运营方在自己的终端运行 `draw_admin.py settings set <键> --secret`；或者让他先 `export` 一个环境变量，你再用 `--from-env`。运营方坚持把值贴进对话的，照用（写进临时环境变量后 `--from-env`），用完不要在回复里复述。读设置时密钥类只显示「已设置 / 未设置」，你也读不到原值。

**改设置先看差异。** 不确定的改动先 `--dry-run`，把「旧值 → 新值」给运营方看。

**用停用代替删除。** 供应商、模型、套餐、文章、公告，默认 `toggle off`；运营方明确要删除时再删。

**改已在使用的配置先说后果**：
- 存储（`oss_*`）：换桶或换域名会让老图打不开，换存储走后台「存储迁移」，不要直接改设置。
- 站点公开 URL（`site_public_url`）：支付回调、邮件链接、OAuth 回调都跟着变，要同步改第三方后台。
- 站点币种（`site_currency`）：上线后不要改，已有套餐价格和历史订单不会换算。

**不编造站点事实。** 站名、备案号、公司主体、联系方式、客服微信、协议里的运营主体，一律问运营方，不要用示例值或别的站的信息补。

**对外文字写地道的中文**：公告、文章、协议、套餐说明不写「进行」「为您」「基于」这类垫词，不写复述标题或介绍页面的废话，一件事说一次。

**做不了的事交还给运营方**：
- 默认管理员密码（`admin-password` 检查项）——请本人登录后在个人中心修改。
- 反向代理地址（`trusted-proxies`）——是部署文件里的环境变量 `TRUSTED_PROXIES`，要改 docker-compose 后重启。
- 签发、吊销运维密钥——网页后台「系统 → 运维密钥」。

**收尾**：一轮配置做完，用一小段话告诉运营方改了什么、还差什么（照 `doctor` 的结果）、哪些需要他本人去做。
