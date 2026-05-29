package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/model"
)

type ModelReference struct {
	Name     string
	MimeType string
	Data     []byte
}

type Flow2APIVideoOptions struct {
	ReferenceMode    string
	Seconds          string
	Size             string
	Resolution       string
	GenerationConfig map[string]any
}

type Flow2APIImageOptions struct {
	Quality          string
	Size             string
	GenerationConfig map[string]any
}

func IsFlow2APIChannel(channel model.ModelChannel) bool {
	return strings.EqualFold(strings.TrimSpace(channel.Protocol), "flow2api")
}

func Flow2APIImageGeneration(channel model.ModelChannel, modelName string, prompt string, count int) ([][]byte, error) {
	return flow2APIImages(channel, modelName, prompt, nil, count, Flow2APIImageOptions{})
}

func Flow2APIImageGenerationWithOptions(channel model.ModelChannel, modelName string, prompt string, count int, options Flow2APIImageOptions) ([][]byte, error) {
	return flow2APIImages(channel, modelName, prompt, nil, count, options)
}

func Flow2APIImageEdit(channel model.ModelChannel, modelName string, prompt string, refs []ModelReference, count int) ([][]byte, error) {
	return flow2APIImages(channel, modelName, prompt, refs, count, Flow2APIImageOptions{})
}

func Flow2APIImageEditWithOptions(channel model.ModelChannel, modelName string, prompt string, refs []ModelReference, count int, options Flow2APIImageOptions) ([][]byte, error) {
	return flow2APIImages(channel, modelName, prompt, refs, count, options)
}

func Flow2APIVideoGeneration(channel model.ModelChannel, modelName string, prompt string, refs []ModelReference, options Flow2APIVideoOptions) ([]byte, error) {
	modelName, refs = resolveFlow2APIVideoRequest(channel, modelName, refs, options)
	content, err := flow2APIChatContentWithOptions(channel, modelName, prompt, refs, 20*time.Minute, flow2APIGenerationConfig(options))
	if err != nil {
		return nil, err
	}
	mediaURL := firstFlow2APIMediaURL(content, "video")
	if mediaURL == "" {
		return nil, errors.New("Flow2API 没有返回视频链接")
	}
	return downloadFlow2APIMedia(mediaURL, "video/mp4")
}

func Flow2APIModelSmokeTest(channel model.ModelChannel, modelName string) (string, error) {
	prompt := "Reply with the word ok."
	kind := inferModelModality(modelName)
	if kind == "image" {
		prompt = "A simple red circle icon centered on a plain white background."
	} else if kind == "video" {
		prompt = "A two second minimal animation of a red circle gently pulsing on a plain white background."
	}
	content, err := flow2APIChatContent(channel, modelName, prompt, nil, 20*time.Minute)
	if err != nil {
		return "", err
	}
	if kind == "image" {
		if firstFlow2APIMediaURL(content, "image") == "" {
			return "", errors.New("测试失败：没有返回图片链接")
		}
		return "图片测试成功", nil
	}
	if kind == "video" {
		if firstFlow2APIMediaURL(content, "video") == "" {
			return "", errors.New("测试失败：没有返回视频链接")
		}
		return "视频测试成功", nil
	}
	if strings.TrimSpace(content) == "" {
		return "", errors.New("测试失败：没有返回文本")
	}
	return content, nil
}

func resolveFlow2APIImageModel(channel model.ModelChannel, modelName string, options Flow2APIImageOptions) string {
	name := strings.TrimSpace(modelName)
	lower := strings.ToLower(name)
	prefix := ""
	switch {
	case strings.HasPrefix(lower, "gemini-3.0-pro-image"):
		prefix = "gemini-3.0-pro-image"
	case strings.HasPrefix(lower, "gemini-3.1-flash-image"):
		prefix = "gemini-3.1-flash-image"
	case strings.HasPrefix(lower, "imagen-4"):
		prefix = "imagen-4.0-generate-preview"
	default:
		return name
	}
	ratio := flow2APIImageRatioSuffix(firstString(options.Size, anyToString(options.GenerationConfig["size"])))
	if ratio == "" {
		ratio = flow2APIImageRatioSuffix(anyToString(options.GenerationConfig["aspectRatio"]))
	}
	if ratio == "" {
		ratio = "landscape"
	}
	quality := flow2APIImageQualitySuffix(options)
	candidates := []string{}
	if strings.HasPrefix(prefix, "imagen-4") {
		if ratio == "portrait" || ratio == "three-four" {
			candidates = append(candidates, prefix+"-portrait")
		} else {
			candidates = append(candidates, prefix+"-landscape")
		}
	} else {
		candidates = append(candidates, prefix+"-"+ratio+quality)
		if quality != "" {
			candidates = append(candidates, prefix+"-"+ratio)
		}
	}
	candidates = append(candidates, name)
	for _, candidate := range candidates {
		if flow2APIModelExists(channel.Models, candidate) {
			return candidate
		}
	}
	for _, candidate := range channel.Models {
		if strings.HasPrefix(strings.ToLower(candidate), prefix+"-"+ratio) {
			return candidate
		}
	}
	for _, candidate := range channel.Models {
		if strings.HasPrefix(strings.ToLower(candidate), prefix) {
			return candidate
		}
	}
	return name
}

func flow2APIImageRatioSuffix(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "1:1", "square":
		return "square"
	case "4:3", "four-three":
		return "four-three"
	case "3:4", "three-four":
		return "three-four"
	case "portrait":
		return "portrait"
	case "landscape":
		return "landscape"
	}
	if strings.Contains(value, "x") {
		parts := strings.SplitN(value, "x", 2)
		if len(parts) == 2 {
			var w, h int
			_, _ = fmt.Sscan(parts[0], &w)
			_, _ = fmt.Sscan(parts[1], &h)
			if w > 0 && h > 0 {
				if w == h {
					return "square"
				}
				if w*3 == h*4 {
					return "three-four"
				}
				if w*4 == h*3 {
					return "four-three"
				}
				if h > w {
					return "portrait"
				}
				return "landscape"
			}
		}
	}
	if strings.Contains(value, "9:16") || strings.Contains(value, "2:3") {
		return "portrait"
	}
	if strings.Contains(value, "16:9") {
		return "landscape"
	}
	return ""
}

func flow2APIImageQualitySuffix(options Flow2APIImageOptions) string {
	quality := strings.ToLower(strings.TrimSpace(firstString(options.Quality, anyToString(options.GenerationConfig["quality"]), anyToString(options.GenerationConfig["resolution"]))))
	switch quality {
	case "high", "4k", "uhd":
		return "-4k"
	case "medium", "2k", "hd":
		return "-2k"
	}
	return ""
}

func resolveFlow2APIVideoRequest(channel model.ModelChannel, modelName string, refs []ModelReference, options Flow2APIVideoOptions) (string, []ModelReference) {
	mode := strings.ToLower(strings.TrimSpace(options.ReferenceMode))
	if mode == "" {
		if len(refs) == 0 {
			mode = "text"
		} else {
			mode = "frame"
		}
	}
	switch mode {
	case "text":
		refs = nil
		modelName = replaceFlow2APIVideoFamily(modelName, "t2v")
	case "asset":
		if len(refs) > 3 {
			refs = refs[:3]
		}
		modelName = replaceFlow2APIVideoFamily(modelName, "r2v")
	case "extend":
		if len(refs) > 1 {
			refs = refs[:1]
		}
		modelName = replaceFlow2APIVideoFamily(modelName, "extend")
	default:
		if len(refs) > 2 {
			refs = refs[:2]
		}
		if len(refs) > 0 {
			modelName = replaceFlow2APIVideoFamily(modelName, "i2v")
		} else {
			modelName = replaceFlow2APIVideoFamily(modelName, "t2v")
		}
	}
	modelName = replaceFlow2APIVideoOrientation(modelName, options.Size)
	modelName = selectFlow2APIVideoModel(channel.Models, modelName, mode, options)
	return modelName, refs
}

func selectFlow2APIVideoModel(models []string, modelName string, mode string, options Flow2APIVideoOptions) string {
	name := strings.TrimSpace(modelName)
	if name == "" || !strings.Contains(strings.ToLower(name), "veo_3_1") || len(models) == 0 {
		return name
	}
	family := "t2v"
	switch mode {
	case "asset":
		family = "r2v"
	case "extend":
		family = "extend"
	default:
		if strings.Contains(strings.ToLower(name), "_i2v") {
			family = "i2v"
		}
	}
	tier := flow2APIVideoTier(name)
	orientation := flow2APIVideoOrientation(options.Size)
	seconds := strings.TrimSuffix(strings.TrimSpace(options.Seconds), "s")
	resolution := strings.ToLower(strings.TrimSpace(firstString(options.Resolution, anyToString(options.GenerationConfig["resolution"]))))
	best := name
	bestScore := -1 << 30
	for _, candidate := range models {
		score := scoreFlow2APIVideoModel(candidate, family, tier, orientation, seconds, resolution)
		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}
	if bestScore <= 0 {
		return name
	}
	return best
}

func scoreFlow2APIVideoModel(candidate string, family string, tier string, orientation string, seconds string, resolution string) int {
	name := strings.ToLower(candidate)
	if !strings.Contains(name, "veo_3_1") {
		return -10000
	}
	score := 0
	switch family {
	case "i2v":
		if strings.Contains(name, "_i2v") {
			score += 300
		} else {
			score -= 300
		}
	case "r2v":
		if strings.Contains(name, "_r2v") {
			score += 300
		} else {
			score -= 300
		}
	case "extend":
		if strings.Contains(name, "_extend") {
			score += 300
		} else {
			score -= 300
		}
	default:
		if strings.Contains(name, "_t2v") {
			score += 300
		} else {
			score -= 300
		}
	}
	switch tier {
	case "lite":
		if strings.Contains(name, "lite") {
			score += 160
		} else {
			score -= 120
		}
	case "fast":
		if strings.Contains(name, "fast") && !strings.Contains(name, "lite") {
			score += 160
		} else {
			score -= 120
		}
	default:
		if !strings.Contains(name, "fast") && !strings.Contains(name, "lite") {
			score += 160
		}
	}
	if orientation != "" {
		if strings.Contains(name, orientation) {
			score += 80
		} else {
			score -= 80
		}
	}
	if seconds != "" {
		if strings.Contains(name, "_"+seconds+"s") {
			score += 60
		} else {
			score -= 20
		}
	}
	switch {
	case strings.Contains(resolution, "4k"):
		if strings.Contains(name, "4k") {
			score += 50
		} else {
			score -= 15
		}
	case strings.Contains(resolution, "1080"):
		if strings.Contains(name, "1080p") {
			score += 50
		} else {
			score -= 15
		}
	default:
		if !strings.Contains(name, "4k") && !strings.Contains(name, "1080p") {
			score += 20
		}
	}
	return score
}

func flow2APIVideoTier(modelName string) string {
	name := strings.ToLower(modelName)
	if strings.Contains(name, "lite") {
		return "lite"
	}
	if strings.Contains(name, "fast") {
		return "fast"
	}
	return "quality"
}

func flow2APIVideoOrientation(value string) string {
	ratio := flow2APIImageRatioSuffix(value)
	if ratio == "portrait" || ratio == "three-four" {
		return "portrait"
	}
	if ratio != "" {
		return "landscape"
	}
	return ""
}

func flow2APIModelExists(models []string, modelName string) bool {
	for _, item := range models {
		if item == modelName {
			return true
		}
	}
	return false
}

func replaceFlow2APIVideoFamily(modelName string, family string) string {
	name := strings.TrimSpace(modelName)
	if name == "" || !strings.Contains(strings.ToLower(name), "veo_3_1") {
		return name
	}
	replacements := []string{"_t2v_", "_i2v_", "_r2v_", "_extend_", "_interpolation_"}
	for _, marker := range replacements {
		if strings.Contains(name, marker) {
			return strings.Replace(name, marker, "_"+family+"_", 1)
		}
	}
	return name
}

func replaceFlow2APIVideoOrientation(modelName string, size string) string {
	name := strings.TrimSpace(modelName)
	if name == "" || !strings.Contains(strings.ToLower(name), "veo_3_1") {
		return name
	}
	orientation := ""
	size = strings.ToLower(strings.TrimSpace(size))
	if strings.Contains(size, "x") {
		parts := strings.SplitN(size, "x", 2)
		if len(parts) == 2 {
			var w, h int
			_, _ = fmt.Sscan(parts[0], &w)
			_, _ = fmt.Sscan(parts[1], &h)
			if w > 0 && h > 0 {
				if h > w {
					orientation = "portrait"
				} else {
					orientation = "landscape"
				}
			}
		}
	} else if strings.Contains(size, "9:16") || strings.Contains(size, "2:3") || strings.Contains(size, "3:4") {
		orientation = "portrait"
	} else if strings.Contains(size, "16:9") || strings.Contains(size, "4:3") {
		orientation = "landscape"
	}
	if orientation == "" {
		return name
	}
	if strings.HasSuffix(name, "_portrait") {
		return strings.TrimSuffix(name, "_portrait") + "_" + orientation
	}
	if strings.HasSuffix(name, "_landscape") {
		return strings.TrimSuffix(name, "_landscape") + "_" + orientation
	}
	return name
}

func flow2APIGenerationConfig(options Flow2APIVideoOptions) map[string]any {
	config := map[string]any{}
	for key, value := range options.GenerationConfig {
		if strings.TrimSpace(key) != "" && value != nil {
			config[key] = value
		}
	}
	if options.Seconds != "" {
		config["duration"] = options.Seconds
	}
	if options.Resolution != "" {
		config["resolution"] = options.Resolution
	}
	if options.Size != "" {
		config["size"] = options.Size
	}
	if options.ReferenceMode != "" {
		config["referenceMode"] = options.ReferenceMode
	}
	if len(config) == 0 {
		return nil
	}
	return config
}

func inferModelModality(modelName string) string {
	name := strings.ToLower(modelName)
	isVideo := strings.Contains(name, "video") || strings.Contains(name, "veo") || strings.Contains(name, "sora") || strings.Contains(name, "_t2v") || strings.Contains(name, "_i2v") || strings.Contains(name, "_r2v") || strings.Contains(name, "grok-imagine")
	isImage := strings.Contains(name, "image") || strings.Contains(name, "imagen") || strings.Contains(name, "gpt-image") || strings.Contains(name, "nano-banana")
	if isVideo {
		return "video"
	}
	if isImage {
		return "image"
	}
	return "text"
}

func flow2APIImages(channel model.ModelChannel, modelName string, prompt string, refs []ModelReference, count int, options Flow2APIImageOptions) ([][]byte, error) {
	if count < 1 {
		count = 1
	}
	if count > workflowImageRequestMaxCount {
		count = workflowImageRequestMaxCount
	}
	modelName = resolveFlow2APIImageModel(channel, modelName, options)
	images := make([][]byte, 0, count)
	for index := 0; index < count; index++ {
		content, err := flow2APIChatContentWithOptions(channel, modelName, prompt, refs, 10*time.Minute, options.GenerationConfig)
		if err != nil {
			return nil, err
		}
		mediaURL := firstFlow2APIMediaURL(content, "image")
		if mediaURL == "" {
			return nil, errors.New("Flow2API 没有返回图片链接")
		}
		body, err := downloadFlow2APIMedia(mediaURL, "image/png")
		if err != nil {
			return nil, err
		}
		images = append(images, body)
	}
	return images, nil
}

func flow2APIChatContent(channel model.ModelChannel, modelName string, prompt string, refs []ModelReference, timeout time.Duration) (string, error) {
	return flow2APIChatContentWithOptions(channel, modelName, prompt, refs, timeout, nil)
}

func flow2APIChatContentWithOptions(channel model.ModelChannel, modelName string, prompt string, refs []ModelReference, timeout time.Duration, generationConfig map[string]any) (string, error) {
	content := any(prompt)
	if len(refs) > 0 {
		parts := []map[string]any{{"type": "text", "text": prompt}}
		for _, ref := range refs {
			mediaType := firstString(ref.MimeType, mime.TypeByExtension(filepath.Ext(ref.Name)), "image/png")
			parts = append(parts, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(ref.Data),
				},
			})
		}
		content = parts
	}
	payload := map[string]any{
		"model": modelName,
		"messages": []map[string]any{{
			"role":    "user",
			"content": content,
		}},
		"stream": false,
	}
	if len(generationConfig) > 0 {
		payload["generationConfig"] = generationConfig
	}
	body, _ := json.Marshal(payload)
	request, err := http.NewRequest(http.MethodPost, BuildModelChannelURL(channel, "/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: timeout}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(response.Body)
	if response.StatusCode >= http.StatusBadRequest {
		return "", readAdminChannelError(responseBody, response.StatusCode, "Flow2API 请求失败")
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return "", err
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", errors.New(parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", errors.New("Flow2API 没有返回内容")
	}
	return parsed.Choices[0].Message.Content, nil
}

func firstFlow2APIMediaURL(content string, kind string) string {
	patterns := []*regexp.Regexp{}
	if kind == "video" {
		patterns = append(patterns, regexp.MustCompile(`(?is)<video[^>]+src=['"]([^'"]+)['"]`))
	}
	if kind == "image" {
		patterns = append(patterns, regexp.MustCompile(`!\[[^\]]*]\(([^)]+)\)`))
	}
	patterns = append(patterns, regexp.MustCompile("https?://[^\\s\"'<>)]+"))
	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(content); len(match) > 1 {
			return strings.TrimSpace(match[1])
		}
		if match := pattern.FindString(content); match != "" {
			return strings.TrimSpace(match)
		}
	}
	return ""
}

func downloadFlow2APIMedia(value string, fallbackMime string) ([]byte, error) {
	if strings.HasPrefix(value, "data:") {
		comma := strings.IndexByte(value, ',')
		if comma < 0 {
			return nil, errors.New("invalid data url")
		}
		return base64.StdEncoding.DecodeString(value[comma+1:])
	}
	response, err := (&http.Client{Timeout: 10 * time.Minute}).Get(value)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("media download failed: status=%d", response.StatusCode)
	}
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		contentType = fallbackMime
	}
	if fallbackMime != "" && !strings.HasPrefix(contentType, strings.Split(fallbackMime, "/")[0]+"/") {
		return nil, fmt.Errorf("media download returned unexpected content type: %s", contentType)
	}
	return io.ReadAll(response.Body)
}
