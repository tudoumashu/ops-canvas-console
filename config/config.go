package config

import (
	"crypto/rand"
	"encoding/base64"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	Port                 string `env:"PORT" envDefault:"8080"`
	AdminUsername        string `env:"ADMIN_USERNAME" envDefault:"admin"`
	AdminPassword        string `env:"ADMIN_PASSWORD" envDefault:"infinite-canvas"`
	JWTSecret            string `env:"JWT_SECRET" envDefault:"infinite-canvas"`
	JWTExpireHours       int    `env:"JWT_EXPIRE_HOURS" envDefault:"168"`
	StorageDriver        string `env:"STORAGE_DRIVER" envDefault:"sqlite"`
	DatabaseDSN          string `env:"DATABASE_DSN" envDefault:"data/infinite-canvas.db"`
	LinuxDoAuthorizeURL  string `env:"LINUX_DO_AUTHORIZE_URL" envDefault:"https://connect.linux.do/oauth2/authorize"`
	LinuxDoTokenURL      string `env:"LINUX_DO_TOKEN_URL" envDefault:"https://connect.linux.do/oauth2/token"`
	LinuxDoUserInfoURL   string `env:"LINUX_DO_USERINFO_URL" envDefault:"https://connect.linux.do/api/user"`
	ConsoleAssetsRoot    string `env:"CONSOLE_ASSETS_ROOT" envDefault:"data/assets"`
	VideoStorageRoot     string `env:"VIDEO_STORAGE_ROOT" envDefault:"data/video"`
	PDDWorkflowRoot      string `env:"PDD_WORKFLOW_ROOT" envDefault:"/opt/pdd-workflow"`
	PDDRunsRoot          string `env:"PDD_RUNS_ROOT" envDefault:"/opt/pdd-workflow/runs"`
	PDDMaterialsRoot     string `env:"PDD_MATERIALS_ROOT" envDefault:"/opt/pdd-workflow/materials"`
	PDDPromptsRoot       string `env:"PDD_PROMPTS_ROOT" envDefault:"/opt/pdd-workflow/prompts"`
	PDDPython            string `env:"PDD_PYTHON" envDefault:"/opt/pdd-venv/bin/python"`
	PDDWorkflowConfig    string `env:"PDD_WORKFLOW_CONFIG" envDefault:"config/provider/workflow.remote-chatgpt2api.example.json"`
	PDDWorkflowEnvFile   string `env:"PDD_WORKFLOW_ENV_FILE" envDefault:"/opt/pdd-workflow/.pdd-console.env"`
	PDDActionNSenter     bool   `env:"PDD_ACTION_NSENTER" envDefault:"true"`
	PDDActionAuditLog    string `env:"PDD_ACTION_AUDIT_LOG" envDefault:"data/pdd-action-audit.log"`
	PDDConsoleReadOnly   bool   `env:"PDD_CONSOLE_READ_ONLY" envDefault:"false"`
	PDDActionTimeoutSecs int    `env:"PDD_ACTION_TIMEOUT_SECONDS" envDefault:"120"`
	LocalAgentToken      string `env:"LOCAL_AGENT_TOKEN" envDefault:""`
}

var Cfg Config

func Load() error {
	_ = godotenv.Load()
	if err := env.Parse(&Cfg); err != nil {
		return err
	}
	if strings.TrimSpace(Cfg.JWTSecret) == "" || Cfg.JWTSecret == "infinite-canvas" {
		secret, err := randomSecret()
		if err != nil {
			return err
		}
		Cfg.JWTSecret = secret
	}
	return nil
}

func randomSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
