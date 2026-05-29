package service

import (
	"testing"

	"github.com/basketikun/infinite-canvas/model"
)

func TestNormalizeWorkflowImageQuality(t *testing.T) {
	tests := []struct {
		name    string
		quality string
		want    string
	}{
		{name: "auto omitted", quality: "auto", want: ""},
		{name: "empty omitted", quality: "", want: ""},
		{name: "alias 1k", quality: "1k", want: "low"},
		{name: "alias 2k", quality: "2k", want: "medium"},
		{name: "alias 4k", quality: "4k", want: "high"},
		{name: "case insensitive", quality: " HIGH ", want: "high"},
		{name: "invalid omitted", quality: "bad", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeWorkflowImageQuality(tt.quality); got != tt.want {
				t.Fatalf("normalizeWorkflowImageQuality(%q) = %q, want %q", tt.quality, got, tt.want)
			}
		})
	}
}

func TestResolveWorkflowImageRequestSize(t *testing.T) {
	tests := []struct {
		name    string
		quality string
		size    string
		want    string
	}{
		{name: "auto omitted", quality: "", size: "auto", want: ""},
		{name: "empty omitted", quality: "high", size: "", want: ""},
		{name: "pixel size preserved", quality: "hd", size: "1024x1536", want: "1024x1536"},
		{name: "high square ratio", quality: "high", size: "1:1", want: "2880x2880"},
		{name: "medium portrait ratio", quality: "medium", size: "2:3", want: "1664x2496"},
		{name: "medium landscape ratio", quality: "medium", size: "16:9", want: "2720x1536"},
		{name: "ratio preserved without quality", quality: "", size: "2:3", want: "2:3"},
		{name: "invalid ratio preserved", quality: "high", size: "wide", want: "wide"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveWorkflowImageRequestSize(tt.quality, tt.size); got != tt.want {
				t.Fatalf("resolveWorkflowImageRequestSize(%q, %q) = %q, want %q", tt.quality, tt.size, got, tt.want)
			}
		})
	}
}

func TestNormalizeWorkflowImageCount(t *testing.T) {
	tests := []struct {
		count int
		want  int
	}{
		{count: -1, want: 1},
		{count: 0, want: 1},
		{count: 1, want: 1},
		{count: 15, want: 15},
		{count: 16, want: 15},
	}
	for _, tt := range tests {
		if got := normalizeWorkflowImageCount(tt.count); got != tt.want {
			t.Fatalf("normalizeWorkflowImageCount(%d) = %d, want %d", tt.count, got, tt.want)
		}
	}
}

func TestResolveFlow2APIImageModel(t *testing.T) {
	channel := model.ModelChannel{Models: []string{
		"gemini-3.0-pro-image-landscape",
		"gemini-3.0-pro-image-portrait-4k",
		"gemini-3.1-flash-image-square-2k",
		"imagen-4.0-generate-preview-landscape",
		"imagen-4.0-generate-preview-portrait",
	}}
	tests := []struct {
		name      string
		modelName string
		options   Flow2APIImageOptions
		want      string
	}{
		{name: "nano pro portrait 4k", modelName: "gemini-3.0-pro-image-landscape", options: Flow2APIImageOptions{Size: "portrait", Quality: "high"}, want: "gemini-3.0-pro-image-portrait-4k"},
		{name: "nano flash square 2k", modelName: "gemini-3.1-flash-image-landscape", options: Flow2APIImageOptions{Size: "square", Quality: "medium"}, want: "gemini-3.1-flash-image-square-2k"},
		{name: "imagen portrait", modelName: "imagen-4.0-generate-preview-landscape", options: Flow2APIImageOptions{Size: "720x1280", Quality: "high"}, want: "imagen-4.0-generate-preview-portrait"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveFlow2APIImageModel(channel, tt.modelName, tt.options); got != tt.want {
				t.Fatalf("resolveFlow2APIImageModel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSelectFlow2APIVideoModel(t *testing.T) {
	models := []string{
		"veo_3_1_t2v_fast_landscape_6s",
		"veo_3_1_t2v_fast_portrait_6s_1080p",
		"veo_3_1_i2v_s_fast_portrait_4s_fl",
		"veo_3_1_r2v_fast_landscape_ultra_4k",
	}
	tests := []struct {
		name      string
		modelName string
		mode      string
		options   Flow2APIVideoOptions
		want      string
	}{
		{name: "text portrait 1080", modelName: "veo_3_1_t2v_fast_landscape", mode: "text", options: Flow2APIVideoOptions{Size: "720x1280", Seconds: "6", Resolution: "1080p"}, want: "veo_3_1_t2v_fast_portrait_6s_1080p"},
		{name: "frame portrait 4s", modelName: "veo_3_1_i2v_s_fast_landscape", mode: "frame", options: Flow2APIVideoOptions{Size: "720x1280", Seconds: "4", Resolution: "720p"}, want: "veo_3_1_i2v_s_fast_portrait_4s_fl"},
		{name: "asset landscape 4k", modelName: "veo_3_1_r2v_fast_landscape", mode: "asset", options: Flow2APIVideoOptions{Size: "1280x720", Resolution: "4k"}, want: "veo_3_1_r2v_fast_landscape_ultra_4k"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectFlow2APIVideoModel(models, tt.modelName, tt.mode, tt.options); got != tt.want {
				t.Fatalf("selectFlow2APIVideoModel() = %q, want %q", got, tt.want)
			}
		})
	}
}
