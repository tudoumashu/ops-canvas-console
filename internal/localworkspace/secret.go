package localworkspace

import "strings"

const (
	SecretRefTypeEnv      = "env"
	SecretRefTypeKeychain = "keychain"
	SecretRefTypeFile     = "file"
	SecretRefTypeCloud    = "cloud"
)

type SecretRef struct {
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
	Service   string `json:"service,omitempty"`
	Account   string `json:"account,omitempty"`
	Path      string `json:"path,omitempty"`
	ChannelID string `json:"channelId,omitempty"`
}

type SecretRefSummary struct {
	Type       string `json:"type"`
	Reference  string `json:"reference,omitempty"`
	Redacted   bool   `json:"redacted"`
	Configured bool   `json:"configured"`
}

func (r SecretRef) Validate() error {
	refType := strings.TrimSpace(r.Type)
	if refType == "" {
		return NewError(ErrorInvalidArgument, "secretRef type is empty", 1, nil)
	}
	switch refType {
	case SecretRefTypeEnv:
		if strings.TrimSpace(r.Name) == "" {
			return NewError(ErrorInvalidArgument, "secretRef env name is empty", 1, nil)
		}
	case SecretRefTypeKeychain:
		if strings.TrimSpace(r.Service) == "" || strings.TrimSpace(r.Account) == "" {
			return NewError(ErrorInvalidArgument, "secretRef keychain service and account are required", 1, nil)
		}
	case SecretRefTypeFile:
		if strings.TrimSpace(r.Path) == "" {
			return NewError(ErrorInvalidArgument, "secretRef file path is empty", 1, nil)
		}
	case SecretRefTypeCloud:
		if strings.TrimSpace(r.ChannelID) == "" {
			return NewError(ErrorInvalidArgument, "secretRef cloud channelId is empty", 1, nil)
		}
	default:
		return NewError(ErrorInvalidArgument, "secretRef type is not allowed", 1, map[string]string{"type": refType})
	}
	return nil
}

func (r SecretRef) Summary() SecretRefSummary {
	refType := strings.TrimSpace(r.Type)
	summary := SecretRefSummary{Type: refType, Redacted: true}
	switch refType {
	case SecretRefTypeEnv:
		summary.Reference = strings.TrimSpace(r.Name)
		summary.Configured = summary.Reference != ""
	case SecretRefTypeKeychain:
		service := strings.TrimSpace(r.Service)
		account := strings.TrimSpace(r.Account)
		if service != "" && account != "" {
			summary.Reference = service + ":" + account
			summary.Configured = true
		}
	case SecretRefTypeFile:
		summary.Reference = "<file>"
		summary.Configured = strings.TrimSpace(r.Path) != ""
	case SecretRefTypeCloud:
		summary.Reference = strings.TrimSpace(r.ChannelID)
		summary.Configured = summary.Reference != ""
	default:
		summary.Configured = false
	}
	return summary
}
