// 视频示例一：分辨率写进上游模型名、只支持图生视频（首帧必填）、按次一口价。
// 协议是常见中转的 "request_id 创建 + GET /v1/videos/{id} 轮询" 形态，成品是公开地址。
export const meta = {
  apiVersion: 1,
  kind: "video",
  notes: "示例：中转视频端点，720p / 1080p 各是一个上游模型名。Base URL 填到 /v1 之前。失败状态词按文档：failed / expired。",
  capabilities: { formTemplate: "openai" },
  inputs: { firstFrame: "required", lastFrame: "none" },
  limits: { promptMaxBytes: 4096 },
  poll: { intervalMs: 3000 },
  video: {
    durationRange: { min: 1, max: 15, step: 1 },
    aspectRatios: ["16:9", "9:16", "1:1"],
    resolutions: ["720p", "1080p"],
    materialRefSyntax: "",
  },
  // 示例数字：上游 720p 每次 2 元、1080p 每次 3 元；站内 1 元 = 10 积分；毛利 1.5 倍。定价必须问用户。
  pricing: { mode: "per_call", resolutions: { "720p": 30, "1080p": 45 } },
};

function auth(ctx) {
  return { Authorization: "Bearer " + ctx.apiKey };
}

export function buildSubmitRequest(ctx) {
  const { input } = ctx;
  if (!input.firstFrame) throw new Error("这个模型必须提供首帧图片");
  const body = {
    model: input.model + "-" + (input.resolution || "720p"),
    prompt: input.prompt,
    duration: input.duration,
    aspect_ratio: input.aspectRatio || "16:9",
    image: input.firstFrame.url, // 上游自己拉公网地址
  };
  console.log("model", body.model, "duration", body.duration);
  return { url: ctx.baseUrl + "/v1/videos", headers: Object.assign({ "Content-Type": "application/json" }, auth(ctx)), body };
}

export function parseSubmitResponse(ctx, response) {
  const id = response.body && response.body.request_id;
  if (!id) throw new Error("上游没有返回 request_id");
  return { taskId: id, taskData: response.body };
}

export function buildQueryRequest(ctx) {
  return { url: ctx.baseUrl + "/v1/videos/" + encodeURIComponent(ctx.taskId), method: "GET", headers: auth(ctx) };
}

export function parseTaskResult(ctx, body) {
  const table = { pending: "QUEUED", running: "IN_PROGRESS", done: "SUCCESS", failed: "FAILURE", expired: "FAILURE" };
  const status = table[body.status] || "UNKNOWN";
  const r = { status };
  if (typeof body.progress === "number") r.progress = body.progress;
  if (status === "SUCCESS") {
    const url = body.video && body.video.url;
    if (!url) throw new Error("上游报成功但没有视频地址");
    r.outputs = [{ url, mime: "video/mp4" }];
  }
  if (status === "FAILURE") r.reason = body.status === "expired" ? "任务已过期" : (body.error && body.error.message) || "上游生成失败";
  if (status === "UNKNOWN") r.reason = "unrecognized status: " + String(body.status);
  return r;
}
