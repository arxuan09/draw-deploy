// 视频示例二：混合素材（图 / 视频 / 音频三类分开计数）、提示词用 @素材名 指代、
// 成品要带鉴权头从 /content 下载（buildContentRequest）。协议是 OpenAI videos 风格。
export const meta = {
  apiVersion: 1,
  kind: "video",
  notes: "示例：OpenAI videos 风格中转，全能参考。Base URL 填到 /v1 之前。失败状态词文档未给，按 OpenAI videos 协议推断为 failed / cancelled。",
  capabilities: { formTemplate: "openai" },
  inputs: {
    firstFrame: "optional",
    lastFrame: "optional",
    refImages: { max: 9 },
    refVideos: { max: 3 },
    refAudios: { max: 3 },
    refTotal: 9,
  },
  limits: { promptMaxChars: 5000 },
  poll: { intervalMs: 6000 },
  video: {
    durationRange: { min: 4, max: 15, step: 1 },
    aspectRatios: ["16:9", "9:16", "1:1", "21:9", "4:3", "3:4"],
    resolutions: ["720p", "1080p"],
    materialRefSyntax: "at",
  },
  // 示例数字：上游 720p 每秒 0.5 元、1080p 每秒 0.7 元；站内 1 元 = 10 积分；毛利 1.5 倍，向上取整。定价必须问用户。
  pricing: { mode: "per_second", resolutions: { "720p": 8, "1080p": 11 } },
};

function auth(ctx) {
  return { Authorization: "Bearer " + ctx.apiKey };
}

export function buildSubmitRequest(ctx) {
  const { input } = ctx;
  if (input.refAudios.length && !input.refImages.length && !input.refVideos.length && !input.firstFrame)
    throw new Error("参考音频不能单独使用，请同时提供参考图或参考视频");
  const body = {
    model: input.model + "-" + (input.resolution || "720p"),
    prompt: input.prompt,
    duration: input.duration,
    aspect_ratio: input.aspectRatio || "16:9",
  };
  if (input.firstFrame) body.start_image_url = input.firstFrame.url;
  if (input.lastFrame) body.end_image_url = input.lastFrame.url;
  if (input.refImages.length === 1) body.image_url = input.refImages[0].url;
  if (input.refImages.length > 1) body.image_urls = input.refImages.map((r) => r.url);
  if (input.refVideos.length) body.video_reference = input.refVideos.map((r) => ({ url: r.url }));
  if (input.refAudios.length) body.audio_reference = input.refAudios.map((r) => ({ url: r.url }));
  console.log("model", body.model, "materials", input.refImages.length, input.refVideos.length, input.refAudios.length);
  return { url: ctx.baseUrl + "/v1/videos", headers: Object.assign({ "Content-Type": "application/json" }, auth(ctx)), body };
}

export function parseSubmitResponse(ctx, response) {
  const id = response.body && response.body.id;
  if (!id) throw new Error("上游没有返回任务 id");
  return { taskId: id, taskData: response.body };
}

export function buildQueryRequest(ctx) {
  return { url: ctx.baseUrl + "/v1/videos/" + encodeURIComponent(ctx.taskId), method: "GET", headers: auth(ctx) };
}

export function parseTaskResult(ctx, body) {
  const table = { queued: "QUEUED", in_progress: "IN_PROGRESS", completed: "SUCCESS", failed: "FAILURE", cancelled: "FAILURE" };
  const status = table[body.status] || "UNKNOWN";
  const r = { status };
  if (typeof body.progress === "number") r.progress = body.progress;
  if (status === "SUCCESS") r.outputs = [{ content: true, mime: "video/mp4" }]; // 成品要带鉴权头下载
  if (status === "FAILURE") r.reason = (body.error && body.error.message) || "上游生成失败";
  if (status === "UNKNOWN") r.reason = "unrecognized status: " + String(body.status);
  return r;
}

export function buildContentRequest(ctx) {
  return { url: ctx.baseUrl + "/v1/videos/" + encodeURIComponent(ctx.taskId) + "/content", method: "GET", headers: auth(ctx) };
}
