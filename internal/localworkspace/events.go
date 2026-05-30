package localworkspace

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RunEventActor struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
}

type RunEventSubject struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type RunEventEnvelope struct {
	SchemaVersion string          `json:"schemaVersion"`
	ID            string          `json:"id"`
	Sequence      int64           `json:"sequence"`
	Type          string          `json:"type"`
	Level         string          `json:"level"`
	Actor         RunEventActor   `json:"actor"`
	Subject       RunEventSubject `json:"subject"`
	Message       string          `json:"message"`
	CreatedAt     string          `json:"createdAt"`
	Data          map[string]any  `json:"data,omitempty"`
}

type RunEventInput struct {
	Type      string
	Level     string
	Actor     RunEventActor
	Message   string
	CreatedAt time.Time
	Data      map[string]any
}

func AppendRunEvent(workspace Workspace, runID string, input RunEventInput) (RunEventEnvelope, error) {
	var event RunEventEnvelope
	err := withWorkspaceLock(workspace, func() error {
		if _, err := ReadRun(workspace, runID); err != nil {
			return err
		}
		next, err := nextRunEventSequence(workspace, runID)
		if err != nil {
			return err
		}
		event, err = buildRunEvent(runID, next, input)
		if err != nil {
			return err
		}
		if err := appendRunEventUnlocked(workspace, runID, event); err != nil {
			return err
		}
		return withIndex(workspace, func(index *WorkspaceIndex) error {
			if err := index.UpsertRunEvent(runID, event); err != nil {
				return err
			}
			run, err := ReadRun(workspace, runID)
			if err != nil {
				return err
			}
			return index.UpsertRun(workspace, run)
		})
	})
	return event, err
}

func ReadRunEvents(workspace Workspace, runID string, afterSequence int64) ([]RunEventEnvelope, error) {
	if err := RunRepository(workspace).validateID(runID); err != nil {
		return nil, err
	}
	if _, err := ReadRun(workspace, runID); err != nil {
		return nil, err
	}
	path := runEventsPath(workspace, runID)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []RunEventEnvelope{}, nil
		}
		return nil, WrapError(ErrorInternal, "read run events", 5, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	events := []RunEventEnvelope{}
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var event RunEventEnvelope
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, WrapError(ErrorWorkspaceInvalid, "parse run event line", 2, err)
		}
		if err := validateRunEvent(runID, event); err != nil {
			return nil, NewError(ErrorWorkspaceInvalid, "invalid run event line", 2, map[string]any{"line": line, "error": err.Error()})
		}
		if event.Sequence > afterSequence {
			events = append(events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, WrapError(ErrorInternal, "scan run events", 5, err)
	}
	return events, nil
}

func FollowRunEvents(ctx context.Context, workspace Workspace, runID string, afterSequence int64, pollInterval time.Duration, emit func(RunEventEnvelope) error) error {
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}
	last := afterSequence
	for {
		events, err := ReadRunEvents(workspace, runID, last)
		if err != nil {
			return err
		}
		for _, event := range events {
			if err := emit(event); err != nil {
				return err
			}
			last = event.Sequence
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(pollInterval):
		}
	}
}

func LatestRunEventSequence(workspace Workspace, runID string) (int64, error) {
	events, err := ReadRunEvents(workspace, runID, 0)
	if err != nil {
		return 0, err
	}
	var latest int64
	for _, event := range events {
		if event.Sequence > latest {
			latest = event.Sequence
		}
	}
	return latest, nil
}

func buildRunEvent(runID string, sequence int64, input RunEventInput) (RunEventEnvelope, error) {
	now := input.CreatedAt
	if now.IsZero() {
		now = time.Now()
	}
	id, err := NewID("evt", now)
	if err != nil {
		return RunEventEnvelope{}, err
	}
	level := strings.TrimSpace(input.Level)
	if level == "" {
		level = "info"
	}
	actor := input.Actor
	if strings.TrimSpace(actor.Type) == "" {
		actor.Type = "system"
	}
	if strings.TrimSpace(actor.ID) == "" {
		actor.ID = "localworkspace"
	}
	event := RunEventEnvelope{
		SchemaVersion: SchemaVersion,
		ID:            id,
		Sequence:      sequence,
		Type:          strings.TrimSpace(input.Type),
		Level:         level,
		Actor:         actor,
		Subject: RunEventSubject{
			Kind: KindRun,
			ID:   runID,
		},
		Message:   strings.TrimSpace(input.Message),
		CreatedAt: now.UTC().Format(time.RFC3339),
		Data:      input.Data,
	}
	if event.Data == nil {
		event.Data = map[string]any{}
	}
	if err := validateRunEvent(runID, event); err != nil {
		return RunEventEnvelope{}, err
	}
	return event, nil
}

func appendRunEventUnlocked(workspace Workspace, runID string, event RunEventEnvelope) error {
	if err := validateRunEvent(runID, event); err != nil {
		return err
	}
	path := runEventsPath(workspace, runID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return WrapError(ErrorInternal, "create run events directory", 5, err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return WrapError(ErrorInternal, "open run events", 5, err)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		_ = file.Close()
		return WrapError(ErrorInternal, "encode run event", 5, err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		_ = file.Close()
		return WrapError(ErrorInternal, "append run event", 5, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return WrapError(ErrorInternal, "sync run event", 5, err)
	}
	if err := file.Close(); err != nil {
		return WrapError(ErrorInternal, "close run events", 5, err)
	}
	syncDir(filepath.Dir(path))
	return nil
}

func nextRunEventSequence(workspace Workspace, runID string) (int64, error) {
	latest, err := LatestRunEventSequence(workspace, runID)
	if err != nil {
		return 0, err
	}
	return latest + 1, nil
}

func validateRunEvent(runID string, event RunEventEnvelope) error {
	if event.SchemaVersion != SchemaVersion {
		return NewError(ErrorWorkspaceInvalid, "event schema version mismatch", 2, map[string]string{"schemaVersion": event.SchemaVersion})
	}
	if err := validateScannedID(event.ID, "evt"); err != nil {
		return err
	}
	if event.Sequence < 1 {
		return NewError(ErrorWorkspaceInvalid, "event sequence must be at least 1", 2, nil)
	}
	if strings.TrimSpace(event.Type) == "" {
		return NewError(ErrorWorkspaceInvalid, "event type is empty", 2, nil)
	}
	switch event.Level {
	case "debug", "info", "warn", "error":
	default:
		return NewError(ErrorWorkspaceInvalid, "event level is not allowed", 2, map[string]string{"level": event.Level})
	}
	switch event.Actor.Type {
	case "cli", "serve", "mcp", "web", "system", "project_adapter":
	default:
		return NewError(ErrorWorkspaceInvalid, "event actor type is not allowed", 2, map[string]string{"actorType": event.Actor.Type})
	}
	if event.Subject.Kind != KindRun || event.Subject.ID != runID {
		return NewError(ErrorWorkspaceInvalid, "event subject must match run", 2, map[string]string{"runId": runID, "subjectId": event.Subject.ID})
	}
	if _, err := time.Parse(time.RFC3339, event.CreatedAt); err != nil {
		return WrapError(ErrorWorkspaceInvalid, "parse event createdAt", 2, err)
	}
	return nil
}

func runEventsPath(workspace Workspace, runID string) string {
	return filepath.Join(workspace.Root, "runs", runID, "events.jsonl")
}
