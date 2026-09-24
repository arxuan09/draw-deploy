# 通用生成协议 v1

Draw Studio 与**协议转换器**之间的接口规范。转换器是运营方自己部署的一个小 HTTP 服务：Draw Studio 按本协议发请求给它，它再翻译成上游（中转站、厂商）的私有格式。本文件是契约全文，写转换器时以它为准。

实现对照：Draw Studio 后端 `internal/drawproto`。

---

## 1. 总则

### 1.1 地址与鉴权

- 后台「供应商」的 **Base URL** 填转换器根地址（`https://conv.example.com` 或 `https://conv.example.com/v1` 都可以，Draw Studio 自动补 `/v1`）。下文路径都相对 `{Base URL}/v1`。
- 后台「供应商」的 **API Key** 直接填**上游的 Key**。Draw Studio 每个请求都带 `Authorization: Bearer <Key>`，**转换器原样透传**：拿这个 Key 调上游，自己不保存任何密钥。
  - 所以一份转换器可以给多个站点、多个上游账号共用；上游按分组 / 账号发 Key 的，后台建几个供应商（同一个 Base URL、各填各的 Key）即可。
  - 上游要多个凭据（如 AK + SK）时，约定一种拼法（如 `AK:SK`）填进这一个 Key，转换器拆开用。
- 转换器**不校验** Key 对不对（那是上游的事），但**不带 Key 的请求一律回 `401`**。
- Key 会经网络到达转换器：转换器要走 HTTPS，或和 Draw Studio 部署在同一台机器 / 同一内网。转换器的日志里不要打印 Key。
- 上游的地址由转换器自己配置（环境变量）。

### 1.2 格式

- 请求、响应都是 JSON（UTF-8），字段名 camelCase。
- **不认识的字段一律忽略**，双方都是。Draw Studio 以后加字段不升版本；转换器多回的字段 Draw Studio 也忽略。
- 值为 `null` 与字段缺省等价。
- 只有不兼容的改动才开 `/v2`，届时两版并存至少三个月。

### 1.3 接口一览

| 方法 | 路径 | 用途 | 必须实现 |
|---|---|---|---|
| `GET` | `/capabilities` | 能力声明 | 是 |
| `POST` | `/images/generate` | 生成一张图 | 有图片模型时 |
| `POST` | `/images/query` | 查询异步图片任务 | 图片走异步（回 202）时 |
| `POST` | `/videos/submit` | 提交视频任务 | 有视频模型时 |
| `POST` | `/videos/query` | 查询视频任务 | 有视频模型时 |

### 1.4 素材与结果文件

- **素材（参考图、首尾帧、参考视频 / 音频）一律是公网 URL**，指向 Draw Studio 的对象存储，任何人不带凭据都能下载。转换器（或上游）自己下载。上游要 base64 的，转换器下载后自己编码。
  - 转换器不校验 Key，谁都能调，素材地址又来自请求体，所以转换器**自己下载素材时只访问公网 http(s) 地址**：拒绝解析到内网、本机、链路本地（云主机元数据接口）的地址，重定向后也要再判断，否则别人能借转换器访问内网。
- **结果文件**：转换器可以回 URL 或 base64（仅图片）。回 URL 时，把上游给的链接**原样返回**，由 Draw Studio 服务端直接去上游下载，不经过转换器：
  - 链接的源（协议 + 主机 + 端口）在该模型声明的 `downloadWithKey` 里（2.1），或与 Base URL 相同时，下载带同一个 `Authorization: Bearer <Key>`——也就是上游的 Key；
  - 其它源一律匿名下载（上游的 CDN、带签名的直链都属于这种）。
  - 所以上游的结果链接要带 Key 才能下载时（常见于中转站的 `/v1/videos/{id}/content` 这类接口），在能力声明里把上游的源写进 `downloadWithKey` 就行，例如 `["https://relay.example.com"]`。源必须完全一致：`http` 和 `https`、不同端口都算不同的源。
  - 上游下载要的不是 `Authorization: Bearer <Key>` 这种头（比如要签名）时，图片由转换器下载后回 base64；视频由转换器自己提供下载地址（与 Base URL 同源的链接，Draw Studio 同样会带 Key 来取，转换器再按上游的方式去下载）。这个地址要用与后台 Base URL **完全相同**的协议、主机、端口拼，最好做成配置项写死：部署在 HTTPS 反向代理后面时，按请求推断出来的常是 `http://`，源对不上就不会带 Key，视频全部下载失败。这种情况很少见，能直接用上游链接就不要这么做。
- 结果链接至少要在返回后 **30 分钟内**可下载（视频下载失败会在 15 分钟后重试）。

### 1.5 超时

| 调用 | Draw Studio 的超时 |
|---|---|
| `/images/generate`（同步） | 整个生成的超时，默认 600 秒（后台「系统设置」可调） |
| `/images/query` | 同上，是同一个总时限 |
| `/videos/submit` | 60 秒 |
| `/videos/query` | 30 秒；整条视频任务默认 1800 秒截止 |
| `/capabilities` | 20 秒 |

转换器前面有反向代理（nginx 等）时，代理的读超时要不小于图片生成超时，否则同步出图会被代理掐断；或者改用 202 异步。

### 1.6 幂等

每个生成请求带 `requestId`（Draw Studio 的任务 ID）。Draw Studio 重试同一件事时 `requestId` 不变，转换器可以用它去重。查询请求也带 `requestId`，便于日志关联。

---

## 2. 能力声明：`GET /capabilities`

后台新增 / 编辑模型时点「读取能力」会调用它，运营方选定其中一个模型后，这条声明被**存进模型配置**（快照）。出图时 Draw Studio 不再调用这个接口；转换器改了能力、或者换了上游地址（`downloadWithKey` 跟着变），运营方要回后台重新读取并保存，否则按旧声明走（换了域名的结果会匿名下载、失败）。

```json
{
  "protocol": 1,
  "name": "我的转换器",
  "dryRun": true,
  "models": [ ...ModelCaps ]
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `protocol` | int | 固定 `1` |
| `name` | string | 可选，后台展示用 |
| `dryRun` | bool | 是否实现了第 5 节的预演。**只有声明了才会收到预演请求** |
| `models` | ModelCaps[] | 至少一个；`model` 不能重复 |

### 2.1 ModelCaps

| 字段 | 类型 | 说明 |
|---|---|---|
| `model` | string | 模型名。后台「模型 Key」必须与它完全一致，出图请求里的 `model` 就是它 |
| `kind` | `"image"` \| `"video"` | 图片模型必须带 `image`，视频模型必须带 `video`，不能两个都带 |
| `image` | ImageCaps | 见 2.2 |
| `video` | VideoCaps | 见 2.3 |
| `pricingHint` | PricingHint | 可选，见 2.4 |
| `downloadWithKey` | string[] | 可选。结果链接落在这些源上时，Draw Studio 下载带 Key（1.4）。每项是 `https://主机[:端口]` 形式的源（本机 / 内网测试可以是 `http://`），不带路径，最多 8 个；默认端口写不写都一样。随快照存进模型配置，后台会列出来，运营方能看到 Key 会发给谁。下载途中被重定向到别的源时，Draw Studio 会去掉 Key |

### 2.2 ImageCaps

| 字段 | 类型 | 说明 |
|---|---|---|
| `formTemplate` | `"openai"` \| `"gemini"` | 工作台的尺寸控件样式，见下 |
| `ratios` | string[] | 可选。`"auto"` 或 `宽:高`（如 `"16:9"`）。预填后台的尺寸列表 |
| `tiers` | string[] | 可选。`"1K"` / `"2K"` / `"4K"` 的子集 |
| `qualities` | string[] | 可选，仅 `openai` 模板。质量值（如 `"auto"`、`"low"`、`"high"`） |
| `referenceImages` | `{max}` | 参考图上限。缺省或 `max: 0` = 不接受参考图（也就不能改图、放大、局部重绘、扩图、图层拆分） |
| `inpaint` | bool | 支持局部重绘（需要参考图） |
| `outpaint` | bool | 支持扩图（需要参考图，且必须同时 `customSize: true`：扩图按任意像素尺寸下发） |
| `layers` | bool | 支持图层拆分（需要参考图） |
| `customSize` | bool | 接受任意像素尺寸（只适用于 `openai` 模板） |
| `promptMaxChars` | int | 可选，提示词字符上限，超了在扣费前拦下 |
| `promptMaxBytes` | int | 可选，提示词字节上限（UTF-8） |
| `pollIntervalMs` | int | 可选，异步出图的默认轮询间隔（500～60000） |

**两种模板怎么选：**

- `gemini`：上游按「比例 + 1K/2K/4K 档位」出图（Seedream、Nano Banana 这类）。工作台让用户选比例和档位，档位就是质量。请求里 `quality` 是 `"1K"` / `"2K"` / `"4K"`，`tiers` 决定用户能选哪几档。
- `openai`：上游接受像素尺寸（gpt-image 这类）。工作台让用户选比例 + 自动/1K/2K/4K 档位 + 质量，Draw Studio 把比例 × 档位换算成像素。**这四个档位工作台总是全部提供**，`tiers` 只用于定价预填；上游不支持某档时转换器应回 `user_input` 错误，或者干脆选 `gemini` 模板。

`ratios` / `tiers` / `qualities` 只是**预填**：运营方可以在后台再改。所以转换器收到不支持的值时要回 `user_input` 错误（第 4 节），不能假设它们一定在声明范围内。

### 2.3 VideoCaps

视频的这些字段是**上游事实**：后台只读展示，工作台据此限制用户的选择，并在扣费前拦下不合法的组合。

| 字段 | 类型 | 说明 |
|---|---|---|
| `durations` | int[] | 可选的时长（秒）。与 `durationRange` 二选一 |
| `durationRange` | `{min, max, step?}` | 连续时长，`step` 缺省 1 |
| `ratios` | string[] | 比例，`宽:高` 或 `"adaptive"` |
| `resolutions` | string[] | 分辨率写法，原样作为请求里的 `resolution`（如 `"720p"`、`"1080p"`） |
| `audioToggle` | bool | 用户能否开关声音（请求里的 `generateAudio`） |
| `materialRefSyntax` | `"plain"` \| `"at"` \| `"none"` | 提示词里怎么称呼素材，见 3.2 |
| `firstFrame` | `"none"` \| `"optional"` \| `"required"` | 首帧 |
| `lastFrame` | 同上 | 尾帧（尾帧必填时首帧也必须必填） |
| `references.images` / `.videos` / `.audios` | `{max, required?}` | 参考素材上限；`required: true` 表示必须至少给一个 |
| `references.total` | int | 可选，三类素材合计上限 |
| `framesCountAsImages` | bool | 首尾帧是否占用参考图的名额和编号（方舟就是这样：有首帧时它是「图片1」） |
| `framesExclusiveWithRefs` | bool | 首尾帧与参考素材不能同时用 |
| `materialRequired` | bool | 只写提示词不能出片，必须带某种素材 |
| `referenceDurationLock` | int[] | 带参考图时只能用这些时长 |
| `referenceResolutionLock` | string[] | 带参考图时只能用这些分辨率 |
| `referenceAspectRatioLock` | string[] | 带参考图时只能用这些比例 |
| `resolutionDurationLocks` | `{分辨率: int[]}` | 某分辨率只能用这些时长 |
| `pollIntervalMs` | int | 查询间隔建议（Draw Studio 会夹在 3～60 秒之间；缺省 10 秒） |
| `promptMaxChars` / `promptMaxBytes` | int | 提示词上限 |

### 2.4 PricingHint

只用于后台**预填计费字段**，单位是 **Draw Studio 站内积分**（不是上游价格）。实际扣费只看后台模型配置。

| 字段 | 说明 |
|---|---|
| `mode` | `"per_image"`（图片）/ `"per_second"` / `"per_call"`（视频） |
| `base` | `per_image`：每张基础积分 |
| `tier` | `per_image`：`{"2K": 5, "4K": 15}` 各档位附加积分 |
| `quality` | `per_image`：`{"high": 30}` 某质量的整张积分 |
| `resolutions` | 视频：`{"720p": 10}` 每秒（`per_second`）或每条（`per_call`）积分 |

---

## 3. 请求与响应

### 3.1 图片：`POST /images/generate`

请求：

```json
{
  "requestId": "01J8…",
  "model": "seedream-5.0",
  "intent": "generate",
  "prompt": "白色桌面上的蓝色陶瓷杯，柔和棚拍光线",
  "size": { "raw": "16:9", "ratio": "16:9", "tier": "2K", "width": 2048, "height": 1152 },
  "quality": "2K",
  "outputFormat": "png",
  "background": "auto",
  "references": [
    { "url": "https://oss.example.com/generated/a.webp", "mimeType": "image/webp", "width": 1024, "height": 1024 }
  ],
  "mask": null,
  "layers": null,
  "params": { "negative_prompt": "blurry" }
}
```

| 字段 | 说明 |
|---|---|
| `model` | 声明里的模型名 |
| `intent` | 见下表 |
| `prompt` | **最终提示词**。扩图 / 放大的固定指令、比例提示、图层数都已拼好，转换器不要再改写 |
| `size.raw` | 用户选的原值：`"auto"`、比例 `"W:H"`、或像素 `"WxH"` |
| `size.ratio` | 比例，拿不到时 `null`（如 `auto`、自定义的怪尺寸） |
| `size.tier` | `"1K"` / `"2K"` / `"4K"`，拿不到时 `null` |
| `size.width` / `size.height` | 像素。`gemini` 模板按档位长边 1024 / 2048 / 3840 推算；`openai` 模板是工作台实际换算的尺寸（16 的倍数、长边 ≤ 3840、比例 ≤ 3:1）。`auto` 时为 `null` |
| `quality` | 质量值：`gemini` 模板是档位（`"2K"`），`openai` 模板是后台配置的质量值 |
| `outputFormat` | 可选，`png` / `jpeg` / `webp` |
| `background` | 可选，`auto` / `transparent` / `opaque` |
| `references` | 参考图，按用户给的顺序。没有时是 `[]` |
| `mask` | 仅 `inpaint`：`{"dataUrl": "data:image/png;base64,…"}`，红色区域是要重画的部分，尺寸与 `references[0]` 相同 |
| `layers` | 仅 `layers`：`{"count": 6}`，总层数（含底图）2～17；`0` 表示由上游决定 |
| `params` | 后台模型的「默认参数」JSON，原样放在这里 |

`intent`：

| 值 | 含义 | 素材 |
|---|---|---|
| `generate` | 文生图；带 `references` 时是参考图生图 | 参考图可有可无 |
| `edit` | 按提示词修改 `references[0]` | 至少 1 张 |
| `inpaint` | 只重画 `mask` 的红色区域 | `references[0]` + `mask` |
| `outpaint` | 把 `references[0]` 扩展到 `size` | 1 张 |
| `upscale` | 把 `references[0]` 以更高分辨率重绘 | 1 张 |
| `layers` | 把 `references[0]` 拆成底图 + 若干透明图层 | 恰好 1 张 |

上游不区分 `edit` / `upscale` 的，按「参考图 + 提示词」生图即可。

**同步成功**（HTTP 200）：

```json
{
  "images": [
    { "url": "https://cdn.upstream.com/x.png", "mimeType": "image/png", "revisedPrompt": null }
  ],
  "usage": { "cost": 0.16 }
}
```

- 每张给 `url` 或 `b64`（纯 base64，不带 `data:` 前缀；带了也能识别）之一；单张 ≤ 64 MB。
- 普通生成只取第 1 张。
- `revisedPrompt` 可选：上游改写过提示词时回传。
- `usage` 可选，原样存进任务记录供对账，**不参与计费**。
- 宽高以 Draw Studio 读到的实际文件为准，不用回报。
- 图层拆分：每张带 `layer`：`{"zIndex": 0, "bbox": [l, t, r, b], "bboxPx": [l, t, r, b], "name": "背景", "description": "…"}`。`zIndex` 0 是底图；`bbox` 是相对底图的千分比（0～1000），`bboxPx` 是底图像素坐标，至少给一个。

**异步**（HTTP 202）：

```json
{ "taskId": "t_123", "pollIntervalMs": 3000 }
```

之后 Draw Studio 按间隔调用 `POST /images/query`：

```json
{ "requestId": "01J8…", "taskId": "t_123" }
```

查询响应（HTTP 200）：

- 进行中：`{"status": "running", "progress": 40}`（`status` 也可以是 `queued`）
- 成功：`{"status": "succeeded", "images": [...]}`（格式同同步成功）
- 失败：`{"status": "failed", "error": {...}}`（格式见第 4 节）

查询不存在的任务回 `404`。查询回 5xx / 429（不论带不带 `category`）视为暂时失败，Draw Studio 会重试；回其它 4xx 视为任务失败。连续 20 次暂时失败或整个生成超时，Draw Studio 判失败并退积分。

**建议**：上游本身是同步的就同步返回；上游是「提交 + 轮询」的，可以在转换器里轮询完再同步返回（简单），也可以回 202 让 Draw Studio 轮询（连接不用一直挂着）。

### 3.2 视频：`POST /videos/submit`

```json
{
  "requestId": "01J8…",
  "model": "doubao-seedance-2-0-260128",
  "prompt": "保持图片1中的主体外观，参考视频1的动作",
  "duration": 6,
  "ratio": "16:9",
  "resolution": "1080p",
  "generateAudio": true,
  "firstFrame": null,
  "lastFrame": null,
  "references": {
    "images": ["https://oss.example.com/a.png"],
    "videos": ["https://oss.example.com/b.mp4"],
    "audios": []
  },
  "params": {}
}
```

| 字段 | 说明 |
|---|---|
| `prompt` | 最终提示词。用户在工作台写的「@图片1」已按声明的 `materialRefSyntax` 改写：`plain` → `图片1`，`at` → `@图片1`，`none` → 去掉 `@`。编号按各类素材在 `references` 里的顺序；声明了 `framesCountAsImages` 时首尾帧排在图片编号最前面 |
| `duration` | 秒 |
| `ratio` / `resolution` | 声明里的值 |
| `generateAudio` | `true` / `false`；声明没有 `audioToggle` 时为 `null`，按上游默认 |
| `firstFrame` / `lastFrame` | `{"url": "…"}` 或 `null` |
| `references` | 三类素材各自按用户顺序；没有时是 `[]` |
| `params` | 后台模型的「默认参数」 |

成功（HTTP 200）：`{"taskId": "…"}`。

`taskId` 由转换器决定。**建议把上游任务 ID 直接编码进去**（例如 base64url 编码 `{"u":"上游ID"}`），转换器就不用存状态，重启也不会丢任务。

### 3.3 视频：`POST /videos/query`

请求：`{"requestId": "…", "taskId": "…"}`。

响应（HTTP 200）：

```json
{ "status": "running", "progress": 35 }
{ "status": "succeeded", "videoUrl": "https://…/out.mp4", "usage": { "total_tokens": 100000 } }
{ "status": "failed", "error": { "category": "moderation", "message": "…" } }
```

- `status` 只能是 `queued` / `running` / `succeeded` / `failed`。不认识的词按「进行中」处理（等到截止时间）。
- `succeeded` 必须带 `videoUrl`，否则按失败处理。下载规则见 1.4，上限 500 MB。
- 转换器不认识这个 `taskId`（任务丢了）必须回 **404**：这是 Draw Studio 立即判失败、给用户退积分的唯一信号。
- 5xx / 429 / 网络错误：Draw Studio 退避后重试，不判失败（带不带 `category` 都一样——上游的查询接口临时出错时照实回 5xx 即可）。
- 其它 4xx：判失败。
- 任务确定失败时回 200 + `"status": "failed"` + `error`，不要用 5xx 表达「任务失败」，否则会被一直重试到截止时间。

---

## 4. 错误

任何失败都用同一个形状。同步调用回 HTTP 4xx / 5xx，异步任务在查询结果里 `status: "failed"`：

```json
{ "error": { "category": "user_input", "code": "size_not_supported", "message": "上游原始报错或你的说明" } }
```

| `category` | 什么时候用 | Draw Studio 怎么处理 |
|---|---|---|
| `user_input` | 输入的问题：参数不支持、参考图打不开、提示词太长、模型名不认识 | 用户看到「请调整尺寸、参考图或提示词后重试」；**不**计入模型故障 |
| `moderation` | 上游内容审核拦截（输入或输出） | 用户看到内容审核提示；不计入模型故障 |
| `upstream` | 上游 / 渠道故障：5xx、无可用渠道、余额不足、上游超时 | 用户看到通用失败提示；**计入模型故障**（模型状态页会变红） |
| `busy` | 转换器或上游限流、排队满 | 用户看到「模型方繁忙，请稍后重试」提示；计入模型故障 |
| `converter` | 转换器自己的 bug（解析不了上游响应等） | 同 `upstream` |

- **分类决定用户看到什么、模型状态页怎么判**，一定要认真分。常见映射：上游 400 且说参数不对 → `user_input`；上游说内容违规 / sensitive / moderation → `moderation`；上游 401 / 403（后台供应商填的 Key 不对）→ `upstream`；上游 429 → `busy`；上游 5xx / 超时 → `upstream`。
- `code` 可选，写上游的错误码（字母、数字、`_.-`），便于运营方排查。
- `message` 只进后台任务记录，**不直接给终端用户看**（上游原文可能带渠道信息、英文）。写清楚，运营方要靠它排查。
- 不带 `category` 时 Draw Studio 按 HTTP 状态兜底：400 / 422 算 `user_input`，其它算 `upstream`。
- 所有失败都会退还用户积分，与分类无关。

---

## 5. 预演（dry-run，建议实现）

能力声明里写了 `"dryRun": true`，Draw Studio 的检查工具才会发带请求头 `X-Draw-Dry-Run: 1` 的生成请求（`/images/generate`、`/videos/submit`）。收到这种请求时转换器**不调上游**，只回报它本来要发的上游请求：

```json
{
  "dryRun": true,
  "upstreamRequests": [
    { "method": "POST", "url": "https://relay.example.com/v1/images/generations",
      "headers": { "Authorization": "Bearer sk-***" },
      "body": { "model": "seedream-5.0", "prompt": "…" } }
  ]
}
```

- 密钥必须打码（值里带 `*`）。
- 预演时不要下载素材，body 里需要 base64 的地方放占位文字即可。
- 一次生成要调多个上游接口的，至少回报第一个。

生产环境里 Draw Studio **不会**发这个请求头；它只来自 `protocol check`。

---

## 6. 一致性检查（可选）

写转换器时的自查靠转换器自己的单元测试（模拟上游，不需要 Draw Studio）。转换器部署好以后，如果想从 Draw Studio 这一侧再核对一遍，可以在**部署 Draw Studio 的服务器**上（它本来就用 Docker 运行）跑镜像自带的检查工具：

```
docker run --rm --network host arxuan09/drawnext:latest \
  protocol check --base-url https://conv.example.com --key <上游Key>
```

- 检查鉴权（不带 Key 必须 401）、能力声明、错误格式、查询不存在的任务是否 404；声明了 `dryRun` 的还会按声明造样例请求，打印转换器回报的上游请求。
- 加 `--model <模型>` 只查一个模型；加 `--live --model <模型>` 真实出一次图 / 一条视频，并按 `downloadWithKey` 的规则下载结果（**会花上游额度**）。
- 报告是 Markdown，✗ 必须修，⚠ 是建议。
