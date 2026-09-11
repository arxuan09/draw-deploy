// adapter.d.ts — Draw Studio 自定义端点脚本契约 v1
//
// adapter.js 是一个 ES 模块，导出 `meta` 和几个同步纯函数。宿主（Go）负责发请求、
// 轮询、取素材、扣费；脚本只把系统的输入变成上游请求、把上游响应变成结果。
// 只允许用这里声明的东西。没有 fetch / require / setTimeout / async / await。

// ---------------------------------------------------------------------------
// 钩子
// ---------------------------------------------------------------------------

/** 必需。声明脚本是什么、能开哪些能力、素材上限、定价建议。必须是字面量对象。 */
export declare const meta: Meta;

/** 必需。把一次生成变成一个上游请求。 */
export declare function buildSubmitRequest(ctx: DriverContext): RequestDescriptor;

/** 必需。解析提交响应：要么给出任务 ID（之后轮询），要么直接给出结果。 */
export declare function parseSubmitResponse(ctx: DriverContext, response: UpstreamResponse): SubmitResult;

/** 有 taskId 时必需。拼一次轮询请求。 */
export declare function buildQueryRequest(ctx: TaskQueryContext): RequestDescriptor;

/** 有 taskId 时必需。解析轮询响应。认不出的状态必须返回 UNKNOWN，禁止兜底成 IN_PROGRESS。 */
export declare function parseTaskResult(ctx: TaskQueryContext, body: unknown, response: ResponseMeta): TaskResult;

/** 输出用 { content: true } 时必需：宿主拿这个描述符去下载成品（带鉴权头的下载地址）。 */
export declare function buildContentRequest(ctx: ContentContext): RequestDescriptor;

// ---------------------------------------------------------------------------
// 上下文
// ---------------------------------------------------------------------------

export interface DriverContext {
  baseUrl: string; // 供应商行的 Base URL，已去尾部 /
  apiKey: string; // 供应商行的 API Key，脚本自己决定放哪
  input: Input;
  params: Record<string, unknown>; // 模型行「高级默认参数」
  now: number; // Unix 秒；需要签名的上游用它，fixture 可固定
}

export interface TaskQueryContext {
  baseUrl: string;
  apiKey: string;
  taskId: string; // parseSubmitResponse 给的上游任务 ID
  model: string; // 模型行的 model_key
  data: unknown; // 最近一次成功解析的上游响应快照（宿主每轮覆盖）
  state: unknown; // 脚本自己的跨轮状态（只在钩子返回 state 时写入）
}

export interface ContentContext extends TaskQueryContext {
  output: Output; // 要下载的那个 { content: true } 输出
}

export interface Input {
  model: string; // 模型行的 model_key
  prompt: string; // 已拼好前缀的最终提示词（局部重绘带 inpaint 前缀）
  size: string; // 系统原始尺寸选项："1:1" | "1024x1024" | "auto"
  aspectRatio: string; // 归一后的比例（1:1 / 16:9 / 9:16 / 4:3 / 3:4 / 3:2 / 2:3 / 21:9 / 9:21），无法归一为 ""
  width: number; // 能算出时才非 0
  height: number;
  tier: "1K" | "2K" | "4K"; // 由 quality / 像素派生
  quality: string; // 模型行配置的质量值原样（1K / 2K / 4K 或 low / medium / high）
  outputFormat: string; // "png" | "jpeg" | "webp" | ""
  background: string;
  refs: Ref[]; // 参考图（局部重绘时 refs[0] 是原图）
  mask?: Ref; // 局部重绘的红色遮罩（没有公网 URL，只能用占位符）
  // 以下仅 kind: "video"（第三期）；图片脚本里它们是零值
  duration: number;
  resolution: string;
  generateAudio?: boolean; // 三态：undefined = 用户没选，不要发这个字段
  firstFrame?: Ref;
  lastFrame?: Ref;
  refImages: Ref[];
  refVideos: Ref[];
  refAudios: Ref[];
}

/** 一份素材的描述；字节永远不进 JS。 */
export interface Ref {
  ref: string; // 不透明句柄，用于占位符 / multipart parts[].ref
  // 提示词里对这份素材的称呼：「图片1」「视频1」「音频1」「首帧」「尾帧」。
  // 宿主统一编号，`ctx.input.prompt` 里的编号与它逐一对应。上游要求素材带
  // name 时**必须用它**，不要自己按下标生成——编号规则由端点决定，自己数会
  // 和提示词对不上。图片端点没有编号语法，该字段为空。
  name?: string;
  url: string; // 公网对象存储地址（上游自己去拉的场景直接用它）；遮罩为 ""
  mime: string; // 原图的 MIME；转码后的用占位符 "mime" 取
  size: number; // 字节数，未知为 0
  width: number; // 未知为 0
  height: number;
}

/**
 * 占位符：放在 JSON body 任意深度，或 multipart parts 里。宿主发送前替换。
 * base64 → 纯 base64；dataUrl → data:<mime>;base64,…；mime → 转码后的 MIME 字符串。
 * maxBytes 是这一处允许的最大字节数，宿主会按 meta.accept 转码压缩到这个以内。
 */
export type RefPlaceholder = { __ref: string; encoding: "base64" | "dataUrl" | "mime"; maxBytes?: number };

// ---------------------------------------------------------------------------
// 请求与响应
// ---------------------------------------------------------------------------

export interface RequestDescriptor {
  url: string; // 绝对地址；主机必须等于 baseUrl 的主机或 meta.allowedHosts 之一
  method?: "GET" | "POST" | "PUT" | "DELETE"; // 默认 POST
  headers?: Record<string, string>; // 禁止 Host / Content-Length / Transfer-Encoding
  body?: unknown; // JSON（默认），可含 RefPlaceholder
  bodyType?: "json" | "multipart";
  parts?: MultipartPart[]; // multipart：ref 走文件流
  timeoutMs?: number; // 上限为系统「上游超时」
}

export interface MultipartPart {
  name: string;
  value?: unknown; // 与 ref 二选一；对象会以 JSON 文本发送
  ref?: string; // 素材句柄，宿主以文件流发送
  filename?: string;
}

export interface UpstreamResponse {
  statusCode: number;
  headers: Record<string, string[]>;
  body: unknown; // JSON 解析失败则为字符串
}

export interface ResponseMeta {
  status: number;
  headers: Record<string, string[]>;
}

// ---------------------------------------------------------------------------
// 结果
// ---------------------------------------------------------------------------

/** parseSubmitResponse 的返回：taskId 与 immediate 二选一。 */
export interface SubmitResult {
  taskId?: string;
  taskData?: unknown; // 作为第一轮 TaskQueryContext.data
  state?: unknown;
  immediate?: TaskResult; // 提交响应里就是结果：status 必须是 SUCCESS 或 FAILURE
}

export type TaskStatus = "QUEUED" | "IN_PROGRESS" | "SUCCESS" | "FAILURE" | "UNKNOWN";

export interface TaskResult {
  status: TaskStatus;
  progress?: number; // 0–100
  reason?: string; // FAILURE / UNKNOWN 时的人话，会展示给用户
  retryable?: boolean; // FAILURE 时：限流 / 临时错误
  outputs?: Output[]; // SUCCESS 时必填，最多 32 个
  state?: unknown; // 写入 TaskQueryContext.state，≤ 16 KB
}

export type Output =
  | { url: string; mime?: string } // 公开可下载地址，宿主直接拉
  | { base64: string; mime: string }
  | { dataUrl: string }
  | { content: true; key?: string; mime?: string }; // 宿主调 buildContentRequest 去下载

// ---------------------------------------------------------------------------
// meta
// ---------------------------------------------------------------------------

export interface Meta {
  apiVersion: 1;
  kind: "image" | "video";
  notes?: string; // 文档来源与日期、推断说明，给人看
  allowedHosts?: string[]; // 除 Base URL 主机外允许脚本请求的主机（host 或 host:port）；结果地址不受此限
  capabilities: MetaCapabilities;
  inputs?: MetaInputs; // 素材槽位与上限：扣费前校验
  accept?: string[]; // 占位符转码目标 MIME（image/*），空 = 原样发不转码
  limits?: MetaLimits;
  poll?: MetaPoll;
  video?: MetaVideo; // kind: video 必填
  pricing?: Pricing; // 定价建议，只预填表单，扣费不读
}

export interface MetaCapabilities {
  formTemplate: "openai" | "gemini"; // 图片：可视化选项编辑器模板
  referenceImages?: boolean;
  inpaint?: boolean; // 只有文档明确支持 mask 才开
  customSize?: boolean; // 上游接受任意 WxH；扩图也靠它
}

export interface MetaInputs {
  referenceImages?: MetaRefSlot;
  // 视频
  firstFrame?: "required" | "optional" | "none";
  lastFrame?: "required" | "optional" | "none";
  refImages?: MetaRefSlot;
  refVideos?: MetaRefSlot;
  refAudios?: MetaRefSlot;
  refTotal?: number; // 三类合计上限
  framesExclusiveWithRefs?: boolean; // 首尾帧与参考素材不能同时给
  // 上游不能只靠提示词出片、但不限定给哪一种素材时写 true。
  // 某个槽位本身 required 的不用写；两者都没有 = 纯文生视频可用。
  materialRequired?: boolean;
}

export interface MetaRefSlot {
  max?: number;
  required?: boolean;
}

export interface MetaLimits {
  promptMaxChars?: number;
  promptMaxBytes?: number;
}

export interface MetaPoll {
  intervalMs?: number; // 500–60000，图片默认 3000
}

export interface MetaVideo {
  durations?: number[]; // 与 durationRange 二选一
  durationRange?: MetaDurationRange;
  aspectRatios: string[];
  resolutions: string[];
  materialRefSyntax?: "plain" | "at" | ""; // 提示词里指代素材的写法：图片1 / @图片1 / 无
  audioToggle?: boolean;
  referenceDurationLock?: number[]; // 带参考素材时只允许的时长
  referenceResolutionLock?: string[];
  referenceAspectRatioLock?: string[];
}

export interface MetaDurationRange {
  min: number;
  max: number;
  step?: number;
}

export type Pricing =
  | { mode: "per_image"; base: number; quality?: Record<string, number>; tier?: Record<"1K" | "2K" | "4K", number> }
  | { mode: "per_second"; resolutions: Record<string, number> } // 每个分辨率的每秒积分
  | { mode: "per_call"; resolutions: Record<string, number> }; // 每个分辨率的每次积分

// ---------------------------------------------------------------------------
// 运行时
// ---------------------------------------------------------------------------

/** console.log / warn / error 可用：输出进后台「适配器日志」。禁止打印 apiKey、headers、上游整个报文。 */
declare const console: { log(...args: unknown[]): void; warn(...args: unknown[]): void; error(...args: unknown[]): void };
