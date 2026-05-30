package localworkspace

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanvasProjectRepositoryIndexAndServe(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root, Name: "Canvas Workspace"})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	project, err := NewCanvasProject(workspace, CanvasProjectData{
		Title:          "Local Canvas",
		BackgroundMode: CanvasBackgroundLines,
		Viewport:       CanvasProjectViewport{K: 1},
		Nodes: []CanvasProjectNodeData{
			{
				ID:       "node_image",
				Type:     "image",
				Title:    "Image",
				Position: CanvasPosition{X: 1, Y: 2},
				Width:    320,
				Height:   240,
				Metadata: map[string]any{"content": "asset://local"},
			},
		},
		Connections: []CanvasProjectConnection{
			{ID: "conn_1", FromNodeID: "node_image", ToNodeID: "node_image"},
		},
	})
	if err != nil {
		t.Fatalf("NewCanvasProject() error = %v", err)
	}
	if err := WriteCanvasProject(workspace, project); err != nil {
		t.Fatalf("WriteCanvasProject() error = %v", err)
	}
	assertIndexedCanvasProject(t, workspace, project.ID, 1, 1)
	if err := os.Remove(filepath.Join(root, IndexFileName)); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	if _, err := RebuildIndex(context.Background(), workspace, SQLiteIndexRebuilder{}, ScanOptions{}); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	assertIndexedCanvasProject(t, workspace, project.ID, 1, 1)

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

	createBody := `{"data":{"title":"From Web","nodes":[],"connections":[],"chatSessions":[],"activeChatId":null,"backgroundMode":"lines","viewport":{"x":0,"y":0,"k":1}}}`
	status, body := serveRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/canvas-projects", token, "", strings.NewReader(createBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"kind":"canvas_project"`) || !strings.Contains(body, `"title":"From Web"`) {
		t.Fatalf("canvas project create status=%d body=%s", status, body)
	}
	createdID := jsonPathString(t, body, "data.id")

	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/canvas-projects", token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, createdID) || !strings.Contains(body, `"canvasProjects"`) {
		t.Fatalf("canvas project list status=%d body=%s", status, body)
	}

	fileData := `{"title":"With File","nodes":[{"id":"node_image","type":"image","title":"Image","position":{"x":0,"y":0},"width":100,"height":100,"metadata":{"content":"workspace://canvas-file/node_image","workspaceFileKey":"node_image","mimeType":"image/png","naturalWidth":100,"naturalHeight":100}}],"connections":[],"chatSessions":[],"activeChatId":null,"backgroundMode":"lines","viewport":{"x":0,"y":0,"k":1},"files":{"node_image":{"role":"image","nodeId":"node_image","mime":"image/png","width":100,"height":100,"bytes":8}}}`
	status, body = serveCanvasProjectMultipartRequest(t, http.MethodPost, runtime.BaseURL+"/api/local/canvas-projects", token, fileData, "", "node_image", "tiny.png", "image/png", []byte("png-data"))
	if status != http.StatusOK || !strings.Contains(body, `"files":{"node_image"`) || !strings.Contains(body, `"path":"files/node_image.png"`) {
		t.Fatalf("canvas project multipart create status=%d body=%s", status, body)
	}
	fileProjectID := jsonPathString(t, body, "data.id")
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/canvas-projects/"+fileProjectID+"/files/node_image", token, "", nil, nil)
	if status != http.StatusOK || body != "png-data" {
		t.Fatalf("canvas project file status=%d body=%q", status, body)
	}
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/canvas-projects", token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, `"fileCount":1`) {
		t.Fatalf("canvas project list fileCount status=%d body=%s", status, body)
	}
	updateFileProjectBody := `{"revision":1,"data":{"title":"With File Renamed","nodes":[{"id":"node_image","type":"image","title":"Image","position":{"x":0,"y":0},"width":100,"height":100,"metadata":{"content":"workspace://canvas-file/node_image","workspaceFileKey":"node_image","mimeType":"image/png"}}],"connections":[],"chatSessions":[],"activeChatId":null,"backgroundMode":"lines","viewport":{"x":0,"y":0,"k":1}}}`
	status, body = serveRequest(t, http.MethodPut, runtime.BaseURL+"/api/local/canvas-projects/"+fileProjectID, token, "", strings.NewReader(updateFileProjectBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"revision":2`) || !strings.Contains(body, `"path":"files/node_image.png"`) {
		t.Fatalf("canvas project file-preserving update status=%d body=%s", status, body)
	}
	status, body = serveRequest(t, http.MethodGet, runtime.BaseURL+"/api/local/canvas-projects/"+fileProjectID+"/files/node_image", token, "", nil, nil)
	if status != http.StatusOK || body != "png-data" {
		t.Fatalf("canvas project preserved file status=%d body=%q", status, body)
	}

	updateBody := `{"revision":1,"data":{"title":"Renamed","nodes":[],"connections":[],"chatSessions":[],"activeChatId":null,"backgroundMode":"blank","viewport":{"x":1,"y":2,"k":1}}}`
	status, body = serveRequest(t, http.MethodPut, runtime.BaseURL+"/api/local/canvas-projects/"+createdID, token, "", strings.NewReader(updateBody), nil)
	if status != http.StatusOK || !strings.Contains(body, `"revision":2`) || !strings.Contains(body, `"title":"Renamed"`) {
		t.Fatalf("canvas project update status=%d body=%s", status, body)
	}

	status, body = serveRequest(t, http.MethodDelete, runtime.BaseURL+"/api/local/canvas-projects/"+createdID, token, "", nil, nil)
	if status != http.StatusOK || !strings.Contains(body, `"deleted":true`) {
		t.Fatalf("canvas project delete status=%d body=%s", status, body)
	}
	if _, err := ReadCanvasProject(workspace, createdID); err == nil {
		t.Fatalf("ReadCanvasProject(%s) after delete error = nil", createdID)
	}

	cancel()
	if err := waitServeDone(t, errCh); err != nil {
		t.Fatalf("Serve() error after cancel = %v", err)
	}
}

func TestCanvasProjectValidationRejectsBrokenConnection(t *testing.T) {
	workspace := Workspace{Root: t.TempDir()}
	_, err := NewCanvasProject(workspace, CanvasProjectData{
		Title:          "Broken Canvas",
		BackgroundMode: CanvasBackgroundLines,
		Viewport:       CanvasProjectViewport{K: 1},
		Nodes: []CanvasProjectNodeData{
			{ID: "node_image", Type: "image", Title: "Image", Position: CanvasPosition{}, Width: 320, Height: 240},
		},
		Connections: []CanvasProjectConnection{
			{ID: "conn_1", FromNodeID: "node_image", ToNodeID: "missing_node"},
		},
	})
	if err == nil {
		t.Fatal("NewCanvasProject() broken connection error = nil")
	}
	if !strings.Contains(err.Error(), "canvas connection references missing node") {
		t.Fatalf("NewCanvasProject() error = %v", err)
	}
}

func TestCanvasProjectValidationRejectsFileEscape(t *testing.T) {
	workspace := Workspace{Root: t.TempDir()}
	_, err := NewCanvasProject(workspace, CanvasProjectData{
		Title:          "Broken Files",
		BackgroundMode: CanvasBackgroundLines,
		Viewport:       CanvasProjectViewport{K: 1},
		Nodes: []CanvasProjectNodeData{
			{ID: "node_image", Type: "image", Title: "Image", Position: CanvasPosition{}, Width: 320, Height: 240},
		},
		Files: map[string]CanvasProjectFile{
			"node_image": {Path: "../outside.png"},
		},
	})
	if err == nil {
		t.Fatal("NewCanvasProject() escaping file error = nil")
	}
	if !strings.Contains(err.Error(), "canvas project file path escapes files directory") {
		t.Fatalf("NewCanvasProject() error = %v", err)
	}
}

func assertIndexedCanvasProject(t *testing.T, workspace Workspace, id string, nodes int, connections int) {
	t.Helper()
	items, err := ListCanvasProjectSummaries(workspace)
	if err != nil {
		t.Fatalf("ListCanvasProjectSummaries() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != id || items[0].NodeCount != nodes || items[0].ConnectionCount != connections {
		t.Fatalf("ListCanvasProjectSummaries() = %#v", items)
	}
}

func serveCanvasProjectMultipartRequest(t *testing.T, method string, url string, token string, data string, revision string, fileKey string, fileName string, contentType string, content []byte) (int, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("data", data); err != nil {
		t.Fatalf("write multipart data: %v", err)
	}
	if revision != "" {
		if err := writer.WriteField("revision", revision); err != nil {
			t.Fatalf("write multipart revision: %v", err)
		}
	}
	part, err := writer.CreatePart(textprotoMIMEHeader(map[string]string{
		`Content-Disposition`: `form-data; name="file:` + fileKey + `"; filename="` + fileName + `"`,
		`Content-Type`:        contentType,
	}))
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req, err := http.NewRequest(method, url, &body)
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
	responseData, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read multipart response: %v", err)
	}
	return resp.StatusCode, strings.TrimSpace(string(responseData))
}
