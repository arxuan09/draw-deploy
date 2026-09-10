// 最简同步图片端点：OpenAI /v1/images/generations 风格，提交响应里就是结果。
// 参考图用 image 字段内联 data URL（有的中转这样收），没有参考图就是纯文生图。
export const meta = {
  apiVersion: 1,
  kind: "image",
  notes: "示例：OpenAI images 协议的同步中转。Base URL 填到 /v1 之前，例如 https://relay.example.com。",
  capabilities: { formTemplate: "openai", referenceImages: true, inpaint: false, customSize: false },
  inputs: { referenceImages: { max: 4 } },
  accept: ["image/png", "image/jpeg", "image/webp"],
  limits: { promptMaxChars: 4000 },
  // 示例数字：上游每张 0.1 元，站内 1 元 = 10 积分，毛利 1.5 倍 → 2 积分；2K / 4K 各加价。定价必须问用户。
  pricing: { mode: "per_image", base: 2, tier: { "2K": 2, "4K": 6 } },
};

// 上游只认这几个像素尺寸；系统给的是比例，查表转换，给不出的比例不要写。
const SIZES = { "1:1": "1024x1024", "16:9": "1536x1024", "9:16": "1024x1536", "3:2": "1536x1024", "2:3": "1024x1536" };

export function buildSubmitRequest(ctx) {
  const { input } = ctx;
  if (!input.prompt) throw new Error("提示词不能为空");
  const body = {
    model: input.model,
    prompt: input.prompt,
    size: SIZES[input.aspectRatio] || "1024x1024",
    n: 1,
    response_format: "b64_json",
  };
  if (input.refs.length) {
    // 占位符：JS 不碰字节，宿主发送前替换成 data URL（按 meta.accept 转码压缩）
    body.image = input.refs.map((r) => ({ __ref: r.ref, encoding: "dataUrl" }));
  }
  console.log("size", body.size, "refs", input.refs.length);
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
  // 同步上游：提交响应里就是结果。有的中转回 url 而不是 b64_json，两种都接。
  const outputs = body.data.map((d) => (d.b64_json ? { base64: d.b64_json, mime: "image/png" } : { url: d.url }));
  return { immediate: { status: "SUCCESS", outputs } };
}
