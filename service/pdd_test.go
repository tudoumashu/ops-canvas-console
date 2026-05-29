package service

import (
	"testing"

	"github.com/basketikun/infinite-canvas/model"
)

func TestMergeCreativeMetadataPreservesLocalUpload(t *testing.T) {
	saved := map[string]interface{}{
		"source":       "user_upload",
		"content":      "/api/workflows/pdd/runs/run-a/files/new.png",
		"artifactPath": "logs/creative_canvas/new.png",
		"status":       "success",
		"prompt":       "local prompt",
		"model":        "custom-image-model",
	}
	live := map[string]interface{}{
		"source":       "run_artifact",
		"content":      "/api/workflows/pdd/runs/run-a/files/old.png",
		"artifactPath": "logs/custom_workflow/old.png",
		"status":       "loading",
		"prompt":       "workflow prompt",
		"model":        "gpt-image-2",
	}

	got := mergeCreativeMetadata(saved, live)
	if got["content"] != saved["content"] || got["artifactPath"] != saved["artifactPath"] {
		t.Fatalf("local upload content was overwritten: %#v", got)
	}
	if got["status"] != "success" {
		t.Fatalf("local upload status = %v, want success", got["status"])
	}
	if got["model"] != "custom-image-model" || got["prompt"] != "local prompt" {
		t.Fatalf("local config was overwritten: %#v", got)
	}
}

func TestMergeCreativeMetadataUpdatesRunArtifactWithoutOverwritingConfig(t *testing.T) {
	saved := map[string]interface{}{
		"source":  "run_artifact",
		"content": "/api/workflows/pdd/runs/run-a/files/old.png",
		"status":  "loading",
		"model":   "custom-image-model",
	}
	live := map[string]interface{}{
		"source":        "run_artifact",
		"content":       "/api/workflows/pdd/runs/run-a/files/new.png",
		"artifactPath":  "logs/custom_workflow/new.png",
		"status":        "success",
		"model":         "gpt-image-2",
		"naturalWidth":  float64(2048),
		"naturalHeight": float64(1024),
	}

	got := mergeCreativeMetadata(saved, live)
	if got["content"] != live["content"] || got["artifactPath"] != live["artifactPath"] {
		t.Fatalf("run artifact content was not refreshed: %#v", got)
	}
	if got["status"] != "success" {
		t.Fatalf("run artifact status = %v, want success", got["status"])
	}
	if got["model"] != "custom-image-model" {
		t.Fatalf("saved model was overwritten: %#v", got)
	}
}

func TestCreativeCanvasShouldAutoRelayoutOnlyRunNodes(t *testing.T) {
	nodes := []model.PDDCreativeNode{
		{
			ID:       "reference",
			Type:     "image",
			Position: map[string]float64{"x": 80, "y": 80},
			Width:    420,
			Height:   640,
			Metadata: map[string]interface{}{"source": "run_artifact"},
		},
		{
			ID:       "source",
			Type:     "image",
			Position: map[string]float64{"x": 440, "y": 200},
			Width:    420,
			Height:   640,
			Metadata: map[string]interface{}{"source": "run_artifact"},
		},
	}
	if !creativeCanvasShouldAutoRelayout(nodes) {
		t.Fatal("overlapping run nodes should request auto relayout")
	}

	nodes[1].Metadata["source"] = "user_upload"
	if creativeCanvasShouldAutoRelayout(nodes) {
		t.Fatal("local upload nodes must not trigger auto relayout")
	}
}
