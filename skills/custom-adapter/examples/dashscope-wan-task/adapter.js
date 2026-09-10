// 异步任务的完整示例（DashScope 通义万相风格）：提交拿 task_id，轮询 /tasks/{id}，
// 状态词表映射，认不出的返回 UNKNOWN；state 记录轮询次数，演示跨轮状态怎么用。
export const meta = {
  apiVersion: 1,
  kind: "image",
  notes: "示例：DashScope 文生图 / 图生图异步任务。Base URL 填 https://dashscope.aliyuncs.com（不带 /api/v1）。",
  capabilities: { formTemplate: "gemini", referenceImages: true, inpaint: false, customSize: false },
  inputs: { referenceImages: { max: 4 } },
  accept: ["image/png", "image/jpeg", "image/webp"],
  limits: { promptMaxChars: 800 },
  poll: { intervalMs: 2000 },
  pricing: { mode: "per_image", base: 3, tier: { "2K": 2 } },
};

function auth(ctx) {
  return { Authorization: "Bearer " + ctx.apiKey };
}

// 上游 size 写法是 W*H；系统给比例，按档位换算。
const SIZES = {
  "1K": { "1:1": "1024*1024", "16:9": "1280*720", "9:16": "720*1280", "4:3": "1152*864", "3:4": "864*1152" },
  "2K": { "1:1": "2048*2048", "16:9": "2560*1440", "9:16": "1440*2560", "4:3": "2304*1728", "3:4": "1728*2304" },
};

export function buildSubmitRequest(ctx) {
  const { input } = ctx;
  const table = SIZES[input.tier] || SIZES["1K"];
  const size = table[input.aspectRatio] || table["1:1"];
  const body = {
    model: input.model,
    input: { prompt: input.prompt },
    parameters: Object.assign({ size, n: 1 }, ctx.params),
  };
  if (input.refs.length) {
    // 上游接内联 data URL 的参考图数组
    body.input.images = input.refs.map((r) => ({ __ref: r.ref, encoding: "dataUrl", maxBytes: 10485760 }));
  }
  console.log("size", size);
  return {
    url: ctx.baseUrl + "/api/v1/services/aigc/text2image/image-synthesis",
    headers: Object.assign({ "Content-Type": "application/json", "X-DashScope-Async": "enable" }, auth(ctx)),
    body,
  };
}

export function parseSubmitResponse(ctx, response) {
  const body = response.body || {};
  const id = body.output && body.output.task_id;
  if (!id) throw new Error("上游没有返回 task_id" + (body.message ? ": " + body.message : ""));
  return { taskId: id, taskData: body, state: { polls: 0 } };
}

export function buildQueryRequest(ctx) {
  return { url: ctx.baseUrl + "/api/v1/tasks/" + encodeURIComponent(ctx.taskId), method: "GET", headers: auth(ctx) };
}

export function parseTaskResult(ctx, body) {
  const output = (body && body.output) || {};
  const table = { PENDING: "QUEUED", RUNNING: "IN_PROGRESS", SUCCEEDED: "SUCCESS", FAILED: "FAILURE", CANCELED: "FAILURE" };
  const status = table[output.task_status] || "UNKNOWN";
  const polls = ((ctx.state && ctx.state.polls) || 0) + 1;
  const r = { status, state: { polls } };
  if (status === "SUCCESS") {
    const results = Array.isArray(output.results) ? output.results : [];
    const urls = results.filter((x) => x && x.url).map((x) => ({ url: x.url }));
    if (!urls.length) throw new Error("上游报成功但没有结果地址");
    r.outputs = urls;
  }
  if (status === "FAILURE") r.reason = output.message || body.message || "上游任务失败";
  if (status === "UNKNOWN") r.reason = "unrecognized status: " + String(output.task_status);
  return r;
}
