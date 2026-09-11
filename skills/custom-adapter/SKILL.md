---
name: custom-adapter
description: 读一份图片或视频生成 API 的文档，写出可直接贴进 Draw Studio 后台「自定义端点（脚本）」的 adapter.js 和配套 fixture.json；定价先问用户再写。
---

来源：https://raw.githubusercontent.com/arxuan09/draw-deploy/main/skills/custom-adapter/SKILL.md ，以公开仓最新版为准。同目录下的 `adapter.d.ts` 是契约的类型声明，`examples/` 是六个可直接运行的示例（四个图片、两个视频）。

## 安装位置（Agent 执行「帮我安装这个 skills」时读这里）

- Claude Code：保存为 `.claude/skills/custom-adapter/SKILL.md`，把同目录的 `adapter.d.ts` 和 `examples/` 一起放进去（raw 地址把文件名换掉即可）
- Codex：保存为 `.codex/skills/custom-adapter/SKILL.md`，或把正文追加进 `AGENTS.md`
- Cursor：保存为 `.cursor/rules/custom-adapter.md`
- 其它：保存到工作目录，在系统提示里引用本文件

安装后告诉用户：**「把上游 API 文档（链接、PDF、curl 示例都行）和模型名给我，我来写 adapter.js。」**

---

## 一、你在写什么

Draw Studio 的运营在后台新增模型时可以选「自定义端点（脚本）」，贴一个 `adapter.js`。它是一个 ES 模块，导出一个 `meta` 对象和几个**同步纯函数**：

| 钩子 | 做什么 |
|---|---|
| `meta` | 声明脚本是图片还是视频、开哪些能力、素材上限、定价建议 |
| `buildSubmitRequest(ctx)` | 把一次生成变成一个上游请求（URL、方法、头、body） |
| `parseSubmitResponse(ctx, response)` | 解析提交响应：给出任务 ID，或者直接给出结果 |
| `buildQueryRequest(ctx)` | 有任务 ID 时：拼一次轮询请求 |
| `parseTaskResult(ctx, body, response)` | 有任务 ID 时：解析轮询响应成 QUEUED / IN_PROGRESS / SUCCESS / FAILURE / UNKNOWN |
| `buildContentRequest(ctx)` | 成品要带鉴权头下载时：拼下载请求 |

**脚本不发请求。** 发请求、重试、轮询、取素材字节、扣费、退款全是宿主（Go）做的。脚本拿到的是 `ctx`，还回去的是「请求描述符」或「解析结果」。这意味着：

- 没有 `fetch / require / import / setTimeout / async / await`，写了保存不了
- 钩子之间没有共享变量；跨轮询要记的东西放返回值的 `state`
- 素材（参考图、遮罩）只有句柄和元数据，没有字节。要内联进 body 就放占位符 `{ __ref: ref, encoding: "dataUrl" }`，宿主发送前替换；要发文件就用 `bodyType: "multipart"` + `parts[].ref`

完整类型在 `adapter.d.ts`，**只允许用那里声明的东西**。

## 二、开始前必须拿到的东西

缺一项就问用户，不许猜：

1. 上游文档，或至少一段能跑的 curl 示例
2. 模型标识：上游叫它什么（`gpt-image-1` / `wan2.5-t2i-preview` 这种）
3. 同步返回还是任务轮询：文档看不出来就问
4. 想开哪些能力：图片是参考图、局部重绘（mask）、自定义分辨率；视频是首尾帧、参考图 / 视频 / 音频及各自上限。**文档没写的能力不许开**
5. Base URL 由运营在「供应商」里填，脚本里用 `ctx.baseUrl + "/路径"` 拼。文档给了完整地址就只取路径部分，并告诉用户 Base URL 该填到哪一截（例如「填 `https://api.example.com`，不带 `/v1`」）

## 三、定价问答（写 `meta.pricing` 之前必须走完）

`meta.pricing` 只用来预填后台表单，扣费永远读模型行上的积分配置。但表单预填的数字要对，所以这四个数缺一个就问，不许猜，**不许沿用示例里的数字**：

1. **上游单价**：文档里有就读出来复述给用户确认（「每张 0.1 元、2K 0.2 元，对吗」）；没有就问
2. **计费口径**：图片按张（质量 / 档位是否有差价）；视频按秒还是按次
3. **站内换算**：一积分对应多少钱，或者一块钱多少积分
4. **目标毛利**：上游成本乘多少（「不加价」也是一个答案）

算法：积分 = 上游单价 × 站内换算 × 毛利倍数，**向上取整到整数**。图片写成 `{ mode: "per_image", base, tier: { "2K": 加价, "4K": 加价 } }`，`base` 是最低档每张的积分，`tier` 是更高档在 base 之上**加**的积分。输出末尾附一张「上游价 → 站内积分」核对表。

站内换算和毛利是站点事实，永远不能从示例或别的站点复制。

## 四、写法规则

- `export const meta = { … }` 必须是字面量对象，字段只能是 `adapter.d.ts` 里 `Meta` 有的；`apiVersion: 1`，`kind: "image"`
- `capabilities.formTemplate`：上游像 OpenAI（`size: "1024x1024"`、`quality: low/medium/high`）写 `"openai"`；像 Gemini（比例 + 1K/2K/4K 档位）写 `"gemini"`。这决定后台给运营哪套选项编辑器
- `accept` 写上游文档确认接受的图片 MIME（如 `["image/png","image/jpeg","image/webp"]`）。写了宿主才会把超大参考图压到上游限制以内；不写就原样发
- 上游有自己的尺寸词汇就写一个查表常量，键只能是系统会给的比例：`1:1 / 16:9 / 9:16 / 4:3 / 3:4 / 3:2 / 2:3 / 21:9 / 9:21`。给不出对应值的比例不要写，查不到时回退到 1:1 的值。`input.width / height` 非 0 时是系统已算好的像素，接任意分辨率的上游直接发
- `input.tier` 是 `1K / 2K / 4K`；`input.quality` 是模型行配的原始值（可能是 `high`）。上游按档位收费的用 `tier`
- 素材形态严格按文档：接 URL 用 `ref.url`；接内联的用占位符（`base64` 还是 `dataUrl` 看文档；Gemini 风格的 `inline_data` 要 `mime_type` 和 `data` 两个字段，`mime_type` 用 `encoding: "mime"` 取转码后的类型）；接文件用 `bodyType: "multipart"` + `parts[].ref`
- 局部重绘（`capabilities.inpaint: true`）只在文档明确支持 mask 时才开，并且脚本里必须真的用到 `input.mask`
- 上游默认开启的后处理（例如「参考图默认打码人脸」）要显式关掉；要不要关先问用户
- 状态词表必须完整；认不出返回 `{ status: "UNKNOWN", reason: "unrecognized status: xxx" }`。**禁止 `|| "IN_PROGRESS"` 兜底**——UNKNOWN 是宿主判定连续轮询失败的依据
- 文档没给失败状态词时：上游自称兼容某已知协议（OpenAI images、DashScope 任务）可按该协议补齐并在 `notes` 标明「推断」，否则不写并在输出末尾列出
- `FAILURE` 时 `reason` 写上游给的失败原因（会展示给用户），限流 / 临时错误加 `retryable: true`
- 成品要带鉴权头下载的，`outputs` 给 `{ content: true, mime }` 并实现 `buildContentRequest`；公开地址直接 `{ url }`；内联的 `{ base64, mime }`
- 提交响应里就是结果的，返回 `{ immediate: { status: "SUCCESS", outputs } }`，不要造假的 `taskId`
- 非 2xx 由宿主处理，`parseSubmitResponse` 只需要在 2xx 但拿不到 ID / 结果时 `throw new Error("人话")`——抛出的文字会展示给用户
- 请求主机必须等于 Base URL 的主机；上游用了第二个域名（比如上传走另一个主机）才写 `allowedHosts`
- `console.log` 可以打关键中间量（拼出来的 size、任务 ID），**禁止打 apiKey、headers、上游整个报文**
- `notes` 写清文档来源和日期，以及哪些是推断
- `ctx.params` 是运营在后台「高级默认参数」里填的 JSON；工作区没有输入框的参数（反向提示词、参考强度）从这里取，不要硬编码

## 四点五、视频脚本的特别规则

视频脚本 `kind: "video"`，钩子和图片一样，但有几处不同：

- **必须有轮询两钩子**（`buildQueryRequest` / `parseTaskResult`）；没有视频上游是同步返回的，`parseSubmitResponse` 不许返回 `immediate`
- **`meta.video` 必填**：`durations`（离散档）或 `durationRange`（连续范围）二选一、`aspectRatios`、`resolutions`。这些是上游的固定事实，运营在后台看到的是只读展示，所以要照文档抄全；`materialRefSyntax` 按提示词里指代素材的写法填 `"at"`（`@图片1`）、`"plain"`（`图片1`）或 `""`（无此语法）
- **`meta.inputs` 声明素材槽**：`firstFrame` / `lastFrame` 写 `required` / `optional` / `none`；`refImages` / `refVideos` / `refAudios` 各写 `{ max }`，上游要求必给的加 `required: true`；三类合计封顶写 `refTotal`；首尾帧与参考素材不能同时给写 `framesExclusiveWithRefs: true`；上游不能只靠提示词出片、但随便哪种素材都行的写 `materialRequired: true`。工作区按这里显示上传框、扣费前按这里拒绝超限；**能不能只写提示词出片也只看这里**（没有任何 required 就是能），后台没有别的开关
- **带参考图时的硬锁**写在 `meta.video`：`referenceDurationLock`（只允许这些秒数）、`referenceResolutionLock`、`referenceAspectRatioLock`。有的上游带图时只接 8 秒 + 16:9，就是这个
- **素材只有 URL**：`input.firstFrame.url`、`input.refImages[i].url` 是本站对象存储的公网地址，直接放进请求让上游自己拉；视频脚本不支持占位符内联字节
- **素材名从 `ref.name` 取，别自己数**：宿主统一编号并写进 `ctx.input` 的每个素材——`refImages[i].name` 是 `图片N`、`refVideos[i].name` 是 `视频N`、`refAudios[i].name` 是 `音频N`，首尾帧是 `首帧` / `尾帧`。`ctx.input.prompt` 里的编号已经按 `materialRefSyntax` 渲染好（`"at"` 保留 `@图片1`，`"plain"` 是 `图片1`），与这些 name 逐一对应。上游要求素材带 `name` 的直接用 `r.name`；自己按下标生成会在编号规则不同的端点上和提示词对不上
- **分辨率写进模型名的上游**：站内一个模型、多个分辨率档，脚本按 `input.resolution` 拼上游模型名（见 `examples/relay-video-percall/`）。分辨率之外还不同的（素材上限、价格）是不同模型，各写一份脚本
- **成品下载**：公开地址给 `{ url, mime: "video/mp4" }`；要带鉴权头下载的给 `{ content: true, mime }` 并实现 `buildContentRequest`
- **定价口径**：按秒 `{ mode: "per_second", resolutions: { "720p": 每秒积分 } }`；一口价 `{ mode: "per_call", resolutions: { "720p": 每次积分 } }`。文档写「按次」就问用户要不要按次；键必须是 `meta.video.resolutions` 的子集
- **上游默认开启的后处理**（例如参考图默认打码人脸）要显式关掉，关不关先问用户
- 后台不会真发视频请求（一条视频要渲染几分钟）：「离线诊断」核对请求形状，保存后运营自己出一条片验证。fixture 因此更重要，状态词表要一个不漏

## 五、必须同时产出 fixture

`fixture.json` 是回归用例，格式：

```json
{
  "unixNow": 1757462400,
  "cases": [
    { "name": "用例名", "hook": "buildSubmitRequest", "args": [ … ], "expected": { … } },
    { "name": "抛错用例", "hook": "parseSubmitResponse", "args": [ … ], "expectedError": "错误里应包含的文字" }
  ]
}
```

- `args` 按钩子签名依次传入；`buildSubmitRequest` 的第一个参数是完整的 `DriverContext`（`input` 里每个字段都写，`refs` 等数组没有就写 `[]`）
- `expected` 做**子集匹配**：写了的键必须相等，没写的忽略。所以只写你关心的字段
- 每个导出钩子至少一个用例；`parseTaskResult` 覆盖文档里每一个状态词，外加一个认不出的值 → `UNKNOWN`
- 用例里的上游响应用文档里的示例响应，`expected` 手写

## 六、自检

能跑就跑到全绿（镜像里自带工具，`<镜像>` 用运营告诉你的 `arxuan09/drawnext:latest`）：

```
docker run --rm -v "$PWD:/w" <镜像> adapter lint /w/adapter.js
docker run --rm -v "$PWD:/w" <镜像> adapter test /w/adapter.js --fixture /w/fixture.json
```

跑不了就对着 `adapter.d.ts` 逐条过：`meta` 字段都在声明里；`buildSubmitRequest` 和 `parseSubmitResponse` 都有；`immediate` 与 `taskId` 二选一；有 `taskId` 就有 `buildQueryRequest` 和 `parseTaskResult`；状态词表覆盖文档全部值；密钥只从 `ctx.apiKey` 来；用到的 `input` 字段与 `kind` 一致；没有 `import / require / async / await / fetch`。

本地也能跑同一份离线诊断：`docker run --rm -v "$PWD:/w" <镜像> adapter diagnose /w/adapter.js --base-url <供应商 Base URL> --model-key <模型 Key>`，输出就是后台那份报告。

运营贴进后台后会：点「校验」（编译 + meta），再点「离线诊断」——系统按你的 `meta` 自动造样例输入（纯文本、带参考图、首尾帧、参考素材各一份），离线跑每个钩子，逐项检查提示词 / API Key / 模型 Key / 时长画质比例 / 每个素材是否进了请求、解析函数有没有凭空编 taskId 或猜状态。运营看不懂细节，只看 ✗；有 ✗ 就把整份报告原样贴给你。**报告里每一项都写了输入、输出和哪条检查没过，照着改完把完整脚本再给一遍**，不要只给补丁。你的输出应当一次通过校验和诊断。

## 七、输出格式

按这个顺序给：

1. ```` ```js ```` 围栏的 `adapter.js`
2. ```` ```json ```` 围栏的 `fixture.json`
3. 一段等价的 `curl`（人肉眼核对用：同样的 URL、头、body 形状）
4. 「上游价 → 站内积分」定价核对表
5. 「没做进去的能力和原因」「推断而非文档明确的地方」「Base URL 该填成什么」

## 八、一个完整示例

同步返回的 OpenAI images 风格中转，带参考图：

```js
export const meta = {
  apiVersion: 1,
  kind: "image",
  notes: "示例中转，文档 2026-09 版。Base URL 填 https://relay.example.com（不带 /v1）。",
  capabilities: { formTemplate: "openai", referenceImages: true, inpaint: false, customSize: false },
  inputs: { referenceImages: { max: 4 } },
  accept: ["image/png", "image/jpeg", "image/webp"],
  limits: { promptMaxChars: 4000 },
  pricing: { mode: "per_image", base: 2, tier: { "2K": 2, "4K": 6 } },
};

const SIZES = { "1:1": "1024x1024", "16:9": "1536x1024", "9:16": "1024x1536" };

export function buildSubmitRequest(ctx) {
  const { input } = ctx;
  if (!input.prompt) throw new Error("提示词不能为空");
  const body = { model: input.model, prompt: input.prompt, size: SIZES[input.aspectRatio] || "1024x1024", n: 1, response_format: "b64_json" };
  if (input.refs.length) body.image = input.refs.map((r) => ({ __ref: r.ref, encoding: "dataUrl" }));
  console.log("size", body.size);
  return {
    url: ctx.baseUrl + "/v1/images/generations",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json" },
    body,
  };
}

export function parseSubmitResponse(ctx, response) {
  const body = response.body || {};
  if (!Array.isArray(body.data) || !body.data.length) throw new Error("上游没有返回图片数据");
  const outputs = body.data.map((d) => (d.b64_json ? { base64: d.b64_json, mime: "image/png" } : { url: d.url }));
  return { immediate: { status: "SUCCESS", outputs } };
}
```

异步任务的写法看 `examples/dashscope-wan-task/`，multipart 文件上传看 `examples/openai-edits-multipart/`，Gemini 风格内联图看 `examples/gemini-generate-content/`。视频看 `examples/relay-video-percall/`（分辨率进模型名、首帧必填、一口价）和 `examples/relay-video-content/`（混合素材三类计数、`@素材名`、带鉴权头下载）。每个目录都带 `fixture.json`。
