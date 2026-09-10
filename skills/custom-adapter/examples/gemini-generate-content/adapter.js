// Gemini generateContent 风格：提示词和参考图拼成 contents[0].parts，参考图内联
// inline_data。mime_type 用 "mime" 占位符取转码后的实际类型，data 用 "base64"。
// 响应从 candidates[].content.parts[].inlineData 取图。
export const meta = {
  apiVersion: 1,
  kind: "image",
  notes: "示例：Gemini 风格图片端点。Base URL 填到 /v1beta 之前。",
  capabilities: { formTemplate: "gemini", referenceImages: true, inpaint: false, customSize: false },
  inputs: { referenceImages: { max: 6 } },
  accept: ["image/webp", "image/png", "image/jpeg"],
  limits: { promptMaxChars: 8000 },
  pricing: { mode: "per_image", base: 4, tier: { "2K": 4, "4K": 12 } },
};

export function buildSubmitRequest(ctx) {
  const { input } = ctx;
  const parts = [{ text: input.prompt }];
  input.refs.forEach((r) =>
    parts.push({ inline_data: { mime_type: { __ref: r.ref, encoding: "mime" }, data: { __ref: r.ref, encoding: "base64" } } }),
  );
  const generationConfig = { responseModalities: ["IMAGE"], imageConfig: { imageSize: input.tier } };
  if (input.aspectRatio) generationConfig.imageConfig.aspectRatio = input.aspectRatio;
  return {
    url: ctx.baseUrl + "/v1beta/models/" + encodeURIComponent(input.model) + ":generateContent",
    headers: { "x-goog-api-key": ctx.apiKey, "Content-Type": "application/json" },
    body: { contents: [{ role: "user", parts }], generationConfig },
  };
}

export function parseSubmitResponse(ctx, response) {
  const body = response.body || {};
  const candidates = Array.isArray(body.candidates) ? body.candidates : [];
  const outputs = [];
  candidates.forEach((c) => {
    const parts = (c.content && c.content.parts) || [];
    parts.forEach((p) => {
      const inline = p.inlineData || p.inline_data;
      if (inline && inline.data) outputs.push({ base64: inline.data, mime: inline.mimeType || inline.mime_type || "image/png" });
    });
  });
  if (!outputs.length) {
    const reason = body.promptFeedback && body.promptFeedback.blockReason;
    throw new Error(reason ? "上游拒绝了这条提示词: " + reason : "上游没有返回图片");
  }
  return { immediate: { status: "SUCCESS", outputs } };
}
