package localworkspace

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkbenchLogRepositoryIndexAndServe(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root, Name: "Workbench Workspace"})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace

	textLog, err := NewWorkbenchLog(workspace, WorkbenchLogData{
		Modality:        WorkbenchModalityText,
		Title:           "Text Log",
		CreatedAtMillis: 10,
		Status:          WorkbenchLogStatusSuccess,
		Model:           "text-model",
		Prompt:          "write",
		Payload:         map[string]any{"result": "ok"},
	})
	if err != nil {
		t.Fatalf("NewWorkbenchLog() error = %v", err)
	}
	if err := WriteWorkbenchLog(workspace, textLog); err != nil {
		t.Fatalf("WriteWorkbenchLog() error = %v", err)
	}
	assertIndexedWorkbenchLog(t, workspace, textLog.ID, WorkbenchModalityText, 0)
	if err := os.Remove(filepath.Join(root, IndexFileName)); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	if _, err := RebuildIndex(context.Background(), workspace, SQLiteIndexRebuilder{}, ScanOptions{}); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	assertIndexedWorkbenchLog(t, workspace, textLog.ID, WorkbenchModalityText, 0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan ServeRuntimeInfo, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, ServeOptions{
			WorkspacePath: root,
			Port:          0,
			Ready: func(runtime ServeRuntimeInfo) error {
				ready <- runtime
				return nil
			},
		})
	}()
	runtime := waitServeReady(t, ready)
	token := readServeToken(t, workspace)

	data := `{"modality":"image","title":"Image Log","createdAtMillis":20,"status":"success","model":"image-model","prompt":"draw","media":[{"key":"image_0","role":"result","width":1,"height":1}],"payload":{"prompt":"draw"}}`
	status, body := serveWorkbenchLogMultipartRequest(t, runtime.BaseURL+"/api/local/workbench-logs", token, data, map[string]workbenchLogTestFile{
		"image_0": {Name: "image.png", ContentType: "image/png", Content: []byte("png-data")},
	})
	if status != http.StatusOK || !strings.Contains(body, `"kind":"workbench_log"`) || !strings.Contains(body, `"path":"files/image_0.png"`) {
		t.Fatalf("workbench log create status=%d body=%s", status, body)
	}
	imageLogID := jsonPathString(t, body, "data.id")

	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/workbench-logs?modality=image", token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, imageLogID) || !strings.Contains(body, `"mediaCount":1`) {
		t.Fatalf("workbench log list status=%d body=%s", status, body)
	}
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/workbench-logs/"+imageLogID+"/files/image_0", token, "", nil, nil)
	if status != http.StatusOK || body != "png-data" {
		t.Fatalf("workbench log file status=%d body=%q", status, body)
	}

	status, body = serveRequest(t, http.MethodDelete, runtime.BaseURL+"/api/local/workbench-logs/"+imageLogID, token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, `"deleted":true`) {
		t.Fatalf("workbench log delete status=%d body=%s", status, body)
	}
	if _, err := ReadWorkbenchLog(workspace, imageLogID); err == nil {
		t.Fatalf("ReadWorkbenchLog(%s) after delete error = nil", imageLogID)
	}

	cancel()
	if err := waitServeDone(t, errCh); err != nil {
		t.Fatalf("Serve() error after cancel = %v", err)
	}
}

func TestWorkbenchLogRejectsPlaintextSecretsAndEscapes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	if _, err := NewWorkbenchLog(workspace, WorkbenchLogData{
		Modality: WorkbenchModalityText,
		Payload:  map[string]any{"apiKey": "plain"},
	}); err == nil {
		t.Fatal("NewWorkbenchLog() plaintext secret error = nil")
	}
	if _, err := NewWorkbenchLog(workspace, WorkbenchLogData{
		Modality: WorkbenchModalityImage,
		Media: []WorkbenchLogMedia{
			{Key: "original", Path: "../outside.png"},
		},
	}); err == nil {
		t.Fatal("NewWorkbenchLog() media path escape error = nil")
	}
}

type workbenchLogTestFile struct {
	Name        string
	ContentType string
	Content     []byte
}

func serveWorkbenchLogMultipartRequest(t *testing.T, url string, token string, data string, files map[string]workbenchLogTestFile) (int, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("data", data); err != nil {
		t.Fatalf("write multipart data: %v", err)
	}
	for key, file := range files {
		part, err := writer.CreatePart(textprotoMIMEHeader(map[string]string{
			`Content-Disposition`: `form-data; name="file:` + key + `"; filename="` + file.Name + `"`,
			`Content-Type`:        file.ContentType,
		}))
		if err != nil {
			t.Fatalf("create multipart file: %v", err)
		}
		if _, err := part.Write(file.Content); err != nil {
			t.Fatalf("write multipart file: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		t.Fatalf("new multipart request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("multipart request %s: %v", url, err)
	}
	defer resp.Body.Close()
	responseData := new(bytes.Buffer)
	if _, err := responseData.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read multipart response: %v", err)
	}
	return resp.StatusCode, strings.TrimSpace(responseData.String())
}

func assertIndexedWorkbenchLog(t *testing.T, workspace Workspace, id string, modality string, mediaCount int) {
	t.Helper()
	items, err := ListWorkbenchLogSummaries(workspace, modality)
	if err != nil {
		t.Fatalf("ListWorkbenchLogSummaries() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != id || items[0].Modality != modality || items[0].MediaCount != mediaCount {
		t.Fatalf("ListWorkbenchLogSummaries() = %#v", items)
	}
}
