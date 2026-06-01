package localworkspace

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const executorTestPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/l6r3JwAAAABJRU5ErkJggg=="

func TestRunExecutorOnceExecutesFixedMaterialTextAndImage(t *testing.T) {
	t.Setenv("OPSC_EXECUTOR_TEST_KEY", "provider-secret")
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, profileID, assetID := seedExecutorWorkspace(t, root)

	var providerAuths []string
	var providerPaths []string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerAuths = append(providerAuths, r.Header.Get("Authorization"))
		providerPaths = append(providerPaths, r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/chat/completions":
			if !strings.Contains(string(body), "Mug") || !strings.Contains(string(body), "art_") {
				t.Fatalf("chat payload missing rendered prompt context: %s", body)
			}
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"short product copy"}}]}`)
		case "/v1/images/generations":
			if !strings.Contains(string(body), "short product copy") {
				t.Fatalf("image payload missing text node output: %s", body)
			}
			_, _ = io.WriteString(w, `{"data":[{"b64_json":"`+executorTestPNGBase64+`"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	profile := readExecutorProfile(t, workspace, profileID)
	profile.Data.Channels[0].BaseURL = provider.URL
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}

	template := writeExecutorTemplate(t, workspace, []map[string]any{
		{"id": "input", "type": "input", "operation": "input", "title": "Input"},
		{"id": "material", "type": "material_lookup", "operation": "material_lookup", "title": "Fixed Material", "extra": map[string]any{"assetMode": "fixed", "assetId": assetID}},
		{"id": "copy", "type": "text_generation", "operation": "text_generation", "title": "Copy", "model": "gpt-test", "prompt": "Write {{input.productTitle}} with {{node.material.artifactId}}"},
		{"id": "image", "type": "image_generation", "operation": "image_generation", "title": "Image", "model": "image-test", "prompt": "Render {{node.copy.text}}"},
	}, []map[string]any{
		{"from": "input", "to": "copy"},
		{"from": "material", "to": "copy"},
		{"from": "copy", "to": "image"},
	})
	run := writeExecutorRun(t, workspace, template.ID, profile.ID, RunStatusPending, map[string]any{"productTitle": "Mug"}, nil)
	appendWaitingForExecutor(t, workspace, run.ID)

	result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root})
	if err != nil {
		t.Fatalf("RunExecutorOnce() error = %v", err)
	}
	if result.Processed != 1 || len(result.Runs) != 1 {
		t.Fatalf("executor result = %#v, want one processed run", result)
	}
	if got := result.Runs[0]; got.Status != RunStatusSuccess || got.Executed != 4 || got.Skipped != 0 || got.ArtifactRefs != 3 {
		t.Fatalf("run result = %#v, want success with material/text/image artifacts", got)
	}
	for _, auth := range providerAuths {
		if auth != "Bearer provider-secret" {
			t.Fatalf("provider auth = %q, want secretRef bearer", auth)
		}
	}
	if strings.Join(providerPaths, ",") != "/v1/chat/completions,/v1/images/generations" {
		t.Fatalf("provider paths = %#v", providerPaths)
	}

	status, err := GetRunStatus(workspace, run.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() error = %v", err)
	}
	if status.Run.Status != RunStatusSuccess || status.Run.ArtifactCount != 3 {
		t.Fatalf("run status = %#v, want success with three artifacts", status.Run)
	}
	states, err := ListRunNodeStates(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunNodeStates() error = %v", err)
	}
	if len(states) != 4 {
		t.Fatalf("node states len = %d, want 4", len(states))
	}
	for _, state := range states {
		if state.Data.Status != RunStatusSuccess {
			t.Fatalf("node %s status = %s, want success", state.ID, state.Data.Status)
		}
	}
	events, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	for _, want := range []string{executorEventClaimed, executorEventNodeStarted, executorEventNodeCompleted, executorEventRunCompleted} {
		if !runEventTypesContain(events, want) {
			t.Fatalf("events missing %s: %#v", want, events)
		}
	}

	second, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root})
	if err != nil {
		t.Fatalf("RunExecutorOnce() second error = %v", err)
	}
	if second.Processed != 0 {
		t.Fatalf("second executor result = %#v, want no duplicate work for success run", second)
	}
	refs, err := ListRunArtifacts(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunArtifacts() error = %v", err)
	}
	if len(refs) != 3 {
		t.Fatalf("artifact refs after second run = %d, want 3", len(refs))
	}
	assertNoExecutorSecretLeak(t, status, events, refs)

	if err := os.Remove(filepath.Join(root, IndexFileName)); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	if _, err := RebuildIndex(context.Background(), workspace, SQLiteIndexRebuilder{}, ScanOptions{}); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	rebuiltStatus, err := GetRunStatus(workspace, run.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() after rebuild error = %v", err)
	}
	if rebuiltStatus.Run.Status != RunStatusSuccess || len(rebuiltStatus.Nodes) != 4 || rebuiltStatus.LatestEventSequence == 0 {
		t.Fatalf("rebuilt run status = %#v, want success with nodes and events", rebuiltStatus)
	}
	rebuiltRefs, err := ListRunArtifactSummaries(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunArtifactSummaries() after rebuild error = %v", err)
	}
	if len(rebuiltRefs) != 3 {
		t.Fatalf("rebuilt run artifact refs = %d, want 3", len(rebuiltRefs))
	}
}

func TestRunExecutorOnceExecutesLocalEcommerceTemplate(t *testing.T) {
	t.Setenv("OPSC_EXECUTOR_TEST_KEY", "provider-secret")
	t.Setenv("OPSC_EXECUTOR_WRONG_KEY", "wrong-provider-secret")
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, profileID, _ := seedExecutorWorkspace(t, root)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, "outputs"), 0o755); err != nil {
		t.Fatalf("create project outputs: %v", err)
	}
	project := writeExecutorProject(t, workspace, projectRoot, ProjectCapabilities{FSRead: true, FSWrite: true, ArtifactWrite: true})
	materialRoot := filepath.Join(t.TempDir(), "materials")
	materialDir := filepath.Join(materialRoot, "物语系列", "羽川翼")
	if err := os.MkdirAll(materialDir, 0o755); err != nil {
		t.Fatalf("create material dir: %v", err)
	}
	imageData, err := base64.StdEncoding.DecodeString(executorTestPNGBase64)
	if err != nil {
		t.Fatalf("decode material png: %v", err)
	}
	if err := AtomicWriteFile(filepath.Join(materialDir, "reference.png"), imageData, 0o600); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	if err := AtomicWriteFile(filepath.Join(materialDir, "metadata.json"), []byte(`{"work":"物语系列","character":"羽川翼","aliases":["翼"],"presentation":"anime character reference"}`), 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	var editCalls int
	var auths []string
	var uploadedCounts []int
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("Authorization"))
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("provider path = %s, want image edits", r.URL.Path)
		}
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		editCalls++
		uploadedCounts = append(uploadedCounts, len(r.MultipartForm.File["image"]))
		if !strings.Contains(r.FormValue("prompt"), "羽川翼抱枕") || !strings.Contains(r.FormValue("prompt"), "1:") {
			t.Fatalf("edit prompt missing product or image order: %q", r.FormValue("prompt"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"`+executorTestPNGBase64+`"}]}`)
	}))
	defer provider.Close()

	profile := readExecutorProfile(t, workspace, profileID)
	profile.Data.Channels = []ProfileChannel{
		{
			ID:        "wrong",
			Protocol:  "openai-compatible",
			BaseURL:   "http://127.0.0.1:1",
			Enabled:   true,
			SecretRef: &SecretRef{Type: SecretRefTypeEnv, Name: "OPSC_EXECUTOR_WRONG_KEY"},
		},
		{
			ID:        "edits",
			Protocol:  "openai-compatible",
			BaseURL:   provider.URL,
			Enabled:   true,
			SecretRef: &SecretRef{Type: SecretRefTypeEnv, Name: "OPSC_EXECUTOR_TEST_KEY"},
		},
	}
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}

	template := writeExecutorTemplate(t, workspace, []map[string]any{
		{"id": "input", "type": "input", "operation": "input", "title": "Input"},
		{"id": "reference", "type": "material_lookup", "operation": "material_lookup", "title": "Reference", "extra": map[string]any{"assetMode": "auto", "materialLibrary": localEcommerceMaterialLibrary, "materialLibraryPath": materialRoot}},
		{"id": "source", "type": "image_edit", "operation": "image_edit", "title": "Source", "model": "image-edit-test", "prompt": "{{productTitle}} source {{uploaded_image_order}}"},
		{"id": "mockup_base", "type": "material_lookup", "operation": "material_lookup", "title": "Mockup Base", "extra": map[string]any{"assetMode": "fixed", "assetId": builtinPDDMockupBaseAssetID, "fallback": "builtin_pdd_mockup_base"}},
		{"id": "mockup", "type": "image_edit", "operation": "image_edit", "title": "Mockup", "model": "image-edit-test", "prompt": "{{productTitle}} mockup {{uploaded_image_order}}"},
		{"id": "main", "type": "image_edit", "operation": "image_edit", "title": "Main", "model": "image-edit-test", "prompt": "{{productTitle}} main {{uploaded_image_order}}"},
		{"id": "package", "type": "script", "operation": "script", "title": "Package", "extra": map[string]any{"executor": "local", "localEcommerceAction": "package", "outputRoot": defaultEcommerceProjectOutRoot}},
		{"id": "sync_local", "type": "script", "operation": "script", "title": "Sync Local", "extra": map[string]any{"executor": "local", "localEcommerceAction": "sync_local", "outputRoot": defaultEcommerceProjectOutRoot}},
	}, []map[string]any{
		{"source": "input", "target": "reference"},
		{"source": "reference", "target": "source", "inputOrder": 1, "inputAlias": "角色参考图", "fileSelector": "first"},
		{"source": "source", "target": "mockup", "inputOrder": 1, "inputAlias": "生成原图", "fileSelector": "first"},
		{"source": "mockup_base", "target": "mockup", "inputOrder": 2, "inputAlias": "规格图模板", "fileSelector": "first"},
		{"source": "reference", "target": "main", "inputOrder": 1, "inputAlias": "角色参考图", "fileSelector": "first"},
		{"source": "mockup", "target": "main", "inputOrder": 2, "inputAlias": "规格图", "fileSelector": "first"},
		{"source": "source", "target": "package"},
		{"source": "mockup", "target": "package"},
		{"source": "main", "target": "package"},
		{"source": "package", "target": "sync_local"},
	})
	template.Data.Metadata = map[string]any{localEcommerceKey: map[string]any{"backend": localEcommerceBackend, "channelId": "edits", "materialLibraryPath": materialRoot, "projectOutputRoot": defaultEcommerceProjectOutRoot}}
	if err := WriteTemplate(workspace, nextEnvelopeRevision(template, template.Data)); err != nil {
		t.Fatalf("WriteTemplate(local ecommerce metadata) error = %v", err)
	}
	run := writeExecutorRunWithProject(t, workspace, template.ID, profile.ID, project.ID, RunStatusPending, map[string]any{"productTitle": "羽川翼抱枕", "character": "羽川翼", "theme": "物语系列"}, nil)
	appendWaitingForExecutor(t, workspace, run.ID)

	result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID, HTTPClient: provider.Client()})
	if err != nil {
		t.Fatalf("RunExecutorOnce() error = %v", err)
	}
	if result.Processed != 1 || len(result.Runs) != 1 {
		t.Fatalf("executor result = %#v, want one local ecommerce run", result)
	}
	if got := result.Runs[0]; got.Status != RunStatusSuccess || got.Executed != 8 || got.ArtifactRefs != 7 {
		t.Fatalf("run result = %#v, want all ecommerce nodes executed with artifacts", got)
	}
	if editCalls != 3 || strings.Trim(strings.Join(intSliceStrings(uploadedCounts), ","), ",") != "1,2,2" {
		t.Fatalf("edit calls=%d uploadedCounts=%v, want source/mockup/main edits", editCalls, uploadedCounts)
	}
	for _, auth := range auths {
		if auth != "Bearer provider-secret" {
			t.Fatalf("provider auth = %q, want secretRef bearer", auth)
		}
	}
	productDir := filepath.Join(projectRoot, "outputs", "ecommerce", run.ID, safeProjectFileStem("羽川翼抱枕"))
	for _, rel := range []string{
		filepath.Join("generated", "source.png"),
		filepath.Join("待上架", safeProjectFileStem("羽川翼抱枕"), "规格图", "0001_sku_artwork.png"),
		filepath.Join("待上架", safeProjectFileStem("羽川翼抱枕"), "主图", "1_主图.png"),
		"package.json",
		"sync-local.json",
	} {
		if _, err := os.Stat(filepath.Join(productDir, rel)); err != nil {
			t.Fatalf("project output %s missing: %v", rel, err)
		}
	}
	status, err := GetRunStatus(workspace, run.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() error = %v", err)
	}
	events, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	for _, event := range events {
		switch event.Type {
		case hybridRemoteRunStarted, hybridRemoteRunSynced, hybridRemoteRunCompleted, hybridRemoteRunFailed, "remote.run.dispatched":
			t.Fatalf("local ecommerce emitted remote orchestration event %q", event.Type)
		}
	}
	refs, err := ListRunArtifacts(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunArtifacts() error = %v", err)
	}
	assertNoExecutorSecretLeak(t, status, events, refs)
	assertNoExecutorRootLeak(t, projectRoot, status, events, refs)
	second, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID, HTTPClient: provider.Client()})
	if err != nil {
		t.Fatalf("RunExecutorOnce() second error = %v", err)
	}
	if second.Processed != 0 {
		t.Fatalf("second executor result = %#v, want no duplicate local ecommerce work", second)
	}
}

func TestRunExecutorOnceExecutesLocalEcommerceSourceVariantsPackage(t *testing.T) {
	t.Setenv("OPSC_EXECUTOR_TEST_KEY", "provider-secret")
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, profileID, _ := seedExecutorWorkspace(t, root)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("create project root: %v", err)
	}
	project := writeExecutorProject(t, workspace, projectRoot, ProjectCapabilities{FSRead: true, FSWrite: true, ArtifactWrite: true})
	project.Data.Execution.AllowGlobs = append(project.Data.Execution.AllowGlobs, "runs/**")
	if err := WriteProject(workspace, nextEnvelopeRevision(project, project.Data)); err != nil {
		t.Fatalf("WriteProject(runs allow glob) error = %v", err)
	}
	materialRoot := filepath.Join(t.TempDir(), "materials")
	materialDir := filepath.Join(materialRoot, "物语系列", "羽川翼")
	if err := os.MkdirAll(materialDir, 0o755); err != nil {
		t.Fatalf("create material dir: %v", err)
	}
	imageData, err := base64.StdEncoding.DecodeString(executorTestPNGBase64)
	if err != nil {
		t.Fatalf("decode material png: %v", err)
	}
	if err := AtomicWriteFile(filepath.Join(materialDir, "reference.png"), imageData, 0o600); err != nil {
		t.Fatalf("write reference: %v", err)
	}
	if err := AtomicWriteFile(filepath.Join(materialDir, "metadata.json"), []byte(`{"work":"物语系列","character":"羽川翼"}`), 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	var editCalls int
	var auths []string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("Authorization"))
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("provider path = %s, want image edits", r.URL.Path)
		}
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		editCalls++
		if got := r.FormValue("size"); got != localEcommerceImageSize {
			t.Fatalf("image edit size = %q, want local ecommerce default", got)
		}
		if got := r.FormValue("quality"); got != localEcommerceImageQuality {
			t.Fatalf("image edit quality = %q, want local ecommerce default", got)
		}
		if got := r.FormValue("generation_config"); got != "" {
			t.Fatalf("OpenAI-compatible image edit sent generation_config: %q", got)
		}
		if len(r.MultipartForm.File["image"]) != 1 {
			t.Fatalf("uploaded images = %d, want one reference image", len(r.MultipartForm.File["image"]))
		}
		if !strings.Contains(r.FormValue("prompt"), "源图") || !strings.Contains(r.FormValue("prompt"), "1:") {
			t.Fatalf("edit prompt missing source variant context: %q", r.FormValue("prompt"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"`+executorTestPNGBase64+`"}]}`)
	}))
	defer provider.Close()

	profile := readExecutorProfile(t, workspace, profileID)
	profile.Data.Channels[0].BaseURL = provider.URL
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}

	variants := []map[string]any{
		{"nodeId": "source_japanese", "label": "日式风格", "fileName": "01-日式风格.png"},
		{"nodeId": "source_3d", "label": "3D风格", "fileName": "02-3D风格.png"},
		{"nodeId": "source_korean_semireal", "label": "韩式半写实", "fileName": "03-韩式半写实.png"},
	}
	template := writeExecutorTemplate(t, workspace, []map[string]any{
		{"id": "input", "type": "input", "operation": "input", "title": "Input"},
		{"id": "reference", "type": "material_lookup", "operation": "material_lookup", "title": "Reference", "extra": map[string]any{"assetMode": "auto", "materialLibrary": localEcommerceMaterialLibrary, "materialLibraryPath": materialRoot}},
		{"id": "source_japanese", "type": "image_edit", "operation": "image_edit", "title": "日式风格源图", "model": "image-edit-test", "prompt": "日式源图 {{uploaded_image_order}}", "size": localEcommerceImageSize, "quality": localEcommerceImageQuality},
		{"id": "source_3d", "type": "image_edit", "operation": "image_edit", "title": "3D风格源图", "model": "image-edit-test", "prompt": "3D源图 {{uploaded_image_order}}", "size": localEcommerceImageSize, "quality": localEcommerceImageQuality},
		{"id": "source_korean_semireal", "type": "image_edit", "operation": "image_edit", "title": "韩式半写实源图", "model": "image-edit-test", "prompt": "韩式源图 {{uploaded_image_order}}", "size": localEcommerceImageSize, "quality": localEcommerceImageQuality},
		{"id": "package_sources", "type": "script", "operation": "script", "title": "结果文件夹", "extra": map[string]any{"executor": "local", "localEcommerceAction": "package", "localEcommercePackageMode": "source_variants", "sourceVariantRoot": "runs", "sourceVariants": variants}},
	}, []map[string]any{
		{"source": "input", "target": "reference"},
		{"source": "reference", "target": "source_japanese", "inputOrder": 1, "inputAlias": "标准参考图", "fileSelector": "first"},
		{"source": "reference", "target": "source_3d", "inputOrder": 1, "inputAlias": "标准参考图", "fileSelector": "first"},
		{"source": "reference", "target": "source_korean_semireal", "inputOrder": 1, "inputAlias": "标准参考图", "fileSelector": "first"},
		{"source": "source_japanese", "target": "package_sources"},
		{"source": "source_3d", "target": "package_sources"},
		{"source": "source_korean_semireal", "target": "package_sources"},
	})
	template.Data.Metadata = map[string]any{localEcommerceKey: map[string]any{"backend": localEcommerceBackend, "channelId": "openai", "materialLibraryPath": materialRoot, "projectOutputRoot": "runs"}}
	if err := WriteTemplate(workspace, nextEnvelopeRevision(template, template.Data)); err != nil {
		t.Fatalf("WriteTemplate(local ecommerce metadata) error = %v", err)
	}
	run := writeExecutorRunWithProject(t, workspace, template.ID, profile.ID, project.ID, RunStatusPending, map[string]any{"productTitle": "羽川翼抱枕", "character": "羽川翼", "theme": "《物语系列》"}, nil)
	appendWaitingForExecutor(t, workspace, run.ID)

	result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID, HTTPClient: provider.Client()})
	if err != nil {
		t.Fatalf("RunExecutorOnce() error = %v", err)
	}
	if result.Processed != 1 || len(result.Runs) != 1 {
		t.Fatalf("executor result = %#v, want one processed source variants run", result)
	}
	if got := result.Runs[0]; got.Status != RunStatusSuccess || got.Executed != 6 || got.ArtifactRefs != 5 {
		t.Fatalf("executor result = %#v, want source variants success", result)
	}
	if editCalls != 3 {
		t.Fatalf("editCalls = %d, want three source variants", editCalls)
	}
	for _, auth := range auths {
		if auth != "Bearer provider-secret" {
			t.Fatalf("provider auth = %q, want secretRef bearer", auth)
		}
	}
	packageRoot := filepath.Join(projectRoot, "runs", localEcommerceRunTimestampFolder(run), "generated", "物语系列 - 羽川翼")
	for _, rel := range []string{"01-日式风格.png", "02-3D风格.png", "03-韩式半写实.png", "package.json"} {
		if _, err := os.Stat(filepath.Join(packageRoot, rel)); err != nil {
			t.Fatalf("source variant output %s missing: %v", rel, err)
		}
	}
	states, err := ListRunNodeStates(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunNodeStates() error = %v", err)
	}
	stateByID := executorTestStateMap(states)
	outputs := anySliceFromMap(stateByID["package_sources"].Data.Output, "projectOutputs")
	if len(outputs) != 3 || stringFromMap(stateByID["package_sources"].Data.Output, "packageRoot") != path.Join("runs", localEcommerceRunTimestampFolder(run), "generated", "物语系列 - 羽川翼") {
		t.Fatalf("package output = %#v, want three source variants in theme-character folder", stateByID["package_sources"].Data.Output)
	}
	refs, err := ListRunArtifacts(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunArtifacts() error = %v", err)
	}
	events, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	for _, event := range events {
		switch event.Type {
		case hybridRemoteRunStarted, hybridRemoteRunSynced, hybridRemoteRunCompleted, hybridRemoteRunFailed, "remote.run.dispatched":
			t.Fatalf("local source variants emitted remote orchestration event %q", event.Type)
		}
	}
	status, err := GetRunStatus(workspace, run.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() error = %v", err)
	}
	assertNoExecutorSecretLeak(t, status, events, refs)
	assertNoExecutorRootLeak(t, projectRoot, status, events, refs)
}

func TestRunExecutorOnceExecutesLocalEcommerceMultiProductTemplate(t *testing.T) {
	t.Setenv("OPSC_EXECUTOR_TEST_KEY", "provider-secret")
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, profileID, _ := seedExecutorWorkspace(t, root)
	materialRoot := filepath.Join(t.TempDir(), "materials")
	imageData, err := base64.StdEncoding.DecodeString(executorTestPNGBase64)
	if err != nil {
		t.Fatalf("decode material png: %v", err)
	}
	for _, item := range []struct {
		work      string
		character string
	}{
		{work: "物语系列", character: "羽川翼"},
		{work: "原神", character: "八重神子"},
	} {
		materialDir := filepath.Join(materialRoot, item.work, item.character)
		if err := os.MkdirAll(materialDir, 0o755); err != nil {
			t.Fatalf("create material dir: %v", err)
		}
		if err := AtomicWriteFile(filepath.Join(materialDir, "reference.png"), imageData, 0o600); err != nil {
			t.Fatalf("write reference: %v", err)
		}
		metadata := `{"work":` + strconv.Quote(item.work) + `,"character":` + strconv.Quote(item.character) + `}`
		if err := AtomicWriteFile(filepath.Join(materialDir, "metadata.json"), []byte(metadata), 0o600); err != nil {
			t.Fatalf("write metadata: %v", err)
		}
	}

	var editCalls int
	var auths []string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("Authorization"))
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("provider path = %s, want image edits", r.URL.Path)
		}
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		editCalls++
		if len(r.MultipartForm.File["image"]) != 1 {
			t.Fatalf("uploaded images = %d, want one reference image", len(r.MultipartForm.File["image"]))
		}
		if !strings.Contains(r.FormValue("prompt"), "抱枕") || !strings.Contains(r.FormValue("prompt"), "1:") {
			t.Fatalf("edit prompt missing product or image order: %q", r.FormValue("prompt"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"`+executorTestPNGBase64+`"}]}`)
	}))
	defer provider.Close()

	profile := readExecutorProfile(t, workspace, profileID)
	profile.Data.Channels[0].BaseURL = provider.URL
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}

	template := writeExecutorTemplate(t, workspace, []map[string]any{
		{"id": "input", "type": "input", "operation": "input", "title": "Input"},
		{"id": "reference", "type": "material_lookup", "operation": "material_lookup", "title": "Reference", "extra": map[string]any{"assetMode": "auto", "materialLibrary": localEcommerceMaterialLibrary, "materialLibraryPath": materialRoot}},
		{"id": "source", "type": "image_edit", "operation": "image_edit", "title": "Source", "model": "image-edit-test", "prompt": "{{productTitle}} source {{uploaded_image_order}}"},
	}, []map[string]any{
		{"source": "input", "target": "reference"},
		{"source": "reference", "target": "source", "inputOrder": 1, "inputAlias": "角色参考图", "fileSelector": "first"},
	})
	template.Data.Metadata = map[string]any{localEcommerceKey: map[string]any{"backend": localEcommerceBackend, "channelId": "openai", "materialLibraryPath": materialRoot}}
	template.Data.Settings = map[string]any{"productConcurrency": 1}
	if err := WriteTemplate(workspace, nextEnvelopeRevision(template, template.Data)); err != nil {
		t.Fatalf("WriteTemplate(local ecommerce metadata) error = %v", err)
	}
	run := writeExecutorRun(t, workspace, template.ID, profile.ID, RunStatusPending, map[string]any{
		"inputs": []map[string]any{
			{"productTitle": "羽川翼抱枕", "character": "羽川翼", "theme": "物语系列"},
			{"productTitle": "八重神子抱枕", "character": "八重神子", "theme": "原神"},
		},
	}, nil)
	appendWaitingForExecutor(t, workspace, run.ID)

	result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID, HTTPClient: provider.Client()})
	if err != nil {
		t.Fatalf("RunExecutorOnce() error = %v", err)
	}
	if result.Processed != 1 || len(result.Runs) != 1 {
		t.Fatalf("executor result = %#v, want one multi-product run", result)
	}
	if got := result.Runs[0]; got.Status != RunStatusSuccess || got.Executed != 6 || got.ArtifactRefs != 4 {
		t.Fatalf("run result = %#v, want two product executions with four artifacts", got)
	}
	if editCalls != 2 {
		t.Fatalf("editCalls = %d, want two product image edits", editCalls)
	}
	for _, auth := range auths {
		if auth != "Bearer provider-secret" {
			t.Fatalf("provider auth = %q, want secretRef bearer", auth)
		}
	}
	status, err := GetRunStatus(workspace, run.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() error = %v", err)
	}
	if status.Run.Status != RunStatusSuccess || status.Run.ArtifactCount != 4 {
		t.Fatalf("run status = %#v, want success with four artifacts", status.Run)
	}
	states, err := ListRunNodeStates(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunNodeStates() error = %v", err)
	}
	stateByID := executorTestStateMap(states)
	for _, nodeID := range []string{"input", "reference", "source"} {
		state := stateByID[nodeID]
		if state.Data.Status != RunStatusSuccess || positiveInt(state.Data.Output["success"]) != 2 || positiveInt(state.Data.Output["total"]) != 2 {
			t.Fatalf("aggregate state %s = %#v, want two successful products", nodeID, state.Data)
		}
	}
	for _, scoped := range []string{
		productScopedNodeID("product_001", "source"),
		productScopedNodeID("product_002", "source"),
	} {
		if stateByID[scoped].Data.Status != RunStatusSuccess {
			t.Fatalf("scoped state %s = %#v, want success", scoped, stateByID[scoped].Data)
		}
	}
	refs, err := ListRunArtifacts(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunArtifacts() error = %v", err)
	}
	seenProducts := map[string]bool{}
	for _, ref := range refs {
		productKey := stringFromMap(ref.Ref.Metadata, "productKey")
		if productKey == "" || stringFromMap(ref.Ref.Metadata, "templateNodeId") == "" {
			t.Fatalf("ref metadata = %#v, want productKey and templateNodeId", ref.Ref.Metadata)
		}
		seenProducts[productKey] = true
	}
	if !seenProducts["product_001"] || !seenProducts["product_002"] {
		t.Fatalf("ref products = %#v, want refs for both products", seenProducts)
	}
	events, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	for _, want := range []string{"executor.product.started", "executor.product.completed", executorEventRunCompleted} {
		if !runEventTypesContain(events, want) {
			t.Fatalf("events missing %s: %#v", want, events)
		}
	}
	for _, event := range events {
		switch event.Type {
		case hybridRemoteRunStarted, hybridRemoteRunSynced, hybridRemoteRunCompleted, hybridRemoteRunFailed, "remote.run.dispatched":
			t.Fatalf("local multi-product ecommerce emitted remote orchestration event %q", event.Type)
		}
	}
	assertNoExecutorSecretLeak(t, status, events, refs)
}

func TestRunExecutorOnceStopsRetryingLocalEcommerceConfigErrors(t *testing.T) {
	t.Setenv("OPSC_EXECUTOR_TEST_KEY", "provider-secret")
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, profileID, assetID := seedExecutorWorkspace(t, root)
	template := writeExecutorTemplate(t, workspace, []map[string]any{
		{"id": "input", "type": "input", "operation": "input", "title": "Input"},
		{"id": "reference", "type": "material_lookup", "operation": "material_lookup", "title": "Reference", "extra": map[string]any{"assetMode": "fixed", "assetId": assetID}},
		{"id": "source", "type": "image_edit", "operation": "image_edit", "title": "Source", "model": "image-edit-test", "prompt": "{{productTitle}} source {{uploaded_image_order}}", "retry": map[string]any{"enabled": true, "retryCount": 0, "intervalSeconds": 0}},
	}, []map[string]any{
		{"source": "reference", "target": "source", "inputOrder": 1, "inputAlias": "角色参考图", "fileSelector": "first"},
	})
	template.Data.Metadata = map[string]any{localEcommerceKey: map[string]any{"backend": localEcommerceBackend, "channelId": "missing-channel"}}
	if err := WriteTemplate(workspace, nextEnvelopeRevision(template, template.Data)); err != nil {
		t.Fatalf("WriteTemplate(local ecommerce metadata) error = %v", err)
	}
	run := writeExecutorRun(t, workspace, template.ID, profileID, RunStatusPending, map[string]any{"productTitle": "配置错误抱枕"}, nil)
	appendWaitingForExecutor(t, workspace, run.ID)

	result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID})
	if err != nil {
		t.Fatalf("RunExecutorOnce() error = %v", err)
	}
	if result.Processed != 1 || len(result.Runs) != 1 {
		t.Fatalf("executor result = %#v, want one failed run", result)
	}
	if got := result.Runs[0]; got.Status != RunStatusError || !strings.Contains(got.Error, "profile has no usable ai channel") {
		t.Fatalf("run result = %#v, want nonretryable profile channel error", got)
	}
	events, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	if runEventTypesContain(events, executorEventNodeRetrying) {
		t.Fatalf("events contain retrying marker for config error: %#v", events)
	}
}

func TestRunExecutorOnceDoesNotRetryProviderClientErrorForever(t *testing.T) {
	t.Setenv("OPSC_EXECUTOR_TEST_KEY", "provider-secret")
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, profileID, assetID := seedExecutorWorkspace(t, root)
	var calls int
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"message":"endpoint not found"}}`)
	}))
	defer provider.Close()
	profile := readExecutorProfile(t, workspace, profileID)
	profile.Data.Channels = []ProfileChannel{{
		ID:        "edits",
		Protocol:  "openai-compatible",
		BaseURL:   provider.URL,
		Enabled:   true,
		SecretRef: &SecretRef{Type: SecretRefTypeEnv, Name: "OPSC_EXECUTOR_TEST_KEY"},
	}}
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}
	template := writeExecutorTemplate(t, workspace, []map[string]any{
		{"id": "reference", "type": "material_lookup", "operation": "material_lookup", "title": "Reference", "extra": map[string]any{"assetMode": "fixed", "assetId": assetID}},
		{"id": "source", "type": "image_edit", "operation": "image_edit", "title": "Source", "model": "image-edit-test", "prompt": "source {{uploaded_image_order}}", "retry": map[string]any{"enabled": true, "retryCount": 0, "intervalSeconds": 0}},
	}, []map[string]any{
		{"source": "reference", "target": "source", "inputOrder": 1, "inputAlias": "角色参考图", "fileSelector": "first"},
	})
	template.Data.Metadata = map[string]any{localEcommerceKey: map[string]any{"backend": localEcommerceBackend, "channelId": "edits"}}
	if err := WriteTemplate(workspace, nextEnvelopeRevision(template, template.Data)); err != nil {
		t.Fatalf("WriteTemplate(local ecommerce metadata) error = %v", err)
	}
	run := writeExecutorRun(t, workspace, template.ID, profileID, RunStatusPending, map[string]any{"productTitle": "错误重试测试"}, nil)
	appendWaitingForExecutor(t, workspace, run.ID)

	result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID, HTTPClient: provider.Client()})
	if err != nil {
		t.Fatalf("RunExecutorOnce() error = %v", err)
	}
	if result.Processed != 1 || len(result.Runs) != 1 || result.Runs[0].Status != RunStatusError {
		t.Fatalf("executor result = %#v, want failed run", result)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want one nonretryable 4xx call", calls)
	}
	events, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	if runEventTypesContain(events, executorEventNodeRetrying) {
		t.Fatalf("events contain retrying marker for provider 4xx: %#v", events)
	}
}

func TestRunExecutorOnceTimesOutSlowOpenAIImageRequest(t *testing.T) {
	t.Setenv("OPSC_EXECUTOR_TEST_KEY", "provider-secret")
	oldTimeout := executorAIImageRequestTimeout
	executorAIImageRequestTimeout = 20 * time.Millisecond
	defer func() { executorAIImageRequestTimeout = oldTimeout }()

	root := filepath.Join(t.TempDir(), "workspace")
	workspace, profileID, assetID := seedExecutorWorkspace(t, root)
	var calls int
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"b64_json":"`+executorTestPNGBase64+`"}]}`)
	}))
	defer provider.Close()
	profile := readExecutorProfile(t, workspace, profileID)
	profile.Data.Channels[0].BaseURL = provider.URL
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}
	template := writeExecutorTemplate(t, workspace, []map[string]any{
		{"id": "reference", "type": "material_lookup", "operation": "material_lookup", "title": "Reference", "extra": map[string]any{"assetMode": "fixed", "assetId": assetID}},
		{"id": "source", "type": "image_edit", "operation": "image_edit", "title": "Source", "model": "image-edit-test", "prompt": "source {{uploaded_image_order}}", "retry": map[string]any{"enabled": false}},
	}, []map[string]any{
		{"source": "reference", "target": "source", "inputOrder": 1, "inputAlias": "角色参考图", "fileSelector": "first"},
	})
	run := writeExecutorRun(t, workspace, template.ID, profileID, RunStatusPending, map[string]any{"productTitle": "超时测试"}, nil)
	appendWaitingForExecutor(t, workspace, run.ID)

	startedAt := time.Now()
	result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID, HTTPClient: provider.Client()})
	elapsed := time.Since(startedAt)
	if err != nil {
		t.Fatalf("RunExecutorOnce() error = %v", err)
	}
	if result.Processed != 1 || len(result.Runs) != 1 || result.Runs[0].Status != RunStatusError {
		t.Fatalf("executor result = %#v, want timed-out failed run", result)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want one call with retry disabled", calls)
	}
	if elapsed > time.Second {
		t.Fatalf("slow image request elapsed = %s, want executor-side timeout", elapsed)
	}
	events, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	if data, _ := json.Marshal(events); strings.Contains(string(data), provider.URL) {
		t.Fatalf("timeout events leaked provider URL: %s", data)
	}
}

func TestLocalEcommerceMaterialLookupUsesEnvLibraryFallback(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	materialRoot := filepath.Join(t.TempDir(), "anime_ip")
	t.Setenv(localEcommerceMaterialLibraryEnv, materialRoot)
	workspace, _, _ := seedExecutorWorkspace(t, root)
	template := writeExecutorTemplate(t, workspace, []map[string]any{
		{"id": "reference", "type": "material_lookup", "operation": "material_lookup", "title": "Reference", "extra": map[string]any{"assetMode": "auto", "materialLibrary": localEcommerceMaterialLibrary}},
	}, nil)
	template.Data.Metadata = map[string]any{localEcommerceKey: map[string]any{"backend": localEcommerceBackend, "projectOutputRoot": defaultEcommerceProjectOutRoot}}
	node := executorNode{ID: "reference", Operation: "material_lookup", Extra: map[string]any{"assetMode": "auto", "materialLibrary": localEcommerceMaterialLibrary}}
	got := executorLocalMaterialLibraryPath(executorContext{workspace: workspace, template: template}, node)
	if got != materialRoot {
		t.Fatalf("material library path = %q, want env fallback path", got)
	}
}

func TestRunExecutorOnceResumesRunningRunWithoutReexecutingSuccessNode(t *testing.T) {
	t.Setenv("OPSC_EXECUTOR_TEST_KEY", "provider-secret")
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, profileID, _ := seedExecutorWorkspace(t, root)

	var providerPaths []string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerPaths = append(providerPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/images/generations":
			_, _ = io.WriteString(w, `{"data":[{"b64_json":"`+executorTestPNGBase64+`"}]}`)
		default:
			t.Fatalf("unexpected provider path on resume: %s", r.URL.Path)
		}
	}))
	defer provider.Close()

	profile := readExecutorProfile(t, workspace, profileID)
	profile.Data.Channels[0].BaseURL = provider.URL
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}

	template := writeExecutorTemplate(t, workspace, []map[string]any{
		{"id": "copy", "type": "text_generation", "operation": "text_generation", "title": "Copy", "model": "gpt-test", "prompt": "Write {{input.productTitle}}"},
		{"id": "image", "type": "image_generation", "operation": "image_generation", "title": "Image", "model": "image-test", "prompt": "Render {{node.copy.text}}"},
	}, []map[string]any{{"from": "copy", "to": "image"}})
	run := writeExecutorRun(t, workspace, template.ID, profile.ID, RunStatusRunning, map[string]any{"productTitle": "Mug"}, map[string]any{"executor": executorID})
	copyState, err := NewRunNodeState("copy", RunNodeStateData{
		NodeID: "copy",
		Status: RunStatusSuccess,
		Output: map[string]any{"text": "already done", "artifactIds": []any{"art_existing"}, "artifactId": "art_existing"},
	})
	if err != nil {
		t.Fatalf("NewRunNodeState() error = %v", err)
	}
	if err := WriteRunNodeState(workspace, run.ID, copyState); err != nil {
		t.Fatalf("WriteRunNodeState() error = %v", err)
	}

	result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID})
	if err != nil {
		t.Fatalf("RunExecutorOnce() error = %v", err)
	}
	if result.Processed != 1 || len(result.Runs) != 1 {
		t.Fatalf("executor result = %#v, want one resumed run", result)
	}
	if got := result.Runs[0]; got.Status != RunStatusSuccess || got.Executed != 1 || got.Skipped != 1 || got.ArtifactRefs != 1 {
		t.Fatalf("run result = %#v, want resumed image-only execution", got)
	}
	if strings.Join(providerPaths, ",") != "/v1/images/generations" {
		t.Fatalf("provider paths = %#v, want only image generation", providerPaths)
	}
	states, err := ListRunNodeStates(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunNodeStates() error = %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("node states len = %d, want 2", len(states))
	}
	for _, state := range states {
		if state.Data.Status != RunStatusSuccess {
			t.Fatalf("node %s status = %s, want success", state.ID, state.Data.Status)
		}
	}
	events, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	if !runEventTypesContain(events, executorEventResumed) {
		t.Fatalf("events missing resume marker: %#v", events)
	}
}

func TestRunExecutorOnceExecutesProjectScriptConditionRetryAndOutputMapping(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, _, _ := seedExecutorWorkspace(t, root)
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, "scripts"), 0o755); err != nil {
		t.Fatalf("create scripts dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, "outputs"), 0o755); err != nil {
		t.Fatalf("create outputs dir: %v", err)
	}
	scriptPath := filepath.Join(projectRoot, "scripts", "generate.sh")
	script := `#!/bin/sh
set -eu
marker="outputs/.retry-marker"
if [ ! -f "$marker" ]; then
  touch "$marker"
  echo "transient failure" >&2
  exit 7
fi
printf '{"decision":"pass","message":"project:%s"}\n' "$1"
`
	if err := AtomicWriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write script: %v", err)
	}
	project := writeExecutorProject(t, workspace, projectRoot, ProjectCapabilities{FSRead: true, FSWrite: true, ProcessExec: true, ArtifactWrite: true})
	retryEnabled := true
	template := writeExecutorTemplate(t, workspace, []map[string]any{
		{"id": "input", "type": "input", "operation": "input", "title": "Input"},
		{"id": "gate", "type": "text", "operation": "condition", "title": "Gate", "extra": map[string]any{
			"conditions":    []any{map[string]any{"path": "input.shouldRun", "operator": "eq", "value": true, "output": "run"}},
			"defaultOutput": "skip",
		}},
		{"id": "script", "type": "text", "operation": "script", "title": "Project Script", "retry": map[string]any{"enabled": retryEnabled, "retryCount": 2, "intervalSeconds": 0}, "extra": map[string]any{
			"executor":   "project",
			"scriptPath": "scripts/generate.sh",
			"args":       []any{"{{input.productTitle}}"},
			"outputPath": "outputs/script-output.json",
		}},
	}, []map[string]any{
		{"source": "input", "target": "gate"},
		{"source": "gate", "target": "script", "fromHandle": "run"},
	})
	run := writeExecutorRunWithProject(t, workspace, template.ID, "", project.ID, RunStatusPending, map[string]any{"productTitle": "Mug", "shouldRun": true}, nil)
	appendWaitingForExecutor(t, workspace, run.ID)

	result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID})
	if err != nil {
		t.Fatalf("RunExecutorOnce() error = %v", err)
	}
	if result.Processed != 1 || len(result.Runs) != 1 {
		t.Fatalf("executor result = %#v, want one project run", result)
	}
	if got := result.Runs[0]; got.Status != RunStatusSuccess || got.Executed != 3 || got.Skipped != 0 || got.ArtifactRefs != 1 {
		t.Fatalf("run result = %#v, want project script success with one artifact", got)
	}
	projectOutput, err := os.ReadFile(filepath.Join(projectRoot, "outputs", "script-output.json"))
	if err != nil {
		t.Fatalf("read project output: %v", err)
	}
	if !strings.Contains(string(projectOutput), `"project:Mug"`) {
		t.Fatalf("project output = %s, want rendered script stdout", projectOutput)
	}
	events, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	if !runEventTypesContain(events, executorEventNodeRetrying) {
		t.Fatalf("events missing retry marker: %#v", events)
	}
	states, err := ListRunNodeStates(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunNodeStates() error = %v", err)
	}
	stateByID := executorTestStateMap(states)
	scriptState := stateByID["script"]
	if scriptState.Data.Status != RunStatusSuccess || len(scriptState.Data.Output["projectOutputs"].([]any)) != 1 {
		t.Fatalf("script state = %#v, want success with projectOutputs", scriptState.Data)
	}
	if scriptState.Data.Output["decision"] != "pass" {
		t.Fatalf("script output decision = %#v, want parsed JSON decision", scriptState.Data.Output["decision"])
	}
	status, err := GetRunStatus(workspace, run.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() error = %v", err)
	}
	refs, err := ListRunArtifacts(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunArtifacts() error = %v", err)
	}
	assertNoExecutorRootLeak(t, projectRoot, status, events, states, refs)

	second, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID})
	if err != nil {
		t.Fatalf("RunExecutorOnce() second error = %v", err)
	}
	if second.Processed != 0 {
		t.Fatalf("second executor result = %#v, want no duplicate project work", second)
	}
	if err := os.Remove(filepath.Join(root, IndexFileName)); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	if _, err := RebuildIndex(context.Background(), workspace, SQLiteIndexRebuilder{}, ScanOptions{}); err != nil {
		t.Fatalf("RebuildIndex() error = %v", err)
	}
	rebuiltStatus, err := GetRunStatus(workspace, run.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() after rebuild error = %v", err)
	}
	if rebuiltStatus.Run.Status != RunStatusSuccess || rebuiltStatus.Run.ArtifactCount != 1 || len(rebuiltStatus.Nodes) != 3 {
		t.Fatalf("rebuilt run status = %#v, want project executor data indexed", rebuiltStatus)
	}
}

func TestHybridEcommerceImportAndExecutorSyncsRemoteRun(t *testing.T) {
	t.Setenv("OPSC_HYBRID_TEST_TOKEN", "remote-secret")
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, profileID, _ := seedExecutorWorkspace(t, root)

	var remoteRunID string
	var startCalls int
	var auths []string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/workflows/pdd/templates/remote_tpl":
			_, _ = io.WriteString(w, `{"code":0,"data":{"id":"remote_tpl","workflowType":"pdd","title":"Confirmed Ecommerce","description":"VPS template","spec":{"version":1,"nodes":[{"id":"stage_generate","operation":"image_generation","title":"Generate"}],"edges":[],"settings":{"productConcurrency":1,"maxRetries":0}},"updatedAt":"2026-01-02T03:04:05Z"},"msg":"ok"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/workflows/pdd/templates/remote_tpl/runs":
			startCalls++
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode start payload: %v", err)
			}
			remoteRunID = payload["runId"].(string)
			if !strings.HasPrefix(remoteRunID, "hybrid_run_") {
				t.Fatalf("remote run id = %q, want hybrid local id", remoteRunID)
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"runId":"`+remoteRunID+`","runDir":"/srv/ops/private/run"},"msg":"ok"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/overview"):
			if remoteRunID == "" || !strings.Contains(r.URL.Path, remoteRunID) {
				t.Fatalf("overview path = %s before remote run id", r.URL.Path)
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"run":{"runId":"`+remoteRunID+`","status":"success","completed":true,"productTotal":1,"completedProducts":1},"stages":[{"id":"stage_generate","title":"Generate","status":"success","total":1,"success":1}],"products":[{"key":"prod_1","product":"Mug","status":"success"}]},"msg":"ok"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/product-detail"):
			if r.URL.Query().Get("key") != "prod_1" {
				t.Fatalf("product-detail key = %q", r.URL.Query().Get("key"))
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"runId":"`+remoteRunID+`","product":{"key":"prod_1","product":"Mug","status":"success"},"nodes":[{"id":"stage_generate","type":"image_generation","title":"Generate","status":"success","artifacts":[{"id":"a1","title":"Preview","path":"logs/custom_workflow/products/prod_1/nodes/stage_generate/output.png","kind":"image","mimeType":"image/png"}]}]},"msg":"ok"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/file"):
			if !strings.Contains(r.URL.Query().Get("path"), "output.png") {
				t.Fatalf("file path = %q", r.URL.Query().Get("path"))
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()

	profile := readExecutorProfile(t, workspace, profileID)
	profile.Data.Channels[0].Protocol = "ops-canvas-vps"
	profile.Data.Channels[0].BaseURL = remote.URL
	profile.Data.Channels[0].SecretRef = &SecretRef{Type: SecretRefTypeEnv, Name: "OPSC_HYBRID_TEST_TOKEN"}
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}

	imported, err := ImportHybridEcommerceTemplate(context.Background(), HybridEcommerceImportOptions{
		WorkspacePath:    root,
		RemoteTemplateID: "remote_tpl",
		ProfileID:        profileID,
		ChannelID:        "openai",
		HTTPClient:       remote.Client(),
	})
	if err != nil {
		t.Fatalf("ImportHybridEcommerceTemplate() error = %v", err)
	}
	if !imported.Created || imported.Template.Data.Title != "Confirmed Ecommerce" {
		t.Fatalf("imported = %#v, want created ecommerce template", imported)
	}
	hybridMetadata, ok := asMapStringAny(imported.Template.Data.Metadata[hybridEcommerceKey])
	if !ok || stringFromMap(hybridMetadata, "sourceFingerprint") == "" || stringFromMap(hybridMetadata, "importedAt") == "" {
		t.Fatalf("hybrid metadata = %#v, want source fingerprint and importedAt", imported.Template.Data.Metadata[hybridEcommerceKey])
	}
	draft, err := CreateHybridEcommerceRun(context.Background(), HybridEcommerceRunOptions{
		WorkspacePath: root,
		TemplateID:    imported.Template.ID,
		ProfileID:     profileID,
		Input: map[string]any{
			"inputs": []map[string]any{{"productTitle": "Mug"}},
		},
	})
	if err != nil {
		t.Fatalf("CreateHybridEcommerceRun() error = %v", err)
	}
	run := draft.Run
	if run.Data.Status != RunStatusPending || run.Data.Input["productConcurrency"] == nil {
		t.Fatalf("draft run = %#v, want pending with template defaults", run.Data)
	}
	draftEvents, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents(draft) error = %v", err)
	}
	if !runEventTypesContain(draftEvents, "run.waiting_for_executor") {
		t.Fatalf("draft events missing waiting_for_executor: %#v", draftEvents)
	}

	result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID, HTTPClient: remote.Client()})
	if err != nil {
		t.Fatalf("RunExecutorOnce() error = %v", err)
	}
	if result.Processed != 1 || len(result.Runs) != 1 {
		t.Fatalf("executor result = %#v, want one hybrid run", result)
	}
	if got := result.Runs[0]; got.Status != RunStatusSuccess || got.Executed != 1 || got.ArtifactRefs != 1 {
		t.Fatalf("hybrid run result = %#v, want success with synced artifact", got)
	}
	if startCalls != 1 {
		t.Fatalf("startCalls = %d, want one remote run", startCalls)
	}
	for _, auth := range auths {
		if auth != "Bearer remote-secret" {
			t.Fatalf("remote auth = %q, want bearer from secretRef", auth)
		}
	}
	status, err := GetRunStatus(workspace, run.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() error = %v", err)
	}
	events, err := ReadRunEvents(workspace, run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	for _, want := range []string{hybridRemoteRunStarted, hybridRemoteRunSynced, hybridRemoteRunCompleted, executorEventRunCompleted} {
		if !runEventTypesContain(events, want) {
			t.Fatalf("events missing %s: %#v", want, events)
		}
	}
	refs, err := ListRunArtifacts(workspace, run.ID)
	if err != nil {
		t.Fatalf("ListRunArtifacts() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs = %#v, want one canonical remote artifact ref", refs)
	}
	artifact, err := ReadArtifact(workspace, refs[0].Artifact.ID)
	if err != nil {
		t.Fatalf("ReadArtifact() error = %v", err)
	}
	if artifact.Data.Source["remotePath"] == "" {
		t.Fatalf("artifact source = %#v, want remotePath", artifact.Data.Source)
	}
	second, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID, HTTPClient: remote.Client()})
	if err != nil {
		t.Fatalf("RunExecutorOnce() second error = %v", err)
	}
	if second.Processed != 0 {
		t.Fatalf("second executor result = %#v, want no duplicate remote sync for success run", second)
	}
	encoded, err := json.Marshal(map[string]any{
		"template": imported.Template,
		"status":   status,
		"events":   events,
		"refs":     refs,
	})
	if err != nil {
		t.Fatalf("marshal observed data: %v", err)
	}
	for _, secret := range []string{"remote-secret", "/srv/ops/private/run"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("observed workspace data leaked %q:\n%s", secret, encoded)
		}
	}
}

func TestRunExecutorWatchSyncsHybridRunUntilTerminal(t *testing.T) {
	t.Setenv("OPSC_HYBRID_TEST_TOKEN", "remote-secret")
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, profileID, _ := seedExecutorWorkspace(t, root)

	var remoteRunID string
	var startCalls atomic.Int32
	var overviewCalls atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/workflows/pdd/templates/remote_tpl":
			_, _ = io.WriteString(w, `{"code":0,"data":{"id":"remote_tpl","workflowType":"pdd","title":"Watch Ecommerce","spec":{"version":1,"nodes":[{"id":"stage_generate","operation":"image_generation","title":"Generate"}],"edges":[],"settings":{"productConcurrency":1,"maxRetries":0}}},"msg":"ok"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/workflows/pdd/templates/remote_tpl/runs":
			startCalls.Add(1)
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode start payload: %v", err)
			}
			remoteRunID = payload["runId"].(string)
			_, _ = io.WriteString(w, `{"code":0,"data":{"runId":"`+remoteRunID+`"},"msg":"ok"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/overview"):
			call := overviewCalls.Add(1)
			if call == 1 {
				_, _ = io.WriteString(w, `{"code":0,"data":{"run":{"runId":"`+remoteRunID+`","status":"running","completed":false,"productTotal":1,"runningProducts":1},"stages":[{"id":"stage_generate","title":"Generate","status":"running","total":1,"running":1}],"products":[{"key":"prod_1","product":"Mug","status":"running"}]},"msg":"ok"}`)
				return
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"run":{"runId":"`+remoteRunID+`","status":"success","completed":true,"productTotal":1,"completedProducts":1},"stages":[{"id":"stage_generate","title":"Generate","status":"success","total":1,"success":1}],"products":[{"key":"prod_1","product":"Mug","status":"success"}]},"msg":"ok"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/product-detail"):
			_, _ = io.WriteString(w, `{"code":0,"data":{"runId":"`+remoteRunID+`","product":{"key":"prod_1","product":"Mug","status":"success"},"nodes":[{"id":"stage_generate","type":"image_generation","title":"Generate","status":"success","artifacts":[{"id":"a1","title":"Preview","path":"logs/custom_workflow/products/prod_1/nodes/stage_generate/output.png","kind":"image","mimeType":"image/png"}]}]},"msg":"ok"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/file"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()

	profile := readExecutorProfile(t, workspace, profileID)
	profile.Data.Channels[0].Protocol = "ops-canvas-vps"
	profile.Data.Channels[0].BaseURL = remote.URL
	profile.Data.Channels[0].SecretRef = &SecretRef{Type: SecretRefTypeEnv, Name: "OPSC_HYBRID_TEST_TOKEN"}
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}
	imported, err := ImportHybridEcommerceTemplate(context.Background(), HybridEcommerceImportOptions{
		WorkspacePath:    root,
		RemoteTemplateID: "remote_tpl",
		ProfileID:        profileID,
		ChannelID:        "openai",
		HTTPClient:       remote.Client(),
	})
	if err != nil {
		t.Fatalf("ImportHybridEcommerceTemplate() error = %v", err)
	}
	draft, err := CreateHybridEcommerceRun(context.Background(), HybridEcommerceRunOptions{
		WorkspacePath: root,
		TemplateID:    imported.Template.ID,
		ProfileID:     profileID,
		Input:         map[string]any{"inputs": []map[string]any{{"productTitle": "Mug"}}},
	})
	if err != nil {
		t.Fatalf("CreateHybridEcommerceRun() error = %v", err)
	}

	result, err := RunExecutorWatch(context.Background(), ExecutorWatchOptions{
		ExecutorOptions: ExecutorOptions{WorkspacePath: root, RunID: draft.Run.ID, HTTPClient: remote.Client()},
		PollInterval:    time.Millisecond,
		MaxIterations:   3,
	})
	if err != nil {
		t.Fatalf("RunExecutorWatch() error = %v", err)
	}
	if startCalls.Load() != 1 || overviewCalls.Load() < 2 {
		t.Fatalf("remote calls start=%d overview=%d, want one start and repeated sync", startCalls.Load(), overviewCalls.Load())
	}
	if result.Processed < 2 {
		t.Fatalf("watch result = %#v, want at least dispatch and sync iterations", result)
	}
	status, err := GetRunStatus(workspace, draft.Run.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() error = %v", err)
	}
	if status.Run.Status != RunStatusSuccess || status.Run.ArtifactCount != 1 {
		t.Fatalf("run status = %#v, want success with one artifact", status.Run)
	}
	if len(status.Nodes) != 1 || status.Nodes[0].Output["total"] == nil || status.Nodes[0].Output["success"] == nil {
		t.Fatalf("node status summary = %#v, want indexed remote progress output", status.Nodes)
	}
	events, err := ReadRunEvents(workspace, draft.Run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	if countRunEventTypes(events, hybridRemoteRunSynced) < 2 {
		t.Fatalf("events = %#v, want synced events for running and terminal progress", events)
	}
	if countRunEventTypes(events, executorEventResumed) > 1 {
		t.Fatalf("events = %#v, want resumed event deduped", events)
	}
	encoded, err := json.Marshal(map[string]any{"result": result, "status": status, "events": events})
	if err != nil {
		t.Fatalf("marshal observed data: %v", err)
	}
	if strings.Contains(string(encoded), "remote-secret") {
		t.Fatalf("watch output leaked secret: %s", encoded)
	}
}

func TestRunExecutorWatchWritesRuntimeAndPreventsSecondWorker(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, _, _ := seedExecutorWorkspace(t, root)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := RunExecutorWatch(ctx, ExecutorWatchOptions{
			ExecutorOptions: ExecutorOptions{WorkspacePath: root},
			PollInterval:    10 * time.Millisecond,
		})
		done <- err
	}()
	t.Cleanup(cancel)

	deadline := time.Now().Add(2 * time.Second)
	for {
		status := readExecutorRuntimeStatus(workspace)
		if status.Active && status.Mode == "watch" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("executor runtime did not become active; last status = %#v", readExecutorRuntimeStatus(workspace))
		}
		time.Sleep(10 * time.Millisecond)
	}
	report, err := Doctor(DoctorOptions{Path: root})
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if !doctorCheckMessageContains(report, "executor_worker", "active") {
		t.Fatalf("Doctor() missing active executor worker check: %#v", report.Checks)
	}

	_, err = RunExecutorWatch(context.Background(), ExecutorWatchOptions{
		ExecutorOptions: ExecutorOptions{WorkspacePath: root},
		PollInterval:    time.Millisecond,
		MaxIterations:   1,
	})
	if err == nil {
		t.Fatal("second RunExecutorWatch() error = nil, want workspace_locked")
	}
	var workspaceErr *Error
	if !asWorkspaceError(err, &workspaceErr) || workspaceErr.Code != ErrorWorkspaceLocked {
		t.Fatalf("second RunExecutorWatch() error = %#v, want workspace_locked", err)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunExecutorWatch() after cancel error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunExecutorWatch() did not stop after cancel")
	}
	if status := readExecutorRuntimeStatus(workspace); status.Active || status.Exists {
		t.Fatalf("executor runtime after shutdown = %#v, want cleaned up", status)
	}
}

func TestHybridEcommerceRedactsRemoteErrors(t *testing.T) {
	t.Setenv("OPSC_HYBRID_TEST_TOKEN", "remote-secret")
	root := filepath.Join(t.TempDir(), "workspace")
	workspace, profileID, _ := seedExecutorWorkspace(t, root)

	var remoteRunID string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/workflows/pdd/templates/remote_tpl":
			_, _ = io.WriteString(w, `{"code":0,"data":{"id":"remote_tpl","workflowType":"pdd","title":"Redaction Ecommerce","spec":{"version":1,"nodes":[{"id":"stage_generate","operation":"image_generation","title":"Generate"}],"edges":[],"settings":{}}},"msg":"ok"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/workflows/pdd/templates/remote_tpl/runs":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode start payload: %v", err)
			}
			remoteRunID = payload["runId"].(string)
			_, _ = io.WriteString(w, `{"code":0,"data":{"runId":"`+remoteRunID+`"},"msg":"ok"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/overview"):
			_, _ = io.WriteString(w, `{"code":0,"data":{"run":{"runId":"`+remoteRunID+`","status":"failed","completed":true,"productTotal":1,"failedProducts":1,"recentError":"Bearer remote-secret failed at /srv/private/run"},"stages":[{"id":"stage_generate","title":"Generate","status":"failed","total":1,"failed":1,"recentError":"token=remote-secret path=/opt/pdd/private"}],"products":[],"recentErrors":["Authorization: Bearer remote-secret from /home/admin/private"]},"msg":"ok"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()

	profile := readExecutorProfile(t, workspace, profileID)
	profile.Data.Channels[0].Protocol = "ops-canvas-vps"
	profile.Data.Channels[0].BaseURL = remote.URL
	profile.Data.Channels[0].SecretRef = &SecretRef{Type: SecretRefTypeEnv, Name: "OPSC_HYBRID_TEST_TOKEN"}
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}
	imported, err := ImportHybridEcommerceTemplate(context.Background(), HybridEcommerceImportOptions{
		WorkspacePath:    root,
		RemoteTemplateID: "remote_tpl",
		ProfileID:        profileID,
		ChannelID:        "openai",
		HTTPClient:       remote.Client(),
	})
	if err != nil {
		t.Fatalf("ImportHybridEcommerceTemplate() error = %v", err)
	}
	draft, err := CreateHybridEcommerceRun(context.Background(), HybridEcommerceRunOptions{
		WorkspacePath: root,
		TemplateID:    imported.Template.ID,
		ProfileID:     profileID,
	})
	if err != nil {
		t.Fatalf("CreateHybridEcommerceRun() error = %v", err)
	}
	result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: draft.Run.ID, HTTPClient: remote.Client()})
	if err != nil {
		t.Fatalf("RunExecutorOnce() error = %v", err)
	}
	if len(result.Runs) != 1 || result.Runs[0].Status != RunStatusError {
		t.Fatalf("executor result = %#v, want failed hybrid run", result)
	}
	status, err := GetRunStatus(workspace, draft.Run.ID)
	if err != nil {
		t.Fatalf("GetRunStatus() error = %v", err)
	}
	events, err := ReadRunEvents(workspace, draft.Run.ID, 0)
	if err != nil {
		t.Fatalf("ReadRunEvents() error = %v", err)
	}
	encoded, err := json.Marshal(map[string]any{"result": result, "status": status, "events": events})
	if err != nil {
		t.Fatalf("marshal observed data: %v", err)
	}
	for _, leaked := range []string{"remote-secret", "/srv/private", "/opt/pdd", "/home/admin"} {
		if strings.Contains(string(encoded), leaked) {
			t.Fatalf("observed workspace data leaked %q:\n%s", leaked, encoded)
		}
	}
}

func TestHybridEcommerceRejectsUnsafeRemoteArtifactPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	result, err := Init(InitOptions{Path: root, Name: "Hybrid Artifact Guard"})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	for _, remotePath := range []string{"/srv/private/output.png", "../private/output.png", "https://example.invalid/output.png"} {
		err := syncHybridRemoteFile(context.Background(), result.Workspace, "run_missing", hybridVPSClient{}, hybridEcommerceConfig{RemoteTemplateID: "remote_tpl"}, "remote_run", "prod", "node", "Output", remotePath, "image", "image/png", "preview", "artifact", 0)
		if err == nil {
			t.Fatalf("syncHybridRemoteFile(%q) error = nil, want invalid path", remotePath)
		}
		if strings.Contains(err.Error(), remotePath) {
			t.Fatalf("unsafe path leaked in error %q: %v", remotePath, err)
		}
	}
}

func TestRunExecutorProjectGuardsCapabilitiesAndPathEscapes(t *testing.T) {
	for _, tc := range []struct {
		name         string
		capabilities ProjectCapabilities
		outputPath   string
		wantError    string
	}{
		{
			name:         "process exec capability disabled",
			capabilities: ProjectCapabilities{FSRead: true, FSWrite: true, ArtifactWrite: true},
			outputPath:   "outputs/out.txt",
			wantError:    "project capability process.exec is disabled",
		},
		{
			name:         "project output path escapes root",
			capabilities: ProjectCapabilities{FSRead: true, FSWrite: true, ProcessExec: true, ArtifactWrite: true},
			outputPath:   "../outside.txt",
			wantError:    "project path escapes root",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			workspace, _, _ := seedExecutorWorkspace(t, root)
			projectRoot := filepath.Join(t.TempDir(), "project")
			if err := os.MkdirAll(filepath.Join(projectRoot, "scripts"), 0o755); err != nil {
				t.Fatalf("create scripts dir: %v", err)
			}
			if err := os.MkdirAll(filepath.Join(projectRoot, "outputs"), 0o755); err != nil {
				t.Fatalf("create outputs dir: %v", err)
			}
			if err := AtomicWriteFile(filepath.Join(projectRoot, "scripts", "ok.sh"), []byte("#!/bin/sh\nprintf 'ok'\n"), 0o700); err != nil {
				t.Fatalf("write script: %v", err)
			}
			project := writeExecutorProject(t, workspace, projectRoot, tc.capabilities)
			template := writeExecutorTemplate(t, workspace, []map[string]any{
				{"id": "script", "type": "text", "operation": "script", "title": "Project Script", "extra": map[string]any{"executor": "project", "scriptPath": "scripts/ok.sh", "outputPath": tc.outputPath}},
			}, nil)
			run := writeExecutorRunWithProject(t, workspace, template.ID, "", project.ID, RunStatusPending, map[string]any{}, nil)
			appendWaitingForExecutor(t, workspace, run.ID)

			result, err := RunExecutorOnce(context.Background(), ExecutorOptions{WorkspacePath: root, RunID: run.ID})
			if err != nil {
				t.Fatalf("RunExecutorOnce() error = %v", err)
			}
			if result.Processed != 1 || len(result.Runs) != 1 || result.Runs[0].Status != RunStatusError {
				t.Fatalf("executor result = %#v, want failed project run", result)
			}
			if !strings.Contains(result.Runs[0].Error, tc.wantError) {
				t.Fatalf("run error = %q, want %q", result.Runs[0].Error, tc.wantError)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(projectRoot), "outside.txt")); !os.IsNotExist(err) {
				t.Fatalf("outside file stat err = %v, want not exist", err)
			}
			events, err := ReadRunEvents(workspace, run.ID, 0)
			if err != nil {
				t.Fatalf("ReadRunEvents() error = %v", err)
			}
			status, err := GetRunStatus(workspace, run.ID)
			if err != nil {
				t.Fatalf("GetRunStatus() error = %v", err)
			}
			assertNoExecutorRootLeak(t, projectRoot, result, events, status)
		})
	}
}

func seedExecutorWorkspace(t *testing.T, root string) (Workspace, string, string) {
	t.Helper()
	result, err := Init(InitOptions{Path: root, Name: "Executor Workspace"})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	workspace := result.Workspace
	profile, err := NewProfile(workspace, ProfileData{
		Name: "Executor Profile",
		Mode: ProfileModeLocal,
		Channels: []ProfileChannel{{
			ID:        "openai",
			Protocol:  "openai-compatible",
			BaseURL:   "http://127.0.0.1:1",
			Enabled:   true,
			SecretRef: &SecretRef{Type: SecretRefTypeEnv, Name: "OPSC_EXECUTOR_TEST_KEY"},
		}},
	})
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	if err := WriteProfile(workspace, profile); err != nil {
		t.Fatalf("WriteProfile() error = %v", err)
	}
	imageData, err := base64.StdEncoding.DecodeString(executorTestPNGBase64)
	if err != nil {
		t.Fatalf("decode test png: %v", err)
	}
	asset, err := NewAsset(workspace, AssetData{
		Type:      "image",
		MIME:      "image/png",
		Title:     "Fixed Image",
		MediaType: "image",
		Privacy:   PrivacyPrivate,
		Files:     map[string]string{"original": "files/original.png"},
	})
	if err != nil {
		t.Fatalf("NewAsset() error = %v", err)
	}
	if err := WriteAsset(workspace, asset); err != nil {
		t.Fatalf("WriteAsset() error = %v", err)
	}
	if err := AtomicWriteFile(filepath.Join(AssetRepository(workspace).Dir(asset.ID), "files", "original.png"), imageData, 0o600); err != nil {
		t.Fatalf("write asset file: %v", err)
	}
	return workspace, profile.ID, asset.ID
}

func readExecutorProfile(t *testing.T, workspace Workspace, profileID string) Envelope[ProfileData] {
	t.Helper()
	profile, err := ReadProfile(workspace, profileID)
	if err != nil {
		t.Fatalf("ReadProfile() error = %v", err)
	}
	return profile
}

func writeExecutorProject(t *testing.T, workspace Workspace, projectRoot string, capabilities ProjectCapabilities) Envelope[ProjectData] {
	t.Helper()
	project, err := NewProject(workspace, ProjectData{
		Name:         "Executor Project",
		Kind:         "code",
		Adapter:      "filesystem",
		RootPath:     projectRoot,
		Capabilities: capabilities,
		Execution: ProjectExecution{
			AllowGlobs: []string{"scripts/**", "outputs/**"},
		},
	})
	if err != nil {
		t.Fatalf("NewProject() error = %v", err)
	}
	if err := WriteProject(workspace, project); err != nil {
		t.Fatalf("WriteProject() error = %v", err)
	}
	return project
}

func writeExecutorTemplate(t *testing.T, workspace Workspace, nodes []map[string]any, edges []map[string]any) Envelope[TemplateData] {
	t.Helper()
	rawNodes := make([]json.RawMessage, 0, len(nodes))
	for _, node := range nodes {
		rawNodes = append(rawNodes, marshalExecutorRaw(t, node))
	}
	rawEdges := make([]json.RawMessage, 0, len(edges))
	for _, edge := range edges {
		rawEdges = append(rawEdges, marshalExecutorRaw(t, edge))
	}
	template, err := NewTemplate(workspace, TemplateData{
		Title:        "Executor Template",
		WorkflowType: "ecommerce",
		Version:      1,
		Nodes:        rawNodes,
		Edges:        rawEdges,
	})
	if err != nil {
		t.Fatalf("NewTemplate() error = %v", err)
	}
	if err := WriteTemplate(workspace, template); err != nil {
		t.Fatalf("WriteTemplate() error = %v", err)
	}
	return template
}

func writeExecutorRun(t *testing.T, workspace Workspace, templateID string, profileID string, status string, input map[string]any, metadata map[string]any) Envelope[RunData] {
	t.Helper()
	run, err := NewRun(workspace, RunData{
		TemplateID: templateID,
		ProfileID:  profileID,
		Status:     status,
		Input:      input,
		Metadata:   metadata,
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if err := WriteRun(workspace, run); err != nil {
		t.Fatalf("WriteRun() error = %v", err)
	}
	return run
}

func writeExecutorRunWithProject(t *testing.T, workspace Workspace, templateID string, profileID string, projectID string, status string, input map[string]any, metadata map[string]any) Envelope[RunData] {
	t.Helper()
	run, err := NewRun(workspace, RunData{
		TemplateID: templateID,
		ProfileID:  profileID,
		ProjectID:  projectID,
		Status:     status,
		Input:      input,
		Metadata:   metadata,
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if err := WriteRun(workspace, run); err != nil {
		t.Fatalf("WriteRun() error = %v", err)
	}
	return run
}

func appendWaitingForExecutor(t *testing.T, workspace Workspace, runID string) {
	t.Helper()
	if _, err := AppendRunEvent(workspace, runID, RunEventInput{
		Type:    "run.waiting_for_executor",
		Level:   "info",
		Actor:   RunEventActor{Type: "web", ID: "ops-canvas-web"},
		Message: "Local run draft created",
		Data:    map[string]any{"mode": "local"},
	}); err != nil {
		t.Fatalf("AppendRunEvent(waiting) error = %v", err)
	}
}

func marshalExecutorRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal raw: %v", err)
	}
	return data
}

func runEventTypesContain(events []RunEventEnvelope, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func doctorCheckMessageContains(report *DoctorReport, name string, part string) bool {
	for _, check := range report.Checks {
		if check.Name == name && strings.Contains(check.Message, part) {
			return true
		}
	}
	return false
}

func countRunEventTypes(events []RunEventEnvelope, eventType string) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func executorTestStateMap(states []Envelope[RunNodeStateData]) map[string]Envelope[RunNodeStateData] {
	out := map[string]Envelope[RunNodeStateData]{}
	for _, state := range states {
		out[state.Data.NodeID] = state
	}
	return out
}

func assertNoExecutorSecretLeak(t *testing.T, values ...any) {
	t.Helper()
	for _, value := range values {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal value for secret leak check: %v", err)
		}
		if strings.Contains(string(data), "provider-secret") {
			t.Fatalf("executor output leaked provider secret: %s", data)
		}
	}
}

func assertNoExecutorRootLeak(t *testing.T, root string, values ...any) {
	t.Helper()
	for _, value := range values {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal value for root leak check: %v", err)
		}
		if strings.Contains(string(data), root) {
			t.Fatalf("executor output leaked project root %q: %s", root, data)
		}
	}
}

func intSliceStrings(values []int) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.Itoa(value))
	}
	return out
}
