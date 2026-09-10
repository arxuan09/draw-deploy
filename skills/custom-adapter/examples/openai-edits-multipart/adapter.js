// OpenAI images 协议的完整版：没有参考图走 JSON 的 /images/generations，
// 有参考图或遮罩走 multipart 的 /images/edits（image[] 文件 + 可选 mask 文件）。
// 素材用 parts[].ref 走文件流，字节不进脚本。
export const meta = {
  apiVersion: 1,
  kind: "image",
  notes: "示例：OpenAI images generations + edits（multipart）。Base URL 填到 /v1 之前。",
  capabilities: { formTemplate: "openai", referenceImages: true, inpaint: true, customSize: true },
  inputs: { referenceImages: { max: 10 } },
  accept: ["image/webp", "image/png", "image/jpeg"],
  limits: { promptMaxChars: 32000 },
  pricing: { mode: "per_image", base: 3, tier: { "2K": 3, "4K": 9 } },
};

function auth(ctx) {
  return { Authorization: "Bearer " + ctx.apiKey };
}

// 上游接任意 16 的倍数的 WxH；系统给了具体像素就直接发，只给了比例就查表。
const RATIO_SIZES = { "1:1": "1024x1024", "16:9": "1536x1024", "9:16": "1024x1536", "3:2": "1536x1024", "2:3": "1024x1536" };

function sizeOf(input) {
  if (input.width && input.height) return input.width + "x" + input.height;
  return RATIO_SIZES[input.aspectRatio] || "1024x1024";
}

export function buildSubmitRequest(ctx) {
  const { input, params } = ctx;
  const useEdits = input.refs.length > 0 || !!input.mask;
  if (!useEdits) {
    const body = Object.assign({}, params, {
      model: input.model,
      prompt: input.prompt,
      size: sizeOf(input),
      quality: input.quality,
      n: 1,
    });
    return { url: ctx.baseUrl + "/v1/images/generations", headers: Object.assign({ "Content-Type": "application/json" }, auth(ctx)), body };
  }
  const parts = [
    { name: "model", value: input.model },
    { name: "prompt", value: input.prompt },
    { name: "size", value: sizeOf(input) },
    { name: "quality", value: input.quality },
    { name: "n", value: 1 },
  ];
  input.refs.forEach((r, i) => parts.push({ name: "image[]", ref: r.ref, filename: "image_" + (i + 1) + ".png" }));
  if (input.mask) parts.push({ name: "mask", ref: input.mask.ref, filename: "mask.png" });
  console.log("edits parts", parts.length);
  return { url: ctx.baseUrl + "/v1/images/edits", bodyType: "multipart", headers: auth(ctx), parts };
}

export function parseSubmitResponse(ctx, response) {
  const body = response.body || {};
  const first = Array.isArray(body.data) && body.data[0];
  if (!first) throw new Error("上游没有返回图片数据");
  const output = first.b64_json ? { base64: first.b64_json, mime: "image/png" } : { url: first.url };
  return { immediate: { status: "SUCCESS", outputs: [output] } };
}
