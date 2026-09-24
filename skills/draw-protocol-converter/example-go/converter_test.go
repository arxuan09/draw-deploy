package main

// converter_test.go —— 代码层面的自查：起一个模拟上游，把 SKILL.md 第三节的清单逐条测一遍。
// 不需要 Draw Studio、不需要 Docker、不花上游额度：
//
//	go test ./...
//
// 换了上游以后，按新上游的请求 / 响应格式改 fakeUpstream 和各测试里的期望值。

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// testKey 是 Draw Studio 带来的 Key，也就是上游的 Key（转换器原样透传）。
const testKey = "up-key"

// fakeUpstream 模拟中转站：记录收到的请求，按路径回预设的响应。
type fakeUpstream struct {
	mu       sync.Mutex
	calls    int
	lastPath string
	lastAuth string
	lastBody map[string]any
	// reply 按「方法 路径」返回 (状态码, 响应体)；没配置的路径回 404。
	reply map[string]func() (int, string)
}

func (f *fakeUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastPath = r.URL.Path
	f.lastAuth = r.Header.Get("Authorization")
	raw, _ := io.ReadAll(r.Body)
	f.lastBody = nil
	_ = json.Unmarshal(raw, &f.lastBody)
	if fn, ok := f.reply[r.Method+" "+r.URL.Path]; ok {
		status, body := fn()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

// setup 起模拟上游，建好转换器的 HTTP handler。
func setup(t *testing.T) (http.Handler, *fakeUpstream, *httptest.Server) {
	t.Helper()
	up := &fakeUpstream{reply: map[string]func() (int, string){}}
	srv := httptest.NewServer(up)
	t.Cleanup(srv.Close)
	t.Setenv("UPSTREAM_BASE_URL", srv.URL)
	s, err := newServer(adapter)
	if err != nil {
		t.Fatal(err)
	}
	return s.routes(), up, srv
}

// call 以 Draw Studio 的身份请求转换器；headers 可覆盖默认的 Authorization。
func call(h http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	var r io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		r = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Authorization", "Bearer "+testKey)
	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应不是 JSON（%d）：%s", rec.Code, rec.Body.String())
	}
	return out
}

// errorCategory 取协议错误体里的 category。
func errorCategory(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	e, _ := decode(t, rec)["error"].(map[string]any)
	c, _ := e["category"].(string)
	return c
}

func imageReq(ratio, quality string, refs ...string) map[string]any {
	references := []map[string]any{}
	for _, u := range refs {
		references = append(references, map[string]any{"url": u})
	}
	return map[string]any{
		"requestId": "req-1", "model": "seedream-5.0", "intent": "generate", "prompt": "一只杯子",
		"size": map[string]any{"raw": ratio, "ratio": ratio}, "quality": quality,
		"references": references, "params": map[string]any{},
	}
}

// ---------------------------------------------------------------------------
// 清单第 2、4 条：Key 透传、能力声明
// ---------------------------------------------------------------------------

func TestKeyIsPassedThrough(t *testing.T) {
	h, up, _ := setup(t)
	up.reply["POST /v1/images/generations"] = func() (int, string) { return 200, `{"data":[{"url":"https://cdn.example.com/x.png"}]}` }
	for _, path := range []string{"/v1/capabilities", "/v1/images/generate"} {
		if rec := call(h, "POST", path, imageReq("16:9", "2K"), map[string]string{"Authorization": ""}); rec.Code != 401 {
			t.Errorf("不带 Key 访问 %s 应回 401，实际 %d", path, rec.Code)
		}
	}
	if up.calls != 0 {
		t.Error("不带 Key 不该打到上游")
	}
	// 不是 Bearer 形式的也当没带。
	if rec := call(h, "POST", "/v1/images/generate", imageReq("16:9", "2K"), map[string]string{"Authorization": "Basic dXNlcjpw"}); rec.Code != 401 {
		t.Errorf("Basic 认证应回 401，实际 %d", rec.Code)
	}
	// 带什么 Key 来，上游就收到什么 Key——转换器不存、也不换 Key。
	call(h, "POST", "/v1/images/generate", imageReq("16:9", "2K"), map[string]string{"Authorization": "bearer another-account"})
	if up.lastAuth != "Bearer another-account" {
		t.Errorf("上游应收到请求带来的 Key，实际 %q", up.lastAuth)
	}
	// 没配上游地址不能启动。
	t.Setenv("UPSTREAM_BASE_URL", "")
	if _, err := newServer(adapter); err == nil {
		t.Error("没配 UPSTREAM_BASE_URL 应拒绝启动")
	}
}

// 清单第 11 条：转换器不校验 Key，谁都能调，所以素材只下载公网地址。
func TestMaterialsMustBePublic(t *testing.T) {
	c := &Ctx{ctx: context.Background()}
	for _, u := range []string{"http://127.0.0.1:1/x.png", "http://169.254.169.254/latest/meta-data/", "http://10.0.0.8/a.png", "file:///etc/passwd"} {
		if _, _, err := c.FetchMaterial(u); err == nil {
			t.Errorf("%s 不是公网地址，应拒绝", u)
		} else if pe, ok := err.(*ProtocolError); !ok || pe.Category != "user_input" {
			t.Errorf("%s：应回 user_input，实际 %v", u, err)
		}
	}
}

func TestCapabilitiesMatchTheCode(t *testing.T) {
	h, _, srv := setup(t)
	rec := call(h, "GET", "/v1/capabilities", nil, nil)
	caps := decode(t, rec)
	if rec.Code != 200 || caps["protocol"] != float64(1) || caps["dryRun"] != true {
		t.Fatalf("能力声明 = %d %v", rec.Code, caps)
	}
	for _, m := range caps["models"].([]any) {
		model := m.(map[string]any)
		// 结果链接在上游自己的域名上时要带 Key 下载：每个模型都要声明这个源。
		if origins, _ := model["downloadWithKey"].([]any); len(origins) != 1 || origins[0] != srv.URL {
			t.Errorf("%v 的 downloadWithKey = %v，应为 [%s]", model["model"], model["downloadWithKey"], srv.URL)
		}
		// 声明的比例必须和代码里接受的一致，否则用户能选、转换器却拒绝。
		if model["kind"] == "image" {
			declared := model["image"].(map[string]any)["ratios"].([]any)
			if len(declared) != len(imageRatios) {
				t.Errorf("声明的比例 %v 和代码里的 %v 不一致", declared, imageRatios)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 清单第 5、12 条：模型校验、不支持的值
// ---------------------------------------------------------------------------

func TestBadRequestsNeverReachTheUpstream(t *testing.T) {
	h, up, _ := setup(t)
	unknown := imageReq("16:9", "2K")
	unknown["model"] = "no-such-model"
	video := imageReq("16:9", "2K")
	video["model"] = "doubao-seedance-2-0-260128"
	inpaint := imageReq("16:9", "2K", "https://oss.example.com/a.png")
	inpaint["intent"] = "inpaint"
	for name, body := range map[string]map[string]any{
		"不认识的模型":   unknown,
		"拿视频模型出图":  video,
		"不支持的比例":   imageReq("7:3", "2K"),
		"不支持的档位":   imageReq("16:9", "4K"),
		"没声明的局部重绘": inpaint,
	} {
		rec := call(h, "POST", "/v1/images/generate", body, nil)
		if rec.Code != 400 || errorCategory(t, rec) != "user_input" {
			t.Errorf("%s：应回 400 + user_input，实际 %d %s", name, rec.Code, rec.Body.String())
		}
	}
	if up.calls != 0 {
		t.Errorf("不合法的请求不应调上游，实际调了 %d 次", up.calls)
	}
}

// ---------------------------------------------------------------------------
// 翻译：协议请求 → 上游请求，上游响应 → 协议响应
// ---------------------------------------------------------------------------

func TestImageRequestIsTranslated(t *testing.T) {
	h, up, srv := setup(t)
	up.reply["POST /v1/images/generations"] = func() (int, string) {
		return 200, `{"data":[{"url":"` + srv.URL + `/out.png"}]}`
	}
	cases := []struct {
		name      string
		body      map[string]any
		wantSize  string
		wantRes   string
		wantImage any
	}{
		{"文生图 16:9 2K", imageReq("16:9", "2K"), "16:9", "2k", nil},
		{"自动比例 1K", imageReq("auto", "1K"), "auto", "1k", nil},
		{"单张参考图", imageReq("1:1", "1K", "https://oss.example.com/a.png"), "1:1", "1k", "https://oss.example.com/a.png"},
	}
	for _, tc := range cases {
		rec := call(h, "POST", "/v1/images/generate", tc.body, nil)
		if rec.Code != 200 {
			t.Fatalf("%s：%d %s", tc.name, rec.Code, rec.Body.String())
		}
		b := up.lastBody
		if up.lastAuth != "Bearer up-key" || b["size"] != tc.wantSize || b["resolution"] != tc.wantRes || b["image"] != tc.wantImage {
			t.Errorf("%s：上游收到 auth=%q body=%v", tc.name, up.lastAuth, b)
		}
		images, _ := decode(t, rec)["images"].([]any)
		if len(images) != 1 || images[0].(map[string]any)["url"] != srv.URL+"/out.png" {
			t.Errorf("%s：响应 = %s", tc.name, rec.Body.String())
		}
	}
	// 多张参考图走 images 数组。
	call(h, "POST", "/v1/images/generate", imageReq("1:1", "1K", "https://oss.example.com/a.png", "https://oss.example.com/b.png"), nil)
	if refs, _ := up.lastBody["images"].([]any); len(refs) != 2 || up.lastBody["image"] != nil {
		t.Errorf("多张参考图：上游收到 %v", up.lastBody)
	}
}

// ---------------------------------------------------------------------------
// 清单第 6 条：错误分类
// ---------------------------------------------------------------------------

func TestUpstreamErrorsAreClassified(t *testing.T) {
	h, up, _ := setup(t)
	cases := []struct {
		status       int
		body         string
		wantStatus   int
		wantCategory string
	}{
		{400, `{"error":{"message":"invalid size"}}`, 400, "user_input"},
		{400, `{"error":{"message":"The request was rejected by content moderation"}}`, 400, "moderation"},
		{429, `{"error":{"message":"rate limited"}}`, 429, "busy"},
		{401, `{"error":{"message":"invalid api key"}}`, 502, "upstream"},
		{503, `service unavailable`, 503, "upstream"},
	}
	for _, tc := range cases {
		up.reply["POST /v1/images/generations"] = func() (int, string) { return tc.status, tc.body }
		rec := call(h, "POST", "/v1/images/generate", imageReq("16:9", "2K"), nil)
		if rec.Code != tc.wantStatus || errorCategory(t, rec) != tc.wantCategory {
			t.Errorf("上游 %d %s：应回 %d + %s，实际 %d %s", tc.status, tc.body, tc.wantStatus, tc.wantCategory, rec.Code, rec.Body.String())
		}
	}
}

// ---------------------------------------------------------------------------
// 清单第 7、13 条：预演不调上游、Key 打码
// ---------------------------------------------------------------------------

func TestDryRunReportsWithoutCallingTheUpstream(t *testing.T) {
	h, up, _ := setup(t)
	rec := call(h, "POST", "/v1/images/generate", imageReq("16:9", "2K", "https://oss.example.com/a.png"), map[string]string{"X-Draw-Dry-Run": "1"})
	out := decode(t, rec)
	reqs, _ := out["upstreamRequests"].([]any)
	if rec.Code != 200 || out["dryRun"] != true || len(reqs) == 0 {
		t.Fatalf("预演应回 dryRun + upstreamRequests，实际 %d %s", rec.Code, rec.Body.String())
	}
	if up.calls != 0 {
		t.Error("预演不能真的调上游")
	}
	first := reqs[0].(map[string]any)
	auth := first["headers"].(map[string]any)["Authorization"].(string)
	if !strings.Contains(auth, "*") || strings.Contains(auth, "up-key") {
		t.Errorf("预演回报里的密钥没打码：%q", auth)
	}
	if !strings.HasSuffix(first["url"].(string), "/v1/images/generations") {
		t.Errorf("预演回报的地址 = %v", first["url"])
	}
}

// ---------------------------------------------------------------------------
// 清单第 8、9、10 条：视频任务、查询状态码、结果链接
// ---------------------------------------------------------------------------

func TestVideoTaskLifecycle(t *testing.T) {
	h, up, srv := setup(t)
	state := "running"
	up.reply["POST /api/v3/contents/generations/tasks"] = func() (int, string) { return 200, `{"id":"cgt-1"}` }
	up.reply["GET /api/v3/contents/generations/tasks/cgt-1"] = func() (int, string) {
		switch state {
		case "running":
			return 200, `{"status":"running"}`
		case "blip":
			return 502, `bad gateway`
		case "blocked":
			return 200, `{"status":"failed","error":{"code":"OutputVideoSensitiveContentDetected","message":"blocked"}}`
		}
		return 200, `{"status":"succeeded","content":{"video_url":"` + srv.URL + `/v1/videos/cgt-1/content"}}`
	}
	sub := call(h, "POST", "/v1/videos/submit", map[string]any{
		"requestId": "req-v", "model": "doubao-seedance-2-0-260128", "prompt": "图片1 里的杯子旋转",
		"duration": 6, "ratio": "16:9", "resolution": "720p", "generateAudio": false,
		"firstFrame": map[string]any{"url": "https://oss.example.com/f.png"},
		"references": map[string]any{"images": []string{}, "videos": []string{}, "audios": []string{}},
		"params":     map[string]any{},
	}, nil)
	taskID, _ := decode(t, sub)["taskId"].(string)
	if sub.Code != 200 || taskID == "" {
		t.Fatalf("提交：%d %s", sub.Code, sub.Body.String())
	}
	content, _ := up.lastBody["content"].([]any)
	if len(content) != 2 || content[1].(map[string]any)["role"] != "first_frame" || up.lastBody["generate_audio"] != false {
		t.Errorf("上游收到的视频请求 = %v", up.lastBody)
	}

	query := func() *httptest.ResponseRecorder {
		return call(h, "POST", "/v1/videos/query", map[string]any{"requestId": "req-v", "taskId": taskID}, nil)
	}
	if rec := query(); rec.Code != 200 || decode(t, rec)["status"] != "running" {
		t.Errorf("进行中：%d %s", rec.Code, rec.Body.String())
	}
	state = "blip"
	if rec := query(); rec.Code != 502 {
		t.Errorf("上游查询接口临时 502 要原样回 5xx（Draw Studio 会重试），实际 %d", rec.Code)
	}
	state = "blocked"
	if rec := query(); rec.Code != 200 || decode(t, rec)["status"] != "failed" || errorCategory(t, rec) != "moderation" {
		t.Errorf("审核失败：%d %s", rec.Code, rec.Body.String())
	}
	state = "done"
	done := query()
	// 结果链接原样给出：它在上游域名上、要带 Key，而 downloadWithKey 已声明这个源，
	// Draw Studio 下载时会带上 Key（TestCapabilitiesMatchTheCode 查过声明）。
	if videoURL, _ := decode(t, done)["videoUrl"].(string); videoURL != srv.URL+"/v1/videos/cgt-1/content" {
		t.Fatalf("videoUrl = %q", videoURL)
	}

	// 解不开的 taskId → 404，Draw Studio 据此立即判失败。
	if rec := call(h, "POST", "/v1/videos/query", map[string]any{"taskId": "garbage!"}, nil); rec.Code != 404 {
		t.Errorf("不认识的 taskId 应回 404，实际 %d", rec.Code)
	}
}
