---
name: draw-protocol-converter
description: 读一份图片或视频生成 API 的文档，用运营方习惯的语言（Go / Node.js / Python FastAPI 等）写一个「协议转换器」小服务，让 Draw Studio 通过「通用协议」端点接入这个上游；负责写代码、写好代码层面的自查测试并跑通、给出用 .env 启动的方法和后台填写说明。语言先问用户再定。
---

来源：https://raw.githubusercontent.com/arxuan09/draw-deploy/main/skills/draw-protocol-converter/SKILL.md ，以公开仓最新版为准。

## 安装位置（Agent 执行「帮我安装这个 skills」时读这里）

把本文件和同目录下这些文件一起保存（raw 地址把 `SKILL.md` 换成对应路径即可）：

- `protocol.md`：协议全文，唯一的契约
- 一个用 Go 写的完整示例：`example-go/server.go`、`example-go/adapter.go`、`example-go/converter_test.go`、`example-go/.env.example`、`example-go/go.mod`、`example-go/Dockerfile`、`example-go/.gitignore`

放哪里：

- Claude Code：`.claude/skills/draw-protocol-converter/`（保持上面的相对路径）
- Codex：`.codex/skills/draw-protocol-converter/`，或把正文追加进 `AGENTS.md`、其它文件放工作目录
- 其它：保存到工作目录，在系统提示里引用本文件

安装后告诉用户：**「把上游 API 文档（链接、PDF、curl 示例都行）和要接的模型名给我，再告诉我你习惯用什么语言写服务（Go / Node.js / Python FastAPI 都行）。」**

---

## 一、你在做什么

Draw Studio 是一个 AI 绘画站点。它内置了几家官方接口，其余上游（中转站、新厂商）不内置，而是由运营方自己部署一个**协议转换器**：

```
Draw Studio ──(通用生成协议 v1)──► 协议转换器（你写的） ──(上游私有格式)──► 上游
```

转换器是一个小 HTTP 服务：按 `protocol.md` 接收 Draw Studio 的请求，翻译成上游的请求，再把上游的结果翻译回来。**`protocol.md` 是唯一的契约**。

`example-go/` 是一个用 Go 写的完整示例，对接一家同时提供图片和视频的中转站：`server.go` 是与上游无关的部分（路由、Key 透传、模型校验、预演、错误格式、补 `downloadWithKey`、读 `.env`），`adapter.go` 是「协议 ↔ 上游」的翻译，`converter_test.go` 是代码层面的自查，`.env.example` 是配置模板。它只是参考：用户选 Go 的话可以直接在它上面改（通常只重写 `adapter.go` 和测试里的上游模拟）；选别的语言就照它的结构和第三节的清单写，不要照搬它的上游细节。

用户多半是在自己的电脑上（常见是 Windows）开发，写完再部署到一台 Draw Studio 能访问到的机器。不要假设本机有 Docker 或 Draw Studio。

## 二、开始前要问清的

只问这三件，缺一项就问，不要猜：

1. **用什么语言写**：Go、Node.js、Python（FastAPI）都可以，其它语言只要能起 HTTP 服务也行。**问用户习惯用什么、以后谁来维护，不要替用户决定。** 用户说都行时，列出各自的特点让用户选：Go 编译成单个可执行文件、部署最省事；Node.js 零依赖、改起来快；Python FastAPI 写起来直观。
2. **上游文档**：接口地址、鉴权方式、请求 / 响应示例，至少一段能跑通的 curl。
3. **模型名**：上游叫它什么（`seedream-5.0`、`doubao-seedance-2-0-260128` 这种），要接哪几个。

定价不用问：价格由运营方在 Draw Studio 后台自己填。能力声明里的 `pricingHint` 是可选的预填建议，用户主动给了站内积分价格才写，否则不写。

## 三、转换器必须做到的事

对照 `protocol.md` 逐条实现，一条都不能少：

1. **五个接口**：`GET /v1/capabilities`、`POST /v1/images/generate`、`POST /v1/images/query`、`POST /v1/videos/submit`、`POST /v1/videos/query`。只接图片的可以不实现视频两个（回 404），反之亦然；图片不走 202 异步的，`/images/query` 对任何 taskId 回 404 即可。
2. **Key 透传**：Draw Studio 每个请求带的 `Authorization: Bearer <Key>` 就是**上游的 Key**（运营方在后台供应商里直接填上游 Key），原样拿去调上游。转换器**不保存任何密钥**，也不判断 Key 对不对（上游会判断）；只有不带 Key 的请求回 401。上游要多个凭据（如 AK + SK）的，约定一种拼法（如 `AK:SK`）让运营方填进这一个 Key，转换器拆开用，并在交付说明里写清楚。
3. **配置从 `.env` / 环境变量读**：上游地址、端口不写进代码。启动时读同目录的 `.env`（已设置的环境变量优先，这样 `docker run -e` 也能用），并附一份 `.env.example` 模板。配置里没有密钥。
4. **能力声明**：照上游文档如实写（protocol.md 第 2 节）。图片按「比例 + 档位」出图用 `formTemplate: "gemini"`，按像素尺寸用 `"openai"`；只声明真的实现了的能力；视频的时长、比例、分辨率写法、首尾帧、素材上限、素材能否与首尾帧同时用、提示词里怎么称呼素材，全部照抄上游文档——它们会直接限制用户能选什么。声明的取值要和代码里接受的一致。
5. **先校验模型**：请求里的 `model` 不在能力声明里、或种类不对（拿视频模型调图片接口），直接回 400 + `user_input`，不要调上游。
6. **错误格式**：所有失败都回 `{"error": {"category", "code", "message"}}`，分类照 protocol.md 第 4 节认真分（它决定用户看到什么、模型状态页会不会变红）：上游说参数不对 → `user_input`；内容审核 / 敏感 / 违规 → `moderation`；上游限流 429 → `busy`；上游 401/403、5xx、超时、模型不存在 → `upstream`；自己解析不了上游响应 → `converter`。上游有明确错误码的按错误码判。
7. **预演（dry-run，强烈建议）**：协议里它是可选的，但实现成本很低，又能让运营以后在站点服务器上用检查工具不花额度地核对字段映射，所以照做：能力声明写 `"dryRun": true`，并实现：带 `X-Draw-Dry-Run: 1` 的生成请求不调上游，回 `{"dryRun": true, "upstreamRequests": [...]}`，列出本来要发的上游请求（方法、地址、请求头、body），**请求头里的密钥打码**（值里带 `*`），不下载素材（要 base64 的地方放占位文字）。最省事的做法是把「发上游请求」收口成一个函数，预演时在这个函数里记录并中止。
8. **异步任务**：上游是「提交 + 轮询」的，把上游任务 ID 编码进协议的 `taskId`（例如 base64url 编码 `{"u":"上游ID"}`），查询时解出来。转换器不存状态，重启不丢任务。`taskId` 解不开、或上游说任务不存在，回 **404**。
9. **查询接口的状态码**：任务确定失败回 200 + `"status": "failed"` + `error`；上游查询接口临时出错（5xx、超时、限流）回 5xx / 429（Draw Studio 会重试）；不要用 5xx 表达「任务失败」。
10. **结果文件**：上游给的结果链接**原样返回**，Draw Studio 直接去上游下载，不经过转换器。结果链接要带 Key 才能下载的（中转站的 `/v1/videos/{id}/content` 这类），在能力声明的每个模型里写 `downloadWithKey`，列出上游的源（如 `["https://relay.example.com"]`，协议 + 主机 + 端口，不带路径），Draw Studio 下载这些源上的链接时会带上 Key；Go 示例的 `server.go` 自动用 `UPSTREAM_BASE_URL` 的源补上。公开 CDN、带签名的直链不用写，Draw Studio 会匿名下载，Key 不会发给它们。上游下载要的不是 `Authorization: Bearer <Key>` 这种头时：图片由转换器下载后回 `b64`；视频照 protocol.md 1.4 的退路做，并在交付说明里写清对外地址怎么配。
11. **素材**：请求里的参考图、首尾帧、参考视频 / 音频都是公网 URL。上游收 URL 就直接给；要 base64 就下载后编码；要文件就下载后 multipart 上传。转换器不校验 Key、谁都能调，所以自己下载素材时**只访问公网 http(s) 地址**，拒绝解析到内网 / 本机 / 云元数据接口的地址（按连接时解析出的 IP 判断，重定向也算）——示例的 `FetchMaterial` 已经这样做，别的语言照着实现。
12. **不改写**：`prompt` 已是最终版（扩图 / 放大指令、比例提示、视频素材编号都拼好了），`size` 已拆成 ratio / tier / width / height，照用即可；上游不支持的值回 `user_input`，不要悄悄换成别的。后台「默认参数」在 `params` 里，是否合并进上游请求由你按上游文档决定。
13. **不泄露 Key**：错误信息、日志、预演回报里都不能出现完整的 Key。

## 四、工作流

1. **读 `protocol.md` 全文**，再读上游文档，把两边的字段一一对上：列一张映射表给自己（协议字段 → 上游字段、上游状态 → 协议状态、上游错误 → 协议分类）。
2. **按用户选的语言写**：一个小项目，结构保持简单——路由与通用逻辑（取 Key、模型校验、预演、错误格式、读配置）一层，「协议 ↔ 上游」翻译一层，和 `example-go/` 一样。依赖越少越好（Go 用标准库；Node 可以零依赖；Python 用 FastAPI + httpx + uvicorn）。
3. **写自查测试并跑通**：用所选语言的测试框架（Go `go test`、Node `node --test`、Python `pytest`），起一个**模拟上游**（按上游文档的请求 / 响应格式回假数据），不联网、不花额度，把第三节的清单逐条测到，至少包括：
   - 不带 Key 回 401；请求带来的 Key 原样出现在上游请求里
   - 能力声明合法，声明的取值和代码接受的一致；结果要带 Key 下载的，每个模型都有 `downloadWithKey`
   - 不认识的模型、不支持的比例 / 档位 / 能力回 400 + `user_input`，且没有调上游
   - 每类请求（文生图、参考图、视频文生 / 首帧 / 参考素材……按声明的能力）翻译成的上游请求字段正确，上游的响应翻译回来正确
   - 上游各类报错（400、审核、429、401、5xx）分到正确的类别
   - 预演不调上游、回报里密钥打了码
   - 下载素材时内网 / 本机地址被拒绝（如果转换器会下载素材）
   - 异步任务：进行中、成功（结果链接原样返回）、失败、上游查询临时 5xx 原样回 5xx、不认识的 taskId 回 404
   
   `example-go/converter_test.go` 就是这样一份自查，照着写。全部通过再往下走。
4. **本地真跑一次**（先征得用户同意，会花上游额度，Key 请用户自己在命令里填）：按 `.env.example` 配好 `.env` 启动转换器，用 curl（Windows 可用 PowerShell 的 `Invoke-RestMethod`）带着 `Authorization: Bearer <上游Key>` 对本机的 `/v1/images/generate` 或 `/v1/videos/submit` + `/v1/videos/query` 各发一次，确认能拿到图片 / 视频，结果链接带 Key 能下载。
5. **交付启动方式**：
   - 首选 `.env`：复制 `.env.example` 为 `.env` 填好，然后直接运行（Go：`go build` 出可执行文件，Windows 上是 `.exe`，和 `.env` 放在一起运行；Node：`node server.mjs`；Python：`python -m uvicorn …` 或写个 `main` 入口）。
   - 可选 Docker：附一个 Dockerfile（Go 多阶段构建出静态二进制；Node 用 `node:22-alpine`；Python 用 `python:3.12-slim`），配置用 `--env-file .env` 传进去。
   - 提醒用户：转换器要部署在 Draw Studio 能访问到的机器上；上游 Key 会经网络到达转换器，所以要走 HTTPS，或者和 Draw Studio 放在同一台机器 / 同一内网；前面有 nginx 之类的反向代理的话，读超时不小于 600 秒。
6. **告诉用户后台怎么填**：
   - 「供应商」新增一个：Base URL 填转换器的对外地址，API Key 填**上游的 Key**。上游按分组 / 账号发 Key 的（不同模型要不同的 Key），一个 Key 建一个供应商，Base URL 都填这个转换器。
   - 「模型」新增：API 适配选「通用协议 · 图片」或「通用协议 · 视频」，供应商选刚建的那个，点「读取能力」，在要用的模型旁点「使用」（会自动填模型 Key、能力和选项），核对「带 Key 下载」列出的是上游自己的域名，填好价格后保存，然后在站里出一张图 / 一条片验证。
   - 以后改了转换器的能力、或者换了上游地址，要回模型里重新「读取能力」并保存（「带 Key 下载」的地址是存在模型里的）。

## 五、交付给用户的东西

1. 转换器源码、自查测试、`.env.example`，以及可选的 Dockerfile。
2. 自查测试全部通过的结果，以及本地真跑的结果（如果做了）。
3. 启动方法和每个配置项的说明。
4. 后台填写说明（第四节第 6 步）。
