package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type apiResponse struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Msg  string          `json:"msg"`
}

type claimResponse struct {
	Job *agentJob `json:"job"`
}

type agentJob struct {
	ID             string   `json:"id"`
	ScriptPath     string   `json:"scriptPath"`
	Args           []string `json:"args"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
}

func main() {
	server := flag.String("server", "http://127.0.0.1:13000", "VPS console URL")
	token := flag.String("token", os.Getenv("LOCAL_AGENT_TOKEN"), "local agent token")
	root := flag.String("root", ".", "local repository root allowed for script execution")
	interval := flag.Duration("interval", 3*time.Second, "poll interval")
	flag.Parse()
	if strings.TrimSpace(*token) == "" {
		fatal("missing --token or LOCAL_AGENT_TOKEN")
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		fatal(err.Error())
	}
	for {
		job, err := claim(*server, *token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[local-agent] claim failed: %v\n", err)
			time.Sleep(*interval)
			continue
		}
		if job == nil {
			time.Sleep(*interval)
			continue
		}
		output, runErr := runJob(absRoot, *job)
		errText := ""
		if runErr != nil {
			errText = runErr.Error()
		}
		if err := complete(*server, *token, job.ID, output, errText); err != nil {
			fmt.Fprintf(os.Stderr, "[local-agent] complete failed job=%s err=%v\n", job.ID, err)
		}
	}
}

func claim(server string, token string) (*agentJob, error) {
	var payload claimResponse
	if err := post(server, token, "/api/local-agent/jobs/claim", nil, &payload); err != nil {
		return nil, err
	}
	return payload.Job, nil
}

func complete(server string, token string, id string, output string, errText string) error {
	return post(server, token, "/api/local-agent/jobs/"+id+"/complete", map[string]string{"output": output, "error": errText}, nil)
}

func post(server string, token string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(server, "/")+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 400 {
		return fmt.Errorf("http %d: %s", response.StatusCode, string(data))
	}
	var parsed apiResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	if parsed.Code != 0 {
		return fmt.Errorf("%s", parsed.Msg)
	}
	if out != nil && len(parsed.Data) > 0 {
		return json.Unmarshal(parsed.Data, out)
	}
	return nil
}

func runJob(root string, job agentJob) (string, error) {
	target, err := safeScriptPath(root, job.ScriptPath)
	if err != nil {
		return "", err
	}
	args := append([]string{}, job.Args...)
	command := target
	switch strings.ToLower(filepath.Ext(target)) {
	case ".py":
		command = "python"
		args = append([]string{target}, args...)
	case ".sh":
		command = "bash"
		args = append([]string{target}, args...)
	}
	timeout := time.Duration(job.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(output), fmt.Errorf("script timeout after %s", timeout)
	}
	return string(output), err
}

func safeScriptPath(root string, scriptPath string) (string, error) {
	scriptPath = filepath.Clean(filepath.FromSlash(strings.TrimSpace(scriptPath)))
	if scriptPath == "." || filepath.IsAbs(scriptPath) || strings.HasPrefix(scriptPath, "..") || strings.Contains(scriptPath, string(filepath.Separator)+".."+string(filepath.Separator)) {
		return "", fmt.Errorf("script path must be relative inside root")
	}
	target := filepath.Join(root, scriptPath)
	resolvedRoot, _ := filepath.Abs(root)
	resolvedTarget, _ := filepath.Abs(target)
	if resolvedTarget != resolvedRoot && !strings.HasPrefix(resolvedTarget, resolvedRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("script path escapes root")
	}
	return resolvedTarget, nil
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
