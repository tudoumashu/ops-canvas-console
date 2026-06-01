package localworkspace

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	DefaultServeHost = "127.0.0.1"
	DefaultServePort = 17680
)

type ServeOptions struct {
	WorkspacePath  string
	Host           string
	Port           int
	AllowedOrigins []string
	Ready          func(ServeRuntimeInfo) error
	Now            func() time.Time
}

type ServeRuntimeInfo struct {
	Active           bool   `json:"active"`
	PID              int    `json:"pid"`
	Host             string `json:"host"`
	Port             int    `json:"port"`
	BaseURL          string `json:"baseUrl"`
	WorkspaceID      string `json:"workspaceId"`
	StartedAt        string `json:"startedAt"`
	TokenFile        string `json:"tokenFile"`
	LaunchSecretFile string `json:"launchSecretFile"`
}

type serveResponse struct {
	Code int    `json:"code"`
	Data any    `json:"data"`
	Msg  string `json:"msg"`
}

type serveAPI struct {
	workspace        Workspace
	bearerToken      string
	launchSecret     string
	launchSecretPath string
	launchConsumed   bool
	sessionStore     *serveSessionStore
	allowedOrigins   map[string]bool
	runtime          ServeRuntimeInfo
	mu               sync.Mutex
}

func Serve(ctx context.Context, opts ServeOptions) error {
	workspace, err := Open(opts.WorkspacePath)
	if err != nil {
		return err
	}
	host := strings.TrimSpace(opts.Host)
	if host == "" {
		host = DefaultServeHost
	}
	if err := validateServeHost(host); err != nil {
		return err
	}
	port := opts.Port
	if port < 0 {
		port = DefaultServePort
	}
	origins, err := normalizeAllowedOrigins(opts.AllowedOrigins)
	if err != nil {
		return err
	}
	stateDir, err := workspace.StateDir()
	if err != nil {
		return err
	}
	if err := ensurePrivateStateDir(stateDir); err != nil {
		return err
	}
	if err := clearStaleServeState(*workspace); err != nil {
		return err
	}
	serveLockPath := filepath.Join(stateDir, "serve.lock")
	lock, err := AcquireLock(serveLockPath)
	if err != nil {
		return err
	}
	defer lock.Release()

	bearerTokenPath := filepath.Join(stateDir, "bearer.token")
	bearerToken, err := readOrCreateServeSecret(bearerTokenPath)
	if err != nil {
		return err
	}
	launchSecret, err := newServeSecret()
	if err != nil {
		return err
	}
	launchSecretPath := filepath.Join(stateDir, "launch.secret")
	if err := AtomicWriteFile(launchSecretPath, []byte(launchSecret+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(launchSecretPath, 0o600); err != nil {
		return WrapError(ErrorInternal, "chmod launch secret", 5, err)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return WrapError(ErrorInvalidArgument, "start opsc serve listener", 1, err)
	}
	defer listener.Close()

	actualPort, err := listenerPort(listener)
	if err != nil {
		return err
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	startedAt := now().UTC().Format(time.RFC3339)
	baseURL := "http://" + net.JoinHostPort(host, strconv.Itoa(actualPort))
	tokenFile := "bearer.token"
	launchSecretFile := "launch.secret"
	metadata := RuntimeMetadata{
		SchemaVersion:    SchemaVersion,
		PID:              os.Getpid(),
		Host:             host,
		Port:             actualPort,
		BaseURL:          baseURL,
		WorkspaceID:      workspace.Document.ID,
		WorkspacePath:    "<redacted>",
		StartedAt:        startedAt,
		TokenFile:        tokenFile,
		LaunchSecretFile: launchSecretFile,
	}
	if err := writeServeRuntimeFiles(*workspace, metadata); err != nil {
		return err
	}
	defer cleanupServeRuntimeFiles(*workspace)

	runtime := ServeRuntimeInfo{
		Active:           true,
		PID:              metadata.PID,
		Host:             metadata.Host,
		Port:             metadata.Port,
		BaseURL:          metadata.BaseURL,
		WorkspaceID:      metadata.WorkspaceID,
		StartedAt:        metadata.StartedAt,
		TokenFile:        tokenFile,
		LaunchSecretFile: launchSecretFile,
	}
	sessionStore := &serveSessionStore{
		path:     filepath.Join(stateDir, "sessions.json"),
		now:      now,
		sessions: map[string]serveSession{},
	}
	api := &serveAPI{
		workspace:        *workspace,
		bearerToken:      bearerToken,
		launchSecret:     launchSecret,
		launchSecretPath: launchSecretPath,
		sessionStore:     sessionStore,
		allowedOrigins:   origins,
		runtime:          runtime,
	}
	server := &http.Server{
		Handler:           api,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if opts.Ready != nil {
		if err := opts.Ready(runtime); err != nil {
			_ = server.Close()
			return err
		}
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
		case <-done:
		}
	}()
	err = server.Serve(listener)
	close(done)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	if err != nil {
		return WrapError(ErrorInternal, "run opsc serve", 5, err)
	}
	return nil
}

func (api *serveAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !api.applyCORS(w, r) {
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.URL.Path == "/health" || r.URL.Path == "/api/health" {
		if r.Method != http.MethodGet {
			writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
		return
	}
	if r.URL.Path == "/api/local/bootstrap/session" {
		if r.Method != http.MethodPost {
			writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
			return
		}
		api.handleBootstrapSession(w, r)
		return
	}
	if !api.authorized(r) {
		writeServeError(w, http.StatusUnauthorized, NewError(ErrorAuthFailed, "missing or invalid authentication", 3, nil))
		return
	}
	api.route(w, r)
}

func (api *serveAPI) applyCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	if !api.allowedOrigins[origin] {
		writeServeError(w, http.StatusForbidden, NewError(ErrorAuthFailed, "origin is not allowed", 3, nil))
		return false
	}
	header := w.Header()
	header.Set("Access-Control-Allow-Origin", origin)
	header.Set("Access-Control-Allow-Credentials", "true")
	header.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	header.Set("Vary", "Origin")
	return true
}

func (api *serveAPI) authorized(r *http.Request) bool {
	if strings.TrimSpace(r.Header.Get("Origin")) != "" {
		return api.sessionStore.authorized(r)
	}
	if api.sessionStore.authorized(r) {
		return true
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	return subtle.ConstantTimeCompare([]byte(token), []byte(api.bearerToken)) == 1
}

func (api *serveAPI) route(w http.ResponseWriter, r *http.Request) {
	localPath, ok := localAPIPath(r.URL.Path)
	if !ok {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "route not found", 2, nil))
		return
	}
	switch {
	case r.Method == http.MethodGet && localPath == "/runtime":
		writeServeSuccess(w, api.runtime, nil)
	case r.Method == http.MethodGet && localPath == "/workspace":
		writeServeSuccess(w, api.workspace.Info(parseBoolQuery(r, "showPaths")), nil)
	case localPath == "/workspace/preferences":
		api.handleWorkspacePreferences(w, r)
	case r.Method == http.MethodGet && localPath == "/workspace/doctor":
		api.handleDoctor(w)
	case r.Method == http.MethodPost && localPath == "/workspace/index/rebuild":
		api.handleIndexRebuild(w, r)
	case r.Method == http.MethodGet && localPath == "/workspace/export/plan":
		api.handleExportPlan(w)
	case r.Method == http.MethodGet && localPath == "/workspace/gc/plan":
		api.handleGCPlan(w)
	case localPath == "/templates" || strings.HasPrefix(localPath, "/templates/"):
		api.handleTemplateRoute(w, r, localPath)
	case localPath == "/runs" || strings.HasPrefix(localPath, "/runs/"):
		api.handleRunRoute(w, r, localPath)
	case localPath == "/artifacts" || strings.HasPrefix(localPath, "/artifacts/"):
		api.handleArtifactRoute(w, r, localPath)
	case localPath == "/profiles" || strings.HasPrefix(localPath, "/profiles/"):
		api.handleProfileRoute(w, r, localPath)
	case localPath == "/projects" || strings.HasPrefix(localPath, "/projects/"):
		api.handleProjectRoute(w, r, localPath)
	case localPath == "/assets" || strings.HasPrefix(localPath, "/assets/"):
		api.handleAssetRoute(w, r, localPath)
	case localPath == "/prompts" || strings.HasPrefix(localPath, "/prompts/"):
		api.handlePromptRoute(w, r, localPath)
	case localPath == "/canvas-projects" || strings.HasPrefix(localPath, "/canvas-projects/"):
		api.handleCanvasProjectRoute(w, r, localPath)
	case localPath == "/workbench-logs" || strings.HasPrefix(localPath, "/workbench-logs/"):
		api.handleWorkbenchLogRoute(w, r, localPath)
	case strings.HasPrefix(localPath, "/ai/v1/"):
		api.handleAIProxy(w, r, localPath)
	default:
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "route not found", 2, nil))
	}
}

func (api *serveAPI) handleDoctor(w http.ResponseWriter) {
	report, err := Doctor(DoctorOptions{Path: api.workspace.Root, CheckLock: true})
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, report, report.Warnings)
}

func (api *serveAPI) handleIndexRebuild(w http.ResponseWriter, r *http.Request) {
	scan, err := RebuildIndex(r.Context(), api.workspace, SQLiteIndexRebuilder{}, ScanOptions{})
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, map[string]any{
		"workspaceId": api.workspace.Document.ID,
		"entries":     len(scan.Entries),
	}, scan.Warnings)
}

func (api *serveAPI) handleExportPlan(w http.ResponseWriter) {
	plan, err := BuildExportPlan(api.workspace, ExportPlanOptions{})
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, plan, plan.Warnings)
}

func (api *serveAPI) handleGCPlan(w http.ResponseWriter) {
	plan, err := BuildGCPlan(api.workspace, GCPlanOptions{})
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	writeServeSuccess(w, plan, plan.Warnings)
}

func (api *serveAPI) handleRunRoute(w http.ResponseWriter, r *http.Request, localPath string) {
	if localPath == "/runs" {
		switch r.Method {
		case http.MethodGet:
			items, err := ListRunSummaries(api.workspace)
			if err != nil {
				writeServeErrorFromError(w, err)
				return
			}
			writeServeSuccess(w, map[string]any{"runs": items}, nil)
		case http.MethodPost:
			api.createRun(w, r)
		default:
			writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
		}
		return
	}
	parts := splitAPISuffix(localPath, "/runs/")
	if len(parts) != 1 && len(parts) != 2 && len(parts) != 3 {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "run route not found", 2, nil))
		return
	}
	runID := parts[0]
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		document, err := ReadRun(api.workspace, runID)
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, document, nil)
	case len(parts) == 1 && r.Method == http.MethodPut:
		api.updateRun(w, r, runID)
	case len(parts) == 2 && parts[1] == "status" && r.Method == http.MethodGet:
		snapshot, err := GetRunStatus(api.workspace, runID)
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, snapshot, nil)
	case len(parts) == 2 && parts[1] == "pdd-overview" && r.Method == http.MethodGet:
		overview, err := buildLocalPDDRunOverview(api.workspace, runID, api.runtime.BaseURL)
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, overview, nil)
	case len(parts) == 2 && parts[1] == "pdd-product-detail" && r.Method == http.MethodGet:
		detail, err := buildLocalPDDProductDetail(api.workspace, runID, r.URL.Query().Get("key"), api.runtime.BaseURL)
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, detail, nil)
	case len(parts) == 2 && parts[1] == "creative-canvas":
		api.handleLocalPDDCreativeCanvas(w, r, runID)
	case len(parts) == 3 && parts[1] == "creative-canvas" && parts[2] == "assets":
		api.handleLocalPDDCreativeCanvasAsset(w, r, runID)
	case len(parts) == 3 && parts[1] == "creative-canvas" && parts[2] == "apply":
		api.handleLocalPDDCreativeCanvasApply(w, r, runID)
	case len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodGet:
		after, err := parseAfterSequence(r)
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		events, err := ReadRunEvents(api.workspace, runID, after)
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, map[string]any{"runId": runID, "events": events}, nil)
	case len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodPost:
		api.appendRunEvent(w, r, runID)
	case len(parts) == 3 && parts[1] == "events" && parts[2] == "stream" && r.Method == http.MethodGet:
		api.streamRunEvents(w, r, runID)
	case len(parts) == 2 && parts[1] == "artifacts" && r.Method == http.MethodGet:
		items, err := ListRunArtifactSummaries(api.workspace, runID)
		if err != nil {
			writeServeErrorFromError(w, err)
			return
		}
		writeServeSuccess(w, map[string]any{"runId": runID, "artifacts": items}, nil)
	case len(parts) == 2 && parts[1] == "artifacts" && r.Method == http.MethodPost:
		api.attachRunArtifact(w, r, runID)
	case len(parts) == 3 && parts[1] == "nodes" && (r.Method == http.MethodPost || r.Method == http.MethodPut):
		api.writeRunNodeState(w, r, runID, parts[2])
	default:
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "run route not found", 2, nil))
	}
}

func (api *serveAPI) streamRunEvents(w http.ResponseWriter, r *http.Request, runID string) {
	after, err := parseAfterSequence(r)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeServeError(w, http.StatusInternalServerError, NewError(ErrorInternal, "streaming is not supported", 5, nil))
		return
	}
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	_ = FollowRunEvents(r.Context(), api.workspace, runID, after, 500*time.Millisecond, func(event RunEventEnvelope) error {
		encoded, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", encoded); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	})
}

type bootstrapSessionRequest struct {
	LaunchSecret string `json:"launchSecret"`
}

func (api *serveAPI) handleBootstrapSession(w http.ResponseWriter, r *http.Request) {
	var payload bootstrapSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeServeError(w, http.StatusBadRequest, NewError(ErrorInvalidArgument, "bootstrap request is invalid", 1, nil))
		return
	}
	secret := strings.TrimSpace(payload.LaunchSecret)
	api.mu.Lock()
	valid := !api.launchConsumed && subtle.ConstantTimeCompare([]byte(secret), []byte(api.launchSecret)) == 1
	if valid {
		api.launchConsumed = true
		_ = os.Remove(api.launchSecretPath)
	}
	api.mu.Unlock()
	if !valid {
		writeServeError(w, http.StatusUnauthorized, NewError(ErrorAuthFailed, "launch secret is invalid or already used", 3, nil))
		return
	}
	sessionID, expiresAt, err := api.sessionStore.create()
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     serveSessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
	writeServeSuccess(w, map[string]any{
		"authenticated": true,
		"expiresAt":     expiresAt.UTC().Format(time.RFC3339),
	}, nil)
}

func (api *serveAPI) handleArtifactRoute(w http.ResponseWriter, r *http.Request, localPath string) {
	if localPath == "/artifacts" {
		switch r.Method {
		case http.MethodGet:
			api.handleArtifacts(w)
		case http.MethodPost:
			api.createArtifact(w, r)
		default:
			writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
		}
		return
	}
	if localPath == "/artifacts/import" && r.Method == http.MethodPost {
		api.createArtifactImport(w, r)
		return
	}
	parts := splitAPISuffix(localPath, "/artifacts/")
	if len(parts) == 2 && parts[1] == "import" && r.Method == http.MethodPut {
		api.updateArtifactImport(w, r, parts[0])
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			document, err := ReadArtifact(api.workspace, parts[0])
			if err != nil {
				writeServeErrorFromError(w, err)
				return
			}
			writeServeSuccess(w, document, nil)
		case http.MethodPut:
			api.updateArtifact(w, r, parts[0])
		default:
			writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
		}
		return
	}
	if len(parts) != 3 || parts[1] != "files" || r.Method != http.MethodGet {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "artifact route not found", 2, nil))
		return
	}
	document, err := ReadArtifact(api.workspace, parts[0])
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	serveObjectFile(w, r, ArtifactRepository(api.workspace).Dir(document.ID), document.Data.Files[parts[2]], document.Data.MIME)
}

func (api *serveAPI) handleAssetFileRoute(w http.ResponseWriter, r *http.Request, localPath string) {
	parts := splitAPISuffix(localPath, "/assets/")
	if len(parts) != 3 || parts[1] != "files" {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "asset route not found", 2, nil))
		return
	}
	document, err := ReadAsset(api.workspace, parts[0])
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	serveObjectFile(w, r, AssetRepository(api.workspace).Dir(document.ID), document.Data.Files[parts[2]], document.Data.MIME)
}

func (api *serveAPI) handlePromptContentRoute(w http.ResponseWriter, r *http.Request, localPath string) {
	parts := splitAPISuffix(localPath, "/prompts/")
	if len(parts) != 2 || parts[1] != "content" {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "prompt route not found", 2, nil))
		return
	}
	content, err := ReadPromptContent(api.workspace, parts[0])
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, content)
}

func serveObjectFile(w http.ResponseWriter, r *http.Request, baseDir string, relPath string, fallbackMIME string) {
	if strings.TrimSpace(relPath) == "" {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "file ref not found", 2, nil))
		return
	}
	if !isWorkspaceRelativeFile(relPath) {
		writeServeError(w, http.StatusUnprocessableEntity, NewError(ErrorWorkspaceInvalid, "file ref escapes object directory", 2, nil))
		return
	}
	filePath := filepath.Join(baseDir, filepath.FromSlash(relPath))
	rel, err := filepath.Rel(baseDir, filePath)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		writeServeError(w, http.StatusUnprocessableEntity, NewError(ErrorWorkspaceInvalid, "file ref escapes object directory", 2, nil))
		return
	}
	stat, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "file not found", 2, nil))
			return
		}
		writeServeErrorFromError(w, WrapError(ErrorInternal, "stat object file", 5, err))
		return
	}
	if stat.IsDir() {
		writeServeError(w, http.StatusNotFound, NewError(ErrorWorkspaceNotFound, "file not found", 2, nil))
		return
	}
	if fallbackMIME != "" {
		w.Header().Set("Content-Type", fallbackMIME)
	} else if ext := filepath.Ext(filePath); ext != "" {
		if value := mime.TypeByExtension(ext); value != "" {
			w.Header().Set("Content-Type", value)
		}
	}
	http.ServeFile(w, r, filePath)
}

func splitAPISuffix(pathValue string, prefix string) []string {
	suffix := strings.Trim(strings.TrimPrefix(pathValue, prefix), "/")
	if suffix == "" {
		return nil
	}
	parts := strings.Split(suffix, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func localAPIPath(pathValue string) (string, bool) {
	if pathValue == "/api/local" {
		return "/", true
	}
	if strings.HasPrefix(pathValue, "/api/local/") {
		return strings.TrimPrefix(pathValue, "/api/local"), true
	}
	return "", false
}

func parseAfterSequence(r *http.Request) (int64, error) {
	value := strings.TrimSpace(r.URL.Query().Get("after"))
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, NewError(ErrorInvalidArgument, "after must be a non-negative integer", 1, nil)
	}
	return parsed, nil
}

func writeServeSuccess(w http.ResponseWriter, data any, warnings []string) {
	if len(warnings) > 0 {
		data = map[string]any{
			"result":   data,
			"warnings": warnings,
		}
	}
	writeJSON(w, http.StatusOK, serveResponse{Code: 0, Data: data, Msg: "ok"})
}

func writeServeErrorFromError(w http.ResponseWriter, err error) {
	var workspaceErr *Error
	if errors.As(err, &workspaceErr) {
		writeServeError(w, httpStatusForError(workspaceErr), workspaceErr)
		return
	}
	writeServeError(w, http.StatusInternalServerError, WrapError(ErrorInternal, "unexpected error", 5, err))
}

func writeServeError(w http.ResponseWriter, status int, err *Error) {
	msg := "操作失败"
	if err != nil && strings.TrimSpace(err.Message) != "" {
		msg = err.Message
	}
	writeJSON(w, status, serveResponse{Code: 1, Data: nil, Msg: msg})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func httpStatusForError(err *Error) int {
	if err == nil {
		return http.StatusInternalServerError
	}
	switch err.Code {
	case ErrorInvalidArgument:
		return http.StatusBadRequest
	case ErrorWorkspaceNotFound:
		return http.StatusNotFound
	case ErrorWorkspaceLocked:
		return http.StatusConflict
	case ErrorWorkspaceInvalid, ErrorWorkspaceUnhealthy:
		return http.StatusUnprocessableEntity
	case ErrorAuthFailed:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

func validateServeHost(host string) error {
	if !isLoopbackHost(host) {
		return NewError(ErrorInvalidArgument, "opsc serve only supports loopback host", 1, map[string]string{"host": host})
	}
	return nil
}

func normalizeAllowedOrigins(values []string) (map[string]bool, error) {
	origins := map[string]bool{}
	for _, raw := range values {
		for _, item := range strings.Split(raw, ",") {
			origin := strings.TrimRight(strings.TrimSpace(item), "/")
			if origin == "" {
				continue
			}
			if origin == "*" {
				return nil, NewError(ErrorInvalidArgument, "CORS origin wildcard is not allowed", 1, nil)
			}
			parsed, err := url.Parse(origin)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
				return nil, NewError(ErrorInvalidArgument, "CORS origin is invalid", 1, map[string]string{"origin": origin})
			}
			if parsed.Scheme != "http" && parsed.Scheme != "https" {
				return nil, NewError(ErrorInvalidArgument, "CORS origin scheme is not allowed", 1, map[string]string{"origin": origin})
			}
			host := parsed.Hostname()
			if !isLoopbackHost(host) {
				return nil, NewError(ErrorInvalidArgument, "CORS origin must be loopback", 1, map[string]string{"origin": origin})
			}
			origins[origin] = true
		}
	}
	return origins, nil
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func newServeSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", WrapError(ErrorInternal, "generate serve secret", 5, err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func readOrCreateServeSecret(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		value := strings.TrimSpace(string(data))
		if value == "" {
			return "", NewError(ErrorWorkspaceInvalid, "serve secret file is empty", 2, nil)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return "", WrapError(ErrorInternal, "chmod serve secret", 5, err)
		}
		return value, nil
	} else if !os.IsNotExist(err) {
		return "", WrapError(ErrorInternal, "read serve secret", 5, err)
	}
	secret, err := newServeSecret()
	if err != nil {
		return "", err
	}
	if err := AtomicWriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", WrapError(ErrorInternal, "chmod serve secret", 5, err)
	}
	return secret, nil
}

func listenerPort(listener net.Listener) (int, error) {
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, NewError(ErrorInternal, "listener address is not TCP", 5, nil)
	}
	return addr.Port, nil
}

func writeServeRuntimeFiles(workspace Workspace, metadata RuntimeMetadata) error {
	if err := AtomicWriteFile(workspace.RuntimePath("serve.pid"), []byte(strconv.Itoa(metadata.PID)+"\n"), 0o600); err != nil {
		return err
	}
	if err := AtomicWriteFile(workspace.RuntimePath("serve.port"), []byte(strconv.Itoa(metadata.Port)+"\n"), 0o600); err != nil {
		return err
	}
	return AtomicWriteJSON(workspace.RuntimePath("serve.json"), metadata, 0o600)
}

func cleanupServeRuntimeFiles(workspace Workspace) {
	for _, name := range []string{"serve.json", "serve.pid", "serve.port", "launch.secret"} {
		_ = os.Remove(workspace.RuntimePath(name))
	}
	if dir, err := workspace.StateDir(); err == nil {
		syncDir(dir)
	}
}

func clearStaleServeState(workspace Workspace) error {
	lockPath := workspace.LockPath("serve.lock")
	if _, err := os.Stat(lockPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return WrapError(ErrorInternal, "stat serve lock", 5, err)
	}
	pid, err := readServePID(workspace)
	if err == nil && pid > 0 && processExists(pid) {
		return nil
	}
	cleanupServeRuntimeFiles(workspace)
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return WrapError(ErrorInternal, "remove stale serve lock", 5, err)
	}
	return nil
}

func readServePID(workspace Workspace) (int, error) {
	data, err := os.ReadFile(workspace.RuntimePath("serve.pid"))
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	return value, nil
}

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

const serveSessionCookieName = "opsc_session"

type serveSession struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	ExpiresAt string `json:"expiresAt"`
}

type serveSessionStore struct {
	path     string
	now      func() time.Time
	mu       sync.Mutex
	sessions map[string]serveSession
}

func (s *serveSessionStore) create() (string, time.Time, error) {
	sessionID, err := newServeSecret()
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	current := now().UTC()
	expiresAt := current.Add(24 * time.Hour)
	record := serveSession{
		ID:        sessionID,
		CreatedAt: current.Format(time.RFC3339),
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}
	s.mu.Lock()
	if s.sessions == nil {
		s.sessions = map[string]serveSession{}
	}
	s.sessions[sessionID] = record
	err = s.writeLocked()
	s.mu.Unlock()
	if err != nil {
		return "", time.Time{}, err
	}
	return sessionID, expiresAt, nil
}

func (s *serveSessionStore) authorized(r *http.Request) bool {
	cookie, err := r.Cookie(serveSessionCookieName)
	if err != nil {
		return false
	}
	value := strings.TrimSpace(cookie.Value)
	if value == "" {
		return false
	}
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.sessions[value]
	if !ok {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, record.ExpiresAt)
	if err != nil || !now().UTC().Before(expiresAt) {
		delete(s.sessions, value)
		_ = s.writeLocked()
		return false
	}
	return true
}

func (s *serveSessionStore) writeLocked() error {
	if s.path == "" {
		return nil
	}
	data := struct {
		Sessions []serveSession `json:"sessions"`
	}{Sessions: make([]serveSession, 0, len(s.sessions))}
	for _, session := range s.sessions {
		data.Sessions = append(data.Sessions, session)
	}
	if err := AtomicWriteJSON(s.path, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return WrapError(ErrorInternal, "chmod serve sessions", 5, err)
	}
	return nil
}
