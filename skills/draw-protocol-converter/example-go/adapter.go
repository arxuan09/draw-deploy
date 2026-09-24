package main

// adapter.go ——「协议 ↔ 上游」的翻译。这个示例对接一家中转站，同时提供图片和视频：
//
//	图片：POST {UPSTREAM_BASE_URL}/v1/images/generations
//	      {model, prompt, size: "16:9"|"auto"（比例）, resolution: "1k"|"2k"（小写 k）,
//	       image: "url"（单张参考图）| images: ["url", …]（多张）, n}
//	      → {data: [{url} | {b64_json}]}
//	视频：方舟（火山）格式
//	      POST {UPSTREAM_BASE_URL}/api/v3/contents/generations/tasks
//	      {model, content: [{type:"text"}, {type:"image_url", image_url:{url}, role}…],
//	       duration, ratio, resolution, generate_audio, watermark} → {id}
//	      GET  {UPSTREAM_BASE_URL}/api/v3/contents/generations/tasks/{id}
//	      → {status: queued|running|succeeded|failed|expired|cancelled, content:{video_url}, error}
//	      结果链接在中转站自己的域名上，下载要带上游 Key → downloadWithKey 声明（server.go 自动补）。
//
// 上游的 Key 一律取 c.APIKey：它就是 Draw Studio 后台供应商里填的 Key，转换器不存密钥。
//
// 接别的上游时重写这个文件：能力声明照上游文档写，翻译函数照上游的请求 / 响应格式写。

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
)

var adapter = Adapter{
	Capabilities:  capabilities,
	GenerateImage: generateImage,
	SubmitVideo:   submitVideo,
	QueryVideo:    queryVideo,
	// 图片同步返回，不需要 QueryImage。
}

// capabilities 是 GET /capabilities 的原文（protocol.md 第 2 节），照上游文档如实写。
// pricingHint 是站内积分，按运营方给的上游单价 × 换算 × 毛利算出（这里是示意）。
const capabilities = `{
  "protocol": 1,
  "name": "中转站（图片 + 视频）",
  "dryRun": true,
  "models": [
    {
      "model": "seedream-5.0",
      "kind": "image",
      "image": {
        "formTemplate": "gemini",
        "ratios": ["auto", "1:1", "3:2", "2:3", "4:3", "3:4", "5:4", "4:5", "16:9", "9:16", "21:9"],
        "tiers": ["1K", "2K"],
        "referenceImages": { "max": 10 }
      },
      "pricingHint": { "mode": "per_image", "base": 20 }
    },
    {
      "model": "doubao-seedance-2-0-260128",
      "kind": "video",
      "video": {
        "durationRange": { "min": 4, "max": 15 },
        "ratios": ["16:9", "9:16", "1:1", "21:9", "4:3", "3:4"],
        "resolutions": ["720p", "1080p"],
        "audioToggle": true,
        "materialRefSyntax": "plain",
        "firstFrame": "optional",
        "lastFrame": "optional",
        "references": { "images": { "max": 9 }, "videos": { "max": 3 }, "audios": { "max": 3 }, "total": 12 },
        "framesCountAsImages": true,
        "framesExclusiveWithRefs": true,
        "pollIntervalMs": 15000
      },
      "pricingHint": { "mode": "per_second", "resolutions": { "720p": 10, "1080p": 20 } }
    }
  ]
}`

var imageRatios = []string{"auto", "1:1", "3:2", "2:3", "4:3", "3:4", "5:4", "4:5", "16:9", "9:16", "21:9"}

func upstreamBase() (string, error) {
	base := strings.TrimRight(os.Getenv("UPSTREAM_BASE_URL"), "/")
	if base == "" {
		return "", Fail("converter", "UPSTREAM_BASE_URL is not set")
	}
	return base, nil
}

// upstreamAuth：上游的 Key 一律取 c.APIKey，即 Draw Studio 这次请求带来的 Key。
func upstreamAuth(c *Ctx) map[string]string {
	return map[string]string{"Authorization": "Bearer " + c.APIKey}
}

func userInput(code, message string) *ProtocolError {
	e := Fail("user_input", message)
	e.Code = code
	return e
}

// ---------------------------------------------------------------------------
// 图片
// ---------------------------------------------------------------------------

func generateImage(c *Ctx, req *ImageRequest) (*ImageResponse, error) {
	// 声明了参考图，工作台就会提供改图和放大；这个上游不区分，都按「参考图 + 提示词」出图。
	if !slices.Contains([]string{"generate", "edit", "upscale"}, req.Intent) {
		return nil, userInput("intent", "intent "+req.Intent+" is not supported by this model")
	}
	ratio := "auto"
	if req.Size.Ratio != nil {
		ratio = *req.Size.Ratio
	}
	if !slices.Contains(imageRatios, ratio) {
		return nil, userInput("ratio", "ratio "+ratio+" is not supported")
	}
	// gemini 模板：quality 就是档位。上游只认小写的 1k / 2k。
	tier := req.Quality
	if tier == "" && req.Size.Tier != nil {
		tier = *req.Size.Tier
	}
	if tier == "" {
		tier = "1K"
	}
	resolution, ok := map[string]string{"1K": "1k", "2K": "2k"}[tier]
	if !ok {
		return nil, userInput("tier", "resolution "+tier+" is not supported")
	}

	body := map[string]any{}
	for k, v := range req.Params { // 后台「默认参数」合并进上游请求
		body[k] = v
	}
	body["model"] = req.Model
	body["prompt"] = req.Prompt
	body["size"] = ratio
	body["resolution"] = resolution
	body["n"] = 1
	refs := make([]string, 0, len(req.References))
	for _, r := range req.References {
		refs = append(refs, r.URL)
	}
	if len(refs) == 1 {
		body["image"] = refs[0]
	} else if len(refs) > 1 {
		body["images"] = refs
	}

	base, err := upstreamBase()
	if err != nil {
		return nil, err
	}
	res, err := c.Upstream("POST", base+"/v1/images/generations", upstreamAuth(c), body)
	if err != nil {
		return nil, err // 包括预演的 errDryRun，原样返回
	}
	if !res.OK() {
		return nil, UpstreamError(res.Status, res.Body)
	}
	var data struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(res.Body, &data); err != nil {
		return nil, Fail("converter", "upstream answered non-JSON: "+truncate(string(res.Body), 200))
	}
	if len(data.Data) == 0 {
		return nil, Fail("upstream", "upstream returned no image")
	}
	first := data.Data[0]
	img := ImageResult{URL: first.URL}
	if first.B64JSON != "" {
		img = ImageResult{B64: first.B64JSON}
	}
	return &ImageResponse{Images: []ImageResult{img}, Usage: data.Usage}, nil
}

// ---------------------------------------------------------------------------
// 视频
// ---------------------------------------------------------------------------

func submitVideo(c *Ctx, req *VideoRequest) (*VideoResponse, error) {
	content := []map[string]any{{"type": "text", "text": req.Prompt}}
	add := func(kind, u, role string) {
		content = append(content, map[string]any{"type": kind, kind: map[string]string{"url": u}, "role": role})
	}
	if req.FirstFrame != nil {
		add("image_url", req.FirstFrame.URL, "first_frame")
	}
	if req.LastFrame != nil {
		add("image_url", req.LastFrame.URL, "last_frame")
	}
	for _, u := range req.References.Images {
		add("image_url", u, "reference_image")
	}
	for _, u := range req.References.Videos {
		add("video_url", u, "reference_video")
	}
	for _, u := range req.References.Audios {
		add("audio_url", u, "reference_audio")
	}

	// 上游默认会加水印；后台「默认参数」里写 {"watermark": true} 可以打开。
	watermark, _ := req.Params["watermark"].(bool)
	body := map[string]any{
		"model":      req.Model,
		"content":    content,
		"duration":   req.Duration,
		"ratio":      req.Ratio,
		"resolution": req.Resolution,
		"watermark":  watermark,
	}
	if req.GenerateAudio != nil {
		body["generate_audio"] = *req.GenerateAudio
	}

	base, err := upstreamBase()
	if err != nil {
		return nil, err
	}
	res, err := c.Upstream("POST", base+"/api/v3/contents/generations/tasks", upstreamAuth(c), body)
	if err != nil {
		return nil, err
	}
	if !res.OK() {
		return nil, UpstreamError(res.Status, res.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(res.Body, &created) != nil || created.ID == "" {
		return nil, Fail("converter", "upstream answered without an id: "+truncate(string(res.Body), 200))
	}
	// 上游任务 ID 编进协议 taskId：转换器不存状态。
	return &VideoResponse{TaskID: EncodeTaskID(map[string]string{"u": created.ID})}, nil
}

var sensitiveRe = regexp.MustCompile(`(?i)sensitive|moderation|risk|违规|审核`)

func queryVideo(c *Ctx, taskID string) (*VideoResponse, error) {
	ids, err := DecodeTaskID(taskID)
	if err != nil {
		return nil, err
	}
	upstreamID := ids["u"]
	if upstreamID == "" {
		return nil, TaskNotFound("unknown taskId")
	}
	base, err := upstreamBase()
	if err != nil {
		return nil, err
	}
	res, err := c.Upstream("GET", base+"/api/v3/contents/generations/tasks/"+url.PathEscape(upstreamID), upstreamAuth(c), nil)
	if err != nil {
		return nil, err
	}
	if res.Status == 404 {
		return nil, TaskNotFound("upstream does not know task " + upstreamID)
	}
	if !res.OK() {
		return nil, UpstreamError(res.Status, res.Body) // 上游 5xx 保留，Draw Studio 会重试
	}
	var task struct {
		Status  string `json:"status"`
		Content struct {
			VideoURL string `json:"video_url"`
		} `json:"content"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Usage map[string]any `json:"usage"`
	}
	if err := json.Unmarshal(res.Body, &task); err != nil {
		return nil, Fail("converter", "upstream answered non-JSON: "+truncate(string(res.Body), 200))
	}
	switch task.Status {
	case "queued", "running":
		return &VideoResponse{Status: task.Status}, nil
	case "succeeded":
		videoURL := task.Content.VideoURL
		if videoURL == "" {
			return &VideoResponse{Status: "failed", Error: &ErrorBody{Category: "upstream", Message: "succeeded without video_url"}}, nil
		}
		// 链接在中转站自己的域名上、要带 Key 才能下也没关系：能力声明里的 downloadWithKey
		// （server.go 自动补上 UPSTREAM_BASE_URL 的源）告诉 Draw Studio 下载时带上 Key。
		return &VideoResponse{Status: "succeeded", VideoURL: videoURL, Usage: task.Usage}, nil
	default: // failed / expired / cancelled：任务确定失败，回 200 + failed
		code, message := task.Status, "task "+task.Status
		if task.Error != nil {
			code, message = task.Error.Code, task.Error.Message
		}
		category := "upstream"
		if sensitiveRe.MatchString(code + " " + message) {
			category = "moderation"
		}
		return &VideoResponse{Status: "failed", Error: &ErrorBody{Category: category, Code: code, Message: fmt.Sprintf("%s: %s", code, message)}}, nil
	}
}
