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
