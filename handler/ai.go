package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/service"
)

type cachedAIVideo struct {
	Body      []byte
	MimeType  string
	ExpiresAt time.Time
}

type storedAIVideo struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	MimeType  string `json:"mimeType"`
	Filename  string `json:"filename"`
	Bytes     int    `json:"bytes"`
	CreatedAt string `json:"createdAt"`
}

var aiVideoCache sync.Map

func AIImagesGenerations(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/images/generations")
}

func AIImagesEdits(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/images/edits")
}

func AIChatCompletions(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/chat/completions")
}

func AIVideos(w http.ResponseWriter, r *http.Request) {
	proxyAIRequest(w, r, "/videos")
}

func AIVideo(w http.ResponseWriter, r *http.Request, id string) {
	proxyAIGetRequest(w, r, "/videos/"+id)
}

func AIVideoContent(w http.ResponseWriter, r *http.Request, id string) {
	proxyAIGetRequest(w, r, "/videos/"+id+"/content")
}

func proxyAIGetRequest(w http.ResponseWriter, r *http.Request, path string) {
	if tryWriteCachedAIVideo(w, path) {
		return
	}
	modelName := r.URL.Query().Get("model")
	if strings.TrimSpace(modelName) == "" {
		modelName = "grok-imagine-video"
	}
	channel, err := service.SelectModelChannel(modelName)
	if err != nil {
		log.Printf("AI proxy select channel failed: model=%s err=%v", modelName, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	request, err := http.NewRequest(http.MethodGet, service.BuildModelChannelURL(channel, path), nil)
	if err != nil {
		Fail(w, "AI 接口请求失败")
		return
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	copyAIResponse(w, request, nil)
}

func proxyAIRequest(w http.ResponseWriter, r *http.Request, path string) {
	body, contentType, modelName, err := readAIRequest(r)
	if err != nil {
		log.Printf("AI proxy request read failed: %v", err)
		Fail(w, "AI 接口请求失败")
		return
	}
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	credits, err := service.ModelCost(modelName)
	if err != nil {
		log.Printf("AI proxy read model cost failed: model=%s err=%v", modelName, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	credits *= readAIRequestCount(body, contentType)
	channel, err := service.SelectModelChannel(modelName)
	if err != nil {
		log.Printf("AI proxy select channel failed: model=%s err=%v", modelName, err)
		Fail(w, "AI 接口请求失败")
		return
	}
	if service.IsFlow2APIChannel(channel) && flow2APIProxyPath(path) {
		if err := service.ConsumeUserCredits(user.ID, modelName, credits, path); err != nil {
			FailError(w, err)
			return
		}
		if err := handleFlow2APIProxy(w, path, body, contentType, modelName, channel); err != nil {
			log.Printf("Flow2API proxy failed: path=%s model=%s err=%v", path, modelName, err)
			if refundErr := service.RefundUserCredits(user.ID, modelName, credits, path); refundErr != nil {
				log.Printf("AI proxy refund credits failed: user=%s model=%s credits=%d err=%v", user.ID, modelName, credits, refundErr)
			}
			Fail(w, "AI 接口请求失败")
		}
		return
	}
	request, err := http.NewRequest(http.MethodPost, service.BuildModelChannelURL(channel, path), bytes.NewReader(body))
	if err != nil {
		log.Printf("AI proxy build request failed: url=%s err=%v", service.BuildModelChannelURL(channel, path), err)
		Fail(w, "AI 接口请求失败")
		return
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if err := service.ConsumeUserCredits(user.ID, modelName, credits, path); err != nil {
		FailError(w, err)
		return
	}
	copyAIResponse(w, request, func() {
		if err := service.RefundUserCredits(user.ID, modelName, credits, path); err != nil {
			log.Printf("AI proxy refund credits failed: user=%s model=%s credits=%d err=%v", user.ID, modelName, credits, err)
		}
	})
}

func flow2APIProxyPath(path string) bool {
	return path == "/images/generations" || path == "/images/edits" || path == "/videos"
}

func handleFlow2APIProxy(w http.ResponseWriter, path string, body []byte, contentType string, modelName string, channel model.ModelChannel) error {
	switch path {
	case "/images/generations":
		var payload struct {
			Prompt           string         `json:"prompt"`
			N                int            `json:"n"`
			Quality          string         `json:"quality"`
			Size             string         `json:"size"`
			GenerationConfig map[string]any `json:"generation_config"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return err
		}
		images, err := service.Flow2APIImageGenerationWithOptions(channel, modelName, payload.Prompt, payload.N, service.Flow2APIImageOptions{
			Quality:          payload.Quality,
			Size:             payload.Size,
			GenerationConfig: payload.GenerationConfig,
		})
		if err != nil {
			return err
		}
		writeAIImageResponse(w, images)
		return nil
	case "/images/edits":
		prompt, count, refs, formValues, err := parseFlow2APIMultipartWithValues(body, contentType, "image")
		if err != nil {
			return err
		}
		images, err := service.Flow2APIImageEditWithOptions(channel, modelName, prompt, refs, count, flow2APIImageOptionsFromForm(formValues))
		if err != nil {
			return err
		}
		writeAIImageResponse(w, images)
		return nil
	case "/videos":
		prompt, _, refs, formValues, err := parseFlow2APIMultipartWithValues(body, contentType, "input_reference")
		if err != nil {
			return err
		}
		video, err := service.Flow2APIVideoGeneration(channel, modelName, prompt, refs, flow2APIVideoOptionsFromForm(formValues))
		if err != nil {
			return err
		}
		id := fmt.Sprintf("flow2api-%d", time.Now().UnixNano())
		if err := storeAIVideo(id, video, "video/mp4"); err != nil {
			return err
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "status": "completed"})
		return nil
	default:
		return fmt.Errorf("unsupported flow2api path: %s", path)
	}
}

func flow2APIImageOptionsFromForm(values map[string]string) service.Flow2APIImageOptions {
	options := service.Flow2APIImageOptions{
		Quality: values["quality"],
		Size:    values["size"],
	}
	if raw := strings.TrimSpace(values["generation_config"]); raw != "" {
		parsed := map[string]any{}
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			options.GenerationConfig = parsed
		}
	}
	return options
}

func parseFlow2APIMultipart(body []byte, contentType string, fileField string) (string, int, []service.ModelReference, error) {
	prompt, count, refs, _, err := parseFlow2APIMultipartWithValues(body, contentType, fileField)
	return prompt, count, refs, err
}

func parseFlow2APIMultipartWithValues(body []byte, contentType string, fileField string) (string, int, []service.ModelReference, map[string]string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", 0, nil, nil, err
	}
	form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(512 << 20)
	if err != nil {
		return "", 0, nil, nil, err
	}
	defer form.RemoveAll()
	prompt := firstFormValue(form, "prompt")
	count := 1
	if raw := firstFormValue(form, "n"); raw != "" {
		_, _ = fmt.Sscan(raw, &count)
	}
	refs := []service.ModelReference{}
	seenFields := map[string]bool{}
	for _, fieldName := range []string{fileField, fileField + "[]", "image", "input_reference", "input_reference[]"} {
		if seenFields[fieldName] {
			continue
		}
		seenFields[fieldName] = true
		for _, header := range form.File[fieldName] {
			file, err := header.Open()
			if err != nil {
				return "", 0, nil, nil, err
			}
			data, readErr := io.ReadAll(file)
			_ = file.Close()
			if readErr != nil {
				return "", 0, nil, nil, readErr
			}
			refs = append(refs, service.ModelReference{Name: header.Filename, MimeType: header.Header.Get("Content-Type"), Data: data})
		}
	}
	values := map[string]string{}
	for key, value := range form.Value {
		if len(value) > 0 {
			values[key] = value[0]
		}
	}
	return prompt, count, refs, values, nil
}

func flow2APIVideoOptionsFromForm(values map[string]string) service.Flow2APIVideoOptions {
	options := service.Flow2APIVideoOptions{
		ReferenceMode: firstNonEmpty(values["reference_mode"], values["video_reference_mode"]),
		Seconds:       values["seconds"],
		Size:          values["size"],
		Resolution:    firstNonEmpty(values["resolution_name"], values["vquality"]),
	}
	if raw := strings.TrimSpace(values["generation_config"]); raw != "" {
		parsed := map[string]any{}
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			options.GenerationConfig = parsed
		}
	}
	return options
}

func firstFormValue(form *multipart.Form, key string) string {
	if values := form.Value[key]; len(values) > 0 {
		return values[0]
	}
	return ""
}

func writeAIImageResponse(w http.ResponseWriter, images [][]byte) {
	data := make([]map[string]string, 0, len(images))
	for _, image := range images {
		data = append(data, map[string]string{"b64_json": base64.StdEncoding.EncodeToString(image)})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func tryWriteCachedAIVideo(w http.ResponseWriter, path string) bool {
	if !strings.HasPrefix(path, "/videos/") {
		return false
	}
	id := strings.TrimPrefix(path, "/videos/")
	content := strings.HasSuffix(id, "/content")
	id = strings.TrimSuffix(id, "/content")
	if tryWriteStoredAIVideo(w, id, content) {
		return true
	}
	value, ok := aiVideoCache.Load(id)
	if !ok {
		return false
	}
	cached := value.(cachedAIVideo)
	if time.Now().After(cached.ExpiresAt) {
		aiVideoCache.Delete(id)
		return false
	}
	if content {
		w.Header().Set("Content-Type", cached.MimeType)
		_, _ = w.Write(cached.Body)
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "status": "completed"})
	return true
}

func storeAIVideo(id string, body []byte, mimeType string) error {
	root := aiVideoStorageRoot()
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	filename := id + ".mp4"
	videoPath := filepath.Join(root, filename)
	tempPath := videoPath + ".tmp"
	if err := os.WriteFile(tempPath, body, 0644); err != nil {
		return err
	}
	if err := os.Rename(tempPath, videoPath); err != nil {
		return err
	}
	meta := storedAIVideo{
		ID:        id,
		Status:    "completed",
		MimeType:  firstNonEmpty(mimeType, "video/mp4"),
		Filename:  filename,
		Bytes:     len(body),
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	payload, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, id+".json"), append(payload, '\n'), 0644)
}

func tryWriteStoredAIVideo(w http.ResponseWriter, id string, content bool) bool {
	if id == "" || filepath.Base(id) != id || strings.Contains(id, "..") {
		return false
	}
	root := aiVideoStorageRoot()
	metaPath := filepath.Join(root, id+".json")
	body, err := os.ReadFile(metaPath)
	if err != nil {
		return false
	}
	var meta storedAIVideo
	if err := json.Unmarshal(body, &meta); err != nil {
		return false
	}
	if content {
		videoPath := filepath.Join(root, filepath.Base(meta.Filename))
		file, err := os.Open(videoPath)
		if err != nil {
			http.NotFound(w, nil)
			return true
		}
		defer file.Close()
		stat, err := file.Stat()
		if err != nil {
			http.NotFound(w, nil)
			return true
		}
		w.Header().Set("Content-Type", firstNonEmpty(meta.MimeType, "video/mp4"))
		http.ServeContent(w, &http.Request{Method: http.MethodGet}, filepath.Base(meta.Filename), stat.ModTime(), file)
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": meta.ID, "status": firstNonEmpty(meta.Status, "completed")})
	return true
}

func aiVideoStorageRoot() string {
	root := strings.TrimSpace(config.Cfg.VideoStorageRoot)
	if root == "" {
		root = "data/video"
	}
	return filepath.Clean(root)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func copyAIResponse(w http.ResponseWriter, request *http.Request, onFailure func()) {
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Printf("AI proxy request failed: url=%s err=%v", request.URL.String(), err)
		if onFailure != nil {
			onFailure()
		}
		Fail(w, "AI 接口请求失败")
		return
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		log.Printf("AI upstream error: url=%s status=%d body=%s", request.URL.String(), response.StatusCode, strings.TrimSpace(string(payload)))
		if onFailure != nil {
			onFailure()
		}
		Fail(w, "AI 接口请求失败")
		return
	}

	for key, values := range response.Header {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func readAIRequest(r *http.Request) ([]byte, string, string, error) {
	contentType := r.Header.Get("Content-Type")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, "", "", err
	}
	modelName := ""
	if strings.HasPrefix(contentType, "multipart/form-data") {
		modelName = readMultipartModel(body, contentType)
	} else {
		var payload struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &payload)
		modelName = payload.Model
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, "", "", errMissingModel
	}
	return body, contentType, modelName, nil
}

func readMultipartModel(body []byte, contentType string) string {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	form, err := reader.ReadForm(32 << 20)
	if err != nil {
		return ""
	}
	defer form.RemoveAll()
	if values := form.Value["model"]; len(values) > 0 {
		return values[0]
	}
	return ""
}

func readAIRequestCount(body []byte, contentType string) int {
	count := 1
	if strings.HasPrefix(contentType, "multipart/form-data") {
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return count
		}
		form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(32 << 20)
		if err != nil {
			return count
		}
		defer form.RemoveAll()
		if values := form.Value["n"]; len(values) > 0 {
			_, _ = fmt.Sscan(values[0], &count)
		}
	} else {
		var payload struct {
			N int `json:"n"`
		}
		_ = json.Unmarshal(body, &payload)
		count = payload.N
	}
	if count < 1 {
		return 1
	}
	return count
}

var errMissingModel = &aiError{"缺少模型名称"}

type aiError struct {
	message string
}

func (err *aiError) Error() string {
	return err.message
}
