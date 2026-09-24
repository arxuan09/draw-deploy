// 通用生成协议 v1 · 协议转换器示例（Go，只用标准库，Go ≥ 1.22）
//
// 这是一个完整可运行的示例：
//
//	server.go          与上游无关的部分：路由、取 Key、请求体解析、模型校验、
//	                   预演（dry-run）拦截、错误格式、读 .env。
//	adapter.go         「协议 ↔ 上游」的翻译：能力声明，以及把协议请求翻译成上游请求、
//	                   把上游响应翻译回来的几个函数。换一个上游只需要重写它。
//	converter_test.go  代码层面的自查：用模拟上游把 SKILL.md 第三节的清单逐条测一遍。
//	.env.example       配置模板，复制成 .env 填好即可启动。
//
// 用别的语言写时照这个结构来，SKILL.md 第三节的每一条在这里都有对应。
//
// 运行（Windows / Linux / macOS 一样）：
//
//	复制 .env.example 为 .env，填好里面的值
//	go test ./...       自查
//	go run .            启动；或 go build 出可执行文件（Windows 上是 converter.exe），和 .env 放在一起运行
//
// 也可以不用 .env，直接设环境变量或用 Dockerfile 打镜像；已设置的环境变量优先于 .env。
//
// 配置项：
//
//	UPSTREAM_BASE_URL  上游地址。
//	PORT               监听端口，默认 8787（0 = 随机端口）。
//	HOST               监听地址，默认 0.0.0.0。
//
// 转换器不存任何密钥（protocol.md 1.1）：Draw Studio 后台供应商的 API Key 直接填上游的 Key，
// 每个请求带着它来，转换器原样拿去调上游（c.APIKey）。所以一份转换器可以给多个站点、多个上游
// 账号共用；Key 要经网络到达转换器，部署时走 HTTPS，或和 Draw Studio 放在同一台机器 / 同一内网。
//
// 协议全文见上一级的 protocol.md。
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
// 协议类型（protocol.md 第 3 节）
// ---------------------------------------------------------------------------

type ImageSize struct {
	Raw    string  `json:"raw"`
	Ratio  *string `json:"ratio"`
	Tier   *string `json:"tier"`
	Width  *int    `json:"width"`
	Height *int    `json:"height"`
}

type ImageRef struct {
	URL      string `json:"url"`
	MimeType string `json:"mimeType,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

type ImageRequest struct {
	RequestID    string     `json:"requestId"`
	Model        string     `json:"model"`
	Intent       string     `json:"intent"`
	Prompt       string     `json:"prompt"`
	Size         ImageSize  `json:"size"`
	Quality      string     `json:"quality"`
	OutputFormat string     `json:"outputFormat,omitempty"`
	Background   string     `json:"background,omitempty"`
	References   []ImageRef `json:"references"`
	Mask         *struct {
		DataURL string `json:"dataUrl"`
	} `json:"mask"`
	Layers *struct {
		Count int `json:"count"`
	} `json:"layers"`
	Params map[string]any `json:"params"`
}

type LayerInfo struct {
	ZIndex      int    `json:"zIndex"`
	BBox        []int  `json:"bbox,omitempty"`
	BBoxPx      []int  `json:"bboxPx,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type ImageResult struct {
	URL           string     `json:"url,omitempty"`
	B64           string     `json:"b64,omitempty"`
	MimeType      string     `json:"mimeType,omitempty"`
	RevisedPrompt string     `json:"revisedPrompt,omitempty"`
	Layer         *LayerInfo `json:"layer,omitempty"`
}

// ImageResponse：同步成功填 Images；异步填 TaskID（回 202）；查询填 Status。
type ImageResponse struct {
	Images         []ImageResult  `json:"images,omitempty"`
	Usage          map[string]any `json:"usage,omitempty"`
	TaskID         string         `json:"taskId,omitempty"`
	PollIntervalMs int            `json:"pollIntervalMs,omitempty"`
	Status         string         `json:"status,omitempty"`
	Progress       int            `json:"progress,omitempty"`
	Error          *ErrorBody     `json:"error,omitempty"`
}

type MediaRef struct {
	URL string `json:"url"`
}

type VideoRequest struct {
	RequestID     string    `json:"requestId"`
	Model         string    `json:"model"`
	Prompt        string    `json:"prompt"`
	Duration      int       `json:"duration"`
	Ratio         string    `json:"ratio"`
	Resolution    string    `json:"resolution"`
	GenerateAudio *bool     `json:"generateAudio"`
	FirstFrame    *MediaRef `json:"firstFrame"`
	LastFrame     *MediaRef `json:"lastFrame"`
	References    struct {
		Images []string `json:"images"`
		Videos []string `json:"videos"`
		Audios []string `json:"audios"`
	} `json:"references"`
	Params map[string]any `json:"params"`
}

type VideoResponse struct {
	TaskID   string         `json:"taskId,omitempty"`
	Status   string         `json:"status,omitempty"`
	Progress int            `json:"progress,omitempty"`
	VideoURL string         `json:"videoUrl,omitempty"`
	Usage    map[string]any `json:"usage,omitempty"`
	Error    *ErrorBody     `json:"error,omitempty"`
}

type ErrorBody struct {
	Category string `json:"category"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
}

// Adapter 是 adapter.go 提供的东西。不支持的能力留 nil。
type Adapter struct {
	// Capabilities 是 GET /capabilities 的 JSON 原文（protocol.md 第 2 节）。
	// downloadWithKey 不用写：server.go 会给每个模型补上 UPSTREAM_BASE_URL 的源和下面的 DownloadWithKey。
	Capabilities  string
	GenerateImage func(c *Ctx, req *ImageRequest) (*ImageResponse, error)
	QueryImage    func(c *Ctx, taskID string) (*ImageResponse, error)
	SubmitVideo   func(c *Ctx, req *VideoRequest) (*VideoResponse, error)
	QueryVideo    func(c *Ctx, taskID string) (*VideoResponse, error)
	// DownloadWithKey：结果链接还会落在哪些别的源（如 "https://files.upstream.com"）上、
	// 并且要带 Key 才能下载。UPSTREAM_BASE_URL 的源总是算在内，不用写。
	DownloadWithKey []string
}

// ---------------------------------------------------------------------------
// 错误（protocol.md 第 4 节）
// ---------------------------------------------------------------------------

// ProtocolError 的 Category 决定用户看到什么、模型状态页怎么判。
type ProtocolError struct {
	Category string
	Code     string
	Message  string
	Status   int
}

func (e *ProtocolError) Error() string { return e.Category + ": " + e.Message }

var defaultStatus = map[string]int{"user_input": 400, "moderation": 400, "busy": 429, "upstream": 502, "converter": 500}

// Fail 构造一个协议错误，HTTP 状态按分类取默认值。
func Fail(category, message string) *ProtocolError {
	status := defaultStatus[category]
	if status == 0 {
		status = 500
	}
	return &ProtocolError{Category: category, Message: message, Status: status}
}

// TaskNotFound：查询不存在的任务回 404，Draw Studio 据此立即判失败并退积分。
func TaskNotFound(message string) *ProtocolError {
	return &ProtocolError{Category: "user_input", Message: message, Status: http.StatusNotFound}
}

var moderationRe = regexp.MustCompile(`(?i)moderation|sensitive|content.?policy|safety|违规|审核|敏感`)

// UpstreamError 把上游的非 2xx 响应翻成协议错误：内容审核字样 → moderation，
// 429 → busy，400 / 422 → user_input，其余 → upstream。上游 5xx / 429 保留原状态码
// （查询接口这样回，Draw Studio 会重试）。上游有明确错误码时先自己分类。
func UpstreamError(status int, body []byte) *ProtocolError {
	text := truncate(string(body), 600)
	var e *ProtocolError
	switch {
	case moderationRe.MatchString(text):
		e = Fail("moderation", "")
	case status == http.StatusTooManyRequests:
		e = Fail("busy", "")
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		e = Fail("user_input", "")
	default:
		e = Fail("upstream", "")
	}
	if status >= 500 || status == http.StatusTooManyRequests {
		e.Status = status
	}
	e.Message = fmt.Sprintf("upstream %d: %s", status, text)
	return e
}

// EncodeTaskID 把上游任务 ID 等状态编进协议 taskId：转换器无状态，重启不丢任务。
func EncodeTaskID(v map[string]string) string {
	raw, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodeTaskID 是 EncodeTaskID 的反操作；解不开就是不认识的任务 → 404。
func DecodeTaskID(taskID string) (map[string]string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(taskID)
	var v map[string]string
	if err != nil || json.Unmarshal(raw, &v) != nil {
		return nil, TaskNotFound("unknown taskId")
	}
	return v, nil
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// ---------------------------------------------------------------------------
// Ctx：每个请求一份，adapter 通过它访问上游
// ---------------------------------------------------------------------------

// UpstreamRequest 是预演时回报的一条上游请求。
type UpstreamRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    any               `json:"body,omitempty"`
}

type Ctx struct {
	// DryRun：预演。此时 Upstream 不发请求，记录后返回 errDryRun，把记录回报给 Draw Studio。
	DryRun bool
	// APIKey：上游的 Key，就是 Draw Studio 这次请求带来的 Key。调上游一律用它。
	APIKey   string
	ctx      context.Context
	recorded []UpstreamRequest
}

var errDryRun = errors.New("dry-run: upstream request recorded")

var upstreamClient = &http.Client{Timeout: 10 * time.Minute}

// UpstreamResponse 是上游的响应，Body 已读完。
type UpstreamResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

func (r *UpstreamResponse) OK() bool { return r.Status >= 200 && r.Status < 300 }

// Upstream 调上游：jsonBody 非 nil 时以 JSON 发送。所有上游请求都走这里——
// 预演时它只记录、返回 errDryRun，adapter 把这个错误原样 return 即可。
// 非 2xx 不报错，由 adapter 判断。
func (c *Ctx) Upstream(method, rawURL string, headers map[string]string, jsonBody any) (*UpstreamResponse, error) {
	var body []byte
	if jsonBody != nil {
		var err error
		if body, err = json.Marshal(jsonBody); err != nil {
			return nil, Fail("converter", err.Error())
		}
		headers = withHeader(headers, "Content-Type", "application/json")
	}
	return c.send(method, rawURL, headers, body, jsonBody)
}

// UpstreamRaw 发任意 body（如 multipart）。dryRunBody 是预演时回报的 body 描述。
func (c *Ctx) UpstreamRaw(method, rawURL string, headers map[string]string, body []byte, dryRunBody any) (*UpstreamResponse, error) {
	return c.send(method, rawURL, headers, body, dryRunBody)
}

func (c *Ctx) send(method, rawURL string, headers map[string]string, body []byte, record any) (*UpstreamResponse, error) {
	if c.DryRun {
		c.recorded = append(c.recorded, UpstreamRequest{Method: method, URL: rawURL, Headers: maskHeaders(headers), Body: record})
		return nil, errDryRun
	}
	req, err := http.NewRequestWithContext(c.ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, Fail("converter", err.Error())
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := upstreamClient.Do(req)
	if err != nil {
		return nil, &ProtocolError{Category: "upstream", Message: "upstream request failed: " + err.Error(), Status: http.StatusBadGateway}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, &ProtocolError{Category: "upstream", Message: "reading upstream response: " + err.Error(), Status: http.StatusBadGateway}
	}
	return &UpstreamResponse{Status: res.StatusCode, Header: res.Header, Body: raw}, nil
}

// materialClient 只连公网地址。转换器不校验 Key（谁带任意 Key 都能调），素材地址又来自请求体，
// 不拦的话别人能借转换器去访问内网、本机和云主机的元数据接口。素材本来就是 Draw Studio
// 对象存储的公网链接（protocol.md 1.4），挡掉内网地址不影响正常使用。每次连接（包括重定向）
// 都按解析出的 IP 判断，域名解析到内网也挡得住。
var materialClient = &http.Client{
	Timeout: 5 * time.Minute,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{Timeout: 30 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if ip := net.ParseIP(host); err != nil || ip == nil || !publicIP(ip) {
				return errPrivateMaterial
			}
			return nil
		}}).DialContext,
	},
}

var errPrivateMaterial = errors.New("material url must point to a public address")

var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

func publicIP(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() &&
		!ip.IsUnspecified() && !ip.IsMulticast() && !cgnat.Contains(ip)
}

// FetchMaterial 下载素材（参考图等），返回字节和 MIME（上游要文件时用）。只下载公网 http(s) 地址。
// 预演时不下载，返回占位内容。
func (c *Ctx) FetchMaterial(rawURL string) ([]byte, string, error) {
	if c.DryRun {
		return []byte("dry-run-placeholder"), "application/octet-stream", nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, "", Fail("user_input", "bad material url: "+rawURL)
	}
	req, err := http.NewRequestWithContext(c.ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", Fail("user_input", "bad material url: "+rawURL)
	}
	res, err := materialClient.Do(req)
	if errors.Is(err, errPrivateMaterial) {
		return nil, "", Fail("user_input", "material url must point to a public address")
	}
	if err != nil {
		return nil, "", Fail("user_input", "material download failed: "+err.Error())
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", Fail("user_input", fmt.Sprintf("material download returned %d: %s", res.StatusCode, rawURL))
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 100<<20))
	if err != nil {
		return nil, "", Fail("user_input", "material download failed: "+err.Error())
	}
	mimeType, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return raw, mimeType, nil
}

// MaterialDataURL 下载素材并转成 data URL（上游要 base64 时用）。
func (c *Ctx) MaterialDataURL(rawURL string) (string, error) {
	raw, mimeType, err := c.FetchMaterial(rawURL)
	if err != nil {
		return "", err
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
}

var secretHeaderRe = regexp.MustCompile(`(?i)auth|key|token|secret|signature`)

// maskHeaders 把疑似密钥的请求头打码，预演回报里不能出现完整密钥。
func maskHeaders(h map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range h {
		if secretHeaderRe.MatchString(k) && v != "" {
			v = v[:min(6, len(v)/2)] + "***"
		}
		out[k] = v
	}
	return out
}

func withHeader(h map[string]string, k, v string) map[string]string {
	out := map[string]string{k: v}
	for hk, hv := range h {
		out[hk] = hv
	}
	return out
}

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	var pe *ProtocolError
	if errors.As(err, &pe) {
		writeJSON(w, pe.Status, map[string]any{"error": ErrorBody{Category: pe.Category, Code: pe.Code, Message: pe.Message}})
		return
	}
	log.Printf("converter error: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": ErrorBody{Category: "converter", Message: err.Error()}})
}

type server struct {
	adapter      Adapter
	capabilities []byte            // adapter.Capabilities，每个模型补上了 downloadWithKey
	models       map[string]string // model → kind
}

// bearer 取请求里 Authorization: Bearer 后面的 Key；不是 Bearer 形式的一律当没带。
func bearer(r *http.Request) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// checkModel：不在能力声明里的模型、或种类不对，直接 400，不调上游。
func (s *server) checkModel(model, kind string) error {
	got, ok := s.models[model]
	if !ok {
		return &ProtocolError{Category: "user_input", Code: "unknown_model", Message: fmt.Sprintf("unknown model %q", model), Status: 400}
	}
	if got != kind {
		return &ProtocolError{Category: "user_input", Code: "wrong_kind", Message: fmt.Sprintf("model %s is a %s model", model, got), Status: 400}
	}
	return nil
}

func (s *server) newCtx(r *http.Request) *Ctx {
	return &Ctx{DryRun: r.Header.Get("X-Draw-Dry-Run") == "1", APIKey: bearer(r), ctx: r.Context()}
}

func decodeBody(r *http.Request, v any) error {
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(v); err != nil {
		return Fail("user_input", "request body is not valid JSON")
	}
	return nil
}

// respond 统一处理 adapter 的返回：预演 → 回报记录；错误 → 协议错误格式。
func respond(w http.ResponseWriter, c *Ctx, status int, result any, err error) {
	if errors.Is(err, errDryRun) {
		writeJSON(w, http.StatusOK, map[string]any{"dryRun": true, "upstreamRequests": c.recorded})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, status, result)
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/capabilities", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(s.capabilities)
	})
	mux.HandleFunc("POST /v1/images/generate", func(w http.ResponseWriter, r *http.Request) {
		var req ImageRequest
		if err := decodeBody(r, &req); err != nil {
			writeError(w, err)
			return
		}
		if err := s.checkModel(req.Model, "image"); err != nil {
			writeError(w, err)
			return
		}
		if s.adapter.GenerateImage == nil {
			writeError(w, &ProtocolError{Category: "user_input", Message: "no image models", Status: 404})
			return
		}
		c := s.newCtx(r)
		res, err := s.adapter.GenerateImage(c, &req)
		status := http.StatusOK
		if err == nil && res != nil && res.TaskID != "" && len(res.Images) == 0 {
			status = http.StatusAccepted // 异步：Draw Studio 之后调 /images/query
		}
		respond(w, c, status, res, err)
	})
	mux.HandleFunc("POST /v1/images/query", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			TaskID string `json:"taskId"`
		}
		if err := decodeBody(r, &q); err != nil {
			writeError(w, err)
			return
		}
		if s.adapter.QueryImage == nil {
			writeError(w, TaskNotFound("no async image tasks"))
			return
		}
		c := s.newCtx(r)
		res, err := s.adapter.QueryImage(c, q.TaskID)
		respond(w, c, http.StatusOK, res, err)
	})
	mux.HandleFunc("POST /v1/videos/submit", func(w http.ResponseWriter, r *http.Request) {
		var req VideoRequest
		if err := decodeBody(r, &req); err != nil {
			writeError(w, err)
			return
		}
		if err := s.checkModel(req.Model, "video"); err != nil {
			writeError(w, err)
			return
		}
		if s.adapter.SubmitVideo == nil {
			writeError(w, &ProtocolError{Category: "user_input", Message: "no video models", Status: 404})
			return
		}
		c := s.newCtx(r)
		res, err := s.adapter.SubmitVideo(c, &req)
		respond(w, c, http.StatusOK, res, err)
	})
	mux.HandleFunc("POST /v1/videos/query", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			TaskID string `json:"taskId"`
		}
		if err := decodeBody(r, &q); err != nil {
			writeError(w, err)
			return
		}
		if s.adapter.QueryVideo == nil {
			writeError(w, TaskNotFound("no video tasks"))
			return
		}
		c := s.newCtx(r)
		res, err := s.adapter.QueryVideo(c, q.TaskID)
		respond(w, c, http.StatusOK, res, err)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, &ProtocolError{Category: "user_input", Message: "not found", Status: 404})
	})
	// 不带 Key 的请求一律 401。Key 对不对由上游判断——转换器不存 Key，也就不校验。
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		if bearer(r) == "" {
			writeError(w, &ProtocolError{Category: "user_input", Message: "missing key: fill the upstream API key into the Draw Studio provider", Status: 401})
		} else {
			mux.ServeHTTP(w, r)
		}
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// originOf 取 URL 的源（scheme://host[:port]），不是合法的绝对地址时回空串。
func originOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// newServer 建好服务；main 和测试共用。给能力声明里的每个模型补上 downloadWithKey：
// 结果链接落在这些源上时，Draw Studio 下载会带上 Key（protocol.md 1.4）。
// UPSTREAM_BASE_URL 的源和 Adapter.DownloadWithKey 总会并进去，模型自己写了的也保留。
func newServer(a Adapter) (*server, error) {
	upstream := originOf(os.Getenv("UPSTREAM_BASE_URL"))
	if upstream == "" {
		return nil, errors.New("UPSTREAM_BASE_URL 未设置或不是 http(s):// 开头的地址")
	}
	var caps map[string]json.RawMessage
	if err := json.Unmarshal([]byte(a.Capabilities), &caps); err != nil {
		return nil, fmt.Errorf("adapter.Capabilities 不是合法 JSON：%v", err)
	}
	var models []map[string]any
	if err := json.Unmarshal(caps["models"], &models); err != nil {
		return nil, fmt.Errorf("adapter.Capabilities 的 models 不对：%v", err)
	}
	s := &server{adapter: a, models: map[string]string{}}
	for _, m := range models {
		name, _ := m["model"].(string)
		kind, _ := m["kind"].(string)
		s.models[name] = kind
		var origins []string
		declared, _ := m["downloadWithKey"].([]any)
		for _, raw := range declared {
			if o, _ := raw.(string); o != "" {
				origins = append(origins, o)
			}
		}
		for _, raw := range append([]string{upstream}, a.DownloadWithKey...) {
			if o := originOf(raw); o != "" && !slices.Contains(origins, o) {
				origins = append(origins, o)
			}
		}
		m["downloadWithKey"] = origins
	}
	caps["models"], _ = json.Marshal(models)
	raw, err := json.MarshalIndent(caps, "", "  ")
	if err != nil {
		return nil, err
	}
	s.capabilities = raw
	return s, nil
}

// loadDotEnv 读取 .env（KEY=VALUE 一行一个，# 开头是注释，值可以用引号包起来）。
// 已经设置的环境变量优先，不会被 .env 覆盖——所以 docker run -e 照样生效。
// 先找当前目录，再找程序所在目录（Windows 上双击 exe 时当前目录不一定是它）。
func loadDotEnv() {
	paths := []string{".env"}
	if exe, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(exe), ".env"))
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k, v = strings.TrimSpace(strings.TrimPrefix(k, "export ")), strings.TrimSpace(v)
			if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
			if _, set := os.LookupEnv(k); !set {
				os.Setenv(k, v)
			}
		}
		log.Printf("loaded %s", p)
		return
	}
}

func main() {
	loadDotEnv()
	s, err := newServer(adapter)
	if err != nil {
		log.Fatal(err)
	}
	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8787"
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		log.Fatal(err)
	}
	log.Print("Draw Studio 后台供应商：Base URL 填转换器地址，API Key 填上游的 Key")
	fmt.Printf("protocol converter listening on http://127.0.0.1:%d\n", ln.Addr().(*net.TCPAddr).Port)
	log.Fatal(http.Serve(ln, s.routes()))
}
