# 供应商与模型

## 概念

- **供应商**（provider）= 一个上游账号：Base URL + API Key。只管「连到哪、用什么 Key」，不管请求格式。
- **端点家族**（endpoint）= 请求格式，写死在站点代码里：OpenAI images、Gemini generateContent、DashScope 万相、火山方舟 Seedream / Seedance、MiniMax、Veo，以及「通用协议」。`draw_admin.py models endpoints` 列出全部，新建模型时填 `endpointKey`。
- **模型** = 供应商 + 端点家族 + 上游模型名（`modelKey`）+ 用户能选的质量 / 尺寸 / 积分价格。

同一个中转站（new-api、one-api 一类）可以同时承载 OpenAI 格式和 Gemini 格式的模型：只建一个供应商，模型各自选端点家族。

## 接供应商

问运营方：上游是官方还是中转站、Base URL（必填，没有默认值）、API Key、并发上限（不限就不填）。

```bash
draw_admin.py providers create --name "RollDek 中转" --base-url https://api.example.com --secret   # 运营方在自己终端运行
# 或：运营方先 export ROLLDEK_KEY=…，你再
draw_admin.py providers create --name "RollDek 中转" --base-url https://api.example.com --from-env ROLLDEK_KEY
draw_admin.py providers test p-k3x9m2      # create 打印出的标识
```

供应商标识由站点生成（形如 `p-k3x9m2`），建好后不变，`providers list` 里能查到。换 Key：`draw_admin.py providers set-key p-k3x9m2 --secret`。

Base URL 填到域名即可，带不带 `/v1`（Gemini 是 `/v1beta`）都行，站点会自动补齐端点路径。

## 上模型

**优先用预设。** `draw_admin.py models presets` 列出站点内置的常见模型配置（GPT Image 2 / 2.5、Nano Banana 系列、万相、Seedream、各视频模型）。预设里有推荐的质量档、比例、思考强度。

最稳的做法是照着一个现有模型改：

```bash
draw_admin.py models list --json              # 找一个同端点家族的模型，看完整字段
draw_admin.py call models/duplicate --data '{"modelId":"<现有模型ID>"}'
draw_admin.py models list --json              # 找到副本的 ID
draw_admin.py call models/update --data-file model.json    # 改 name、modelKey、providerKey、价格等
```

`models/update` 要带完整字段（`describe models/update` 看全部字段）：把 `models list --json` 里那条模型的 JSON 存成文件，`id` 改名为 `modelId`、补上 `enabled`，再改要改的字段。

从零新建用 `call models/create`，必填 `providerKey`、`endpointKey`、`name`、`modelKey`、`creditCost`。图片模型常用字段：

| 字段 | 说明 |
| --- | --- |
| `kind` | `image` / `video` / `text` |
| `modelKey` | 上游的模型名，如 `gpt-image-2` |
| `creditCost` | 基础积分（每张 / 每次） |
| `imageQualities` / `qualityCredits` | 质量档和每档积分 |
| `imageSizes` / `imageSizeCredits` | 比例列表，1K / 2K / 4K 档位附加积分 |
| `maxReferenceImages` | 参考图上限，留空跟随全局；填 0 会同时禁用局部重绘 |
| `transparentBackgroundEnabled` / `outpaintEnabled` / `customSizeEnabled` | 按端点家族的能力开，端点不支持的别开 |
| `upstreamCostCents` | 上游成本（分），只用于成本看板 |
| `hiddenFromUsers` | 只给 Agent 或内部用、不在用户模型列表里显示 |

### 新站：没有可复制的模型

`models presets` 只给能力（模型 Key、质量档、比例、参考图上限、能否扩图 / 透明 / 拆分），不给价格；把它和下面三种骨架拼起来就是完整请求体。没列出的字段照 `describe models/create` 补默认值（布尔 false、数组 []、对象 {}）。

OpenAI 表单（`formTemplate: openai` 的预设不带 `endpoint`：官方或完整透传的中转用 `openai-image-generations`，会拒绝 `response_format` 的中转用 `openai-image-generations-no-format`）：

```json
{"providerKey":"p-xxxxxx","endpointKey":"openai-image-generations","kind":"image","name":"GPT Image 2.5 Flare",
 "modelKey":"gpt-image-2.5-flare","creditCost":5,
 "imageQualities":[{"quality":"auto","credits":12},{"quality":"low","credits":3},{"quality":"medium","credits":5},{"quality":"high","credits":12},{"quality":"xhigh","credits":22},{"quality":"max","credits":50}],
 "imageSizes":[{"size":"auto","label":"Auto"},{"size":"1:1"},{"size":"16:9"},{"size":"9:16"}],
 "imageSizeCredits":{"1K":0,"2K":10,"4K":25,"auto":0},
 "outpaintEnabled":true,"customSizeEnabled":true,"transparentBackgroundEnabled":true,"maxReferenceImages":16,"enabled":true}
```

档位表单（Nano Banana、Seedream、万相，预设带 `endpoint`）：质量档就是分辨率档，价格写在 `qualityCredits`，比例写进 `imageSizes`：

```json
{"providerKey":"p-xxxxxx","endpointKey":"gemini-generate-content","kind":"image","name":"Nano Banana 2.1",
 "modelKey":"gemini-nano-banana-2.1","creditCost":8,
 "qualityLevels":["1K","2K","4K"],"qualityCredits":{"1K":8,"2K":12,"4K":25},
 "imageSizes":[{"size":"1:1"},{"size":"16:9"}],"maxReferenceImages":14,
 "thinkingLevels":["minimal","medium","high"],"thinkingDefault":"medium","enabled":true}
```

图层拆分模型（Seedream 5.0）再加 `"layersEnabled":true,"layersCreditCost":10`，只给拆分用就加 `"hiddenFromUsers":true`，然后把 `layers_model_id` 设成它。

视频（`videoPresets` 带 `endpointKey`）：时长和比例由端点固定，传空；`resolutions` 每行填每秒积分，`capabilities` 原样拷进 `videoCapabilities`（去掉值为 false / 0 / 空的项即可）：

```json
{"providerKey":"p-xxxxxx","endpointKey":"ark-video-seedance","kind":"video","name":"Seedance 2.0 Fast",
 "modelKey":"doubao-seedance-2-0-fast-260128","creditCost":0,
 "videoDurations":[],"videoDurationRange":{},"videoAspectRatios":[],
 "videoResolutionMap":[{"resolution":"480p","creditsPerSecond":15},{"resolution":"720p","creditsPerSecond":30}],
 "videoCapabilities":{"textToVideo":true,"firstFrame":true,"lastFrame":true,"maxReferenceImages":9,"maxReferenceVideos":3,"maxReferenceAudios":3,"defaultDuration":5,"defaultResolution":"720p","blocksRealFace":true},
 "videoBadges":["omni_reference","first_last_frame"],"videoBillingMode":"per_second","enabled":true}
```

`models/create` 不是幂等的：网络错误后先 `models list` 看是否已经建上，再决定重试。Logo 写 `logoSvg`（SVG 源码，单色图标会跟随主题色）；推荐 `@lobehub/icons-static-svg` 的图标，彩色版优先，去掉 `width="1em" height="1em"`。

每个模型的质量档、分辨率、参考图上限：https://raw.githubusercontent.com/arxuan09/draw-deploy/main/model-config-reference.md 。定价问运营方，可以按「上游成本 × 期望毛利 ÷ 每积分售价」给建议。

建好后：

```bash
draw_admin.py models ping <模型ID>
draw_admin.py models toggle <模型ID> on
```

**`models ping` 会真的调用上游、消耗少量额度**，先告诉运营方。

## 站点代码没适配的上游

中转站、新厂商、自建服务不要硬套现有端点家族。站点有「通用协议」端点（`draw-protocol-image` / `draw-protocol-video`）：运营方部署一个协议转换器，供应商的 Base URL 指向转换器。怎么写转换器见 draw-protocol-converter 技能：https://github.com/arxuan09/draw-deploy/tree/main/skills/draw-protocol-converter 。

## 探测失败怎么查

`models ping` 或 `doctor` 显示失败时，看 `errorMessage` 和 `httpStatus`：

| 现象 | 多半是 |
| --- | --- |
| 401 / 403 | API Key 错或没有该模型权限 |
| 404 | Base URL 多了或少了 `/v1`，或 `modelKey` 拼错 |
| 400 + 参数报错 | 开了端点不支持的能力（透明背景、某个比例、质量档） |
| 429 | 上游限流或余额不足 |
| 超时 | 上游慢或网络不通；万相、Seedance 等异步端点本来就慢 |

查完再看 `draw_admin.py logs query --level error --since 1h --keyword <模型名或供应商>` 里的完整报错。

## 文本模型的用途

Agent、提示词优化、反推、提示词审查都用文本模型（端点 `openai-responses` / `openai-chat-completions`）。相关设置：

```bash
draw_admin.py settings list --group agent
draw_admin.py settings list --group prompts
```
