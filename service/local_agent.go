package service

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/basketikun/infinite-canvas/config"
)

type LocalAgentScriptJob struct {
	ID             string   `json:"id"`
	RunID          string   `json:"runId"`
	ProductKey     string   `json:"productKey"`
	NodeID         string   `json:"nodeId"`
	ScriptPath     string   `json:"scriptPath"`
	Args           []string `json:"args"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
	Status         string   `json:"status"`
	Output         string   `json:"output,omitempty"`
	Error          string   `json:"error,omitempty"`
	CreatedAt      string   `json:"createdAt"`
	UpdatedAt      string   `json:"updatedAt"`
}

var localAgentStore = struct {
	sync.Mutex
	jobs map[string]*LocalAgentScriptJob
}{
	jobs: map[string]*LocalAgentScriptJob{},
}

func ValidateLocalAgentToken(token string) error {
	expected := strings.TrimSpace(config.Cfg.LocalAgentToken)
	if expected == "" {
		return safeMessageError{message: "LOCAL_AGENT_TOKEN 未配置，本地 agent 已禁用"}
	}
	if strings.TrimSpace(token) != expected {
		return safeMessageError{message: "本地 agent token 不正确"}
	}
	return nil
}

func RunLocalAgentScript(job LocalAgentScriptJob, timeout time.Duration) (string, error) {
	if strings.TrimSpace(config.Cfg.LocalAgentToken) == "" {
		return "", safeMessageError{message: "LOCAL_AGENT_TOKEN 未配置，无法执行本地脚本节点"}
	}
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	job.ID = newID("local-agent-job")
	job.Status = "pending"
	job.CreatedAt = now()
	job.UpdatedAt = job.CreatedAt
	localAgentStore.Lock()
	localAgentStore.jobs[job.ID] = &job
	localAgentStore.Unlock()

	deadline := time.Now().Add(timeout)
	for {
		localAgentStore.Lock()
		current := localAgentStore.jobs[job.ID]
		status := ""
		output := ""
		errText := ""
		if current != nil {
			status = current.Status
			output = current.Output
			errText = current.Error
		}
		localAgentStore.Unlock()
		switch status {
		case "completed":
			return output, nil
		case "failed":
			return output, errors.New(firstString(errText, "本地脚本执行失败"))
		}
		if time.Now().After(deadline) {
			return output, safeMessageError{message: "本地脚本节点等待 agent 超时"}
		}
		time.Sleep(time.Second)
	}
}

func ClaimLocalAgentJob(token string) (LocalAgentScriptJob, bool, error) {
	if err := ValidateLocalAgentToken(token); err != nil {
		return LocalAgentScriptJob{}, false, err
	}
	localAgentStore.Lock()
	defer localAgentStore.Unlock()
	for _, job := range localAgentStore.jobs {
		if job.Status != "pending" {
			continue
		}
		job.Status = "running"
		job.UpdatedAt = now()
		return *job, true, nil
	}
	return LocalAgentScriptJob{}, false, nil
}

func CompleteLocalAgentJob(token string, jobID string, output string, errText string) error {
	if err := ValidateLocalAgentToken(token); err != nil {
		return err
	}
	localAgentStore.Lock()
	defer localAgentStore.Unlock()
	job := localAgentStore.jobs[jobID]
	if job == nil {
		return safeMessageError{message: "本地 agent 任务不存在"}
	}
	job.Output = output
	job.Error = errText
	job.UpdatedAt = now()
	if strings.TrimSpace(errText) != "" {
		job.Status = "failed"
	} else {
		job.Status = "completed"
	}
	return nil
}
