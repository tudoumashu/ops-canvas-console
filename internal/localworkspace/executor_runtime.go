package localworkspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	executorRuntimeFile = "executor.json"
	executorPIDFile     = "executor.pid"
	executorWatchLock   = "executor.watch.lock"
)

type ExecutorRuntimeMetadata struct {
	SchemaVersion      string `json:"schemaVersion"`
	PID                int    `json:"pid"`
	WorkspaceID        string `json:"workspaceId"`
	Mode               string `json:"mode"`
	RunID              string `json:"runId,omitempty"`
	StartedAt          string `json:"startedAt"`
	HeartbeatAt        string `json:"heartbeatAt"`
	PollIntervalMillis int    `json:"pollIntervalMillis,omitempty"`
	Iteration          int    `json:"iteration"`
	Processed          int    `json:"processed"`
	LastRunID          string `json:"lastRunId,omitempty"`
	LastRunStatus      string `json:"lastRunStatus,omitempty"`
	LastError          string `json:"lastError,omitempty"`
}

type executorRuntimeStatus struct {
	Exists      bool
	LockExists  bool
	Active      bool
	Stale       bool
	PID         int
	Mode        string
	RunID       string
	StartedAt   string
	HeartbeatAt string
	Message     string
}

func writeExecutorRuntimeFiles(workspace Workspace, metadata ExecutorRuntimeMetadata) error {
	if err := AtomicWriteFile(workspace.RuntimePath(executorPIDFile), []byte(strconv.Itoa(metadata.PID)+"\n"), 0o600); err != nil {
		return err
	}
	return AtomicWriteJSON(workspace.RuntimePath(executorRuntimeFile), metadata, 0o600)
}

func cleanupExecutorRuntimeFiles(workspace Workspace) {
	for _, name := range []string{executorRuntimeFile, executorPIDFile} {
		_ = os.Remove(workspace.RuntimePath(name))
	}
	if dir, err := workspace.StateDir(); err == nil {
		syncDir(dir)
	}
}

func clearStaleExecutorState(workspace Workspace) error {
	status := readExecutorRuntimeStatus(workspace)
	if status.LockExists && status.Active {
		return nil
	}
	if !status.LockExists && !status.Exists {
		return nil
	}
	cleanupExecutorRuntimeFiles(workspace)
	if !status.Active {
		if err := os.Remove(workspace.LockPath(executorWatchLock)); err != nil && !os.IsNotExist(err) {
			return WrapError(ErrorInternal, "remove stale executor lock", 5, err)
		}
	}
	return nil
}

func readExecutorRuntimeStatus(workspace Workspace) executorRuntimeStatus {
	status := executorRuntimeStatus{}
	if _, err := os.Stat(workspace.LockPath(executorWatchLock)); err == nil {
		status.LockExists = true
	}
	metadata, metadataOK := readExecutorRuntimeMetadata(workspace)
	if metadataOK {
		status.Exists = true
		status.PID = metadata.PID
		status.Mode = metadata.Mode
		status.RunID = metadata.RunID
		status.StartedAt = metadata.StartedAt
		status.HeartbeatAt = metadata.HeartbeatAt
	} else if pid := readExecutorRuntimePID(workspace); pid > 0 {
		status.Exists = true
		status.PID = pid
	}
	status.Active = status.LockExists && status.PID > 0 && processExists(status.PID)
	status.Stale = (status.LockExists || status.Exists) && !status.Active
	switch {
	case status.Active:
		status.Message = "executor worker is active"
	case status.Stale:
		status.Message = "executor worker runtime is stale; restart opsc executor --watch or remove stale state by starting it again"
	default:
		status.Message = "executor worker is not running"
	}
	return status
}

func readExecutorRuntimeMetadata(workspace Workspace) (ExecutorRuntimeMetadata, bool) {
	var metadata ExecutorRuntimeMetadata
	data, err := os.ReadFile(workspace.RuntimePath(executorRuntimeFile))
	if err != nil {
		return metadata, false
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return ExecutorRuntimeMetadata{}, false
	}
	return metadata, true
}

func readExecutorRuntimePID(workspace Workspace) int {
	data, err := os.ReadFile(workspace.RuntimePath(executorPIDFile))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

func latestCanonicalFileModTime(root string) (time.Time, error) {
	latest := time.Time{}
	includeRootFile := filepath.Join(root, WorkspaceFileName)
	if stat, err := os.Stat(includeRootFile); err == nil && !stat.IsDir() {
		latest = stat.ModTime()
	} else if err != nil && !os.IsNotExist(err) {
		return latest, WrapError(ErrorInternal, "stat workspace document", 5, err)
	}
	for _, dir := range RequiredDirs {
		if dir == "cache" || dir == "exports" {
			continue
		}
		base := filepath.Join(root, dir)
		if err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if filepath.Base(path) == IndexFileName {
				return nil
			}
			stat, err := entry.Info()
			if err != nil {
				return err
			}
			if stat.ModTime().After(latest) {
				latest = stat.ModTime()
			}
			return nil
		}); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return latest, WrapError(ErrorInternal, "scan canonical file mtimes", 5, err)
		}
	}
	return latest, nil
}
