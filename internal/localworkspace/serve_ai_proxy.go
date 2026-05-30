package localworkspace

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func (api *serveAPI) handleAIProxy(w http.ResponseWriter, r *http.Request, localPath string) {
	switch r.Method {
	case http.MethodGet, http.MethodPost, http.MethodPut:
	default:
		writeServeError(w, http.StatusMethodNotAllowed, NewError(ErrorInvalidArgument, "method not allowed", 1, nil))
		return
	}
	channel, err := api.selectAIProxyChannel(r)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	secret, err := resolveAIProxySecret(channel.SecretRef)
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	target, err := buildAIProxyTargetURL(channel.BaseURL, localPath, aiProxyRawQuery(r.URL.Query()))
	if err != nil {
		writeServeErrorFromError(w, err)
		return
	}
	proxyRequest, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), r.Body)
	if err != nil {
		writeServeErrorFromError(w, WrapError(ErrorInternal, "create ai proxy request", 5, err))
		return
	}
	copyAIProxyRequestHeaders(proxyRequest.Header, r.Header)
	proxyRequest.Header.Set("Authorization", "Bearer "+secret)
	response, err := http.DefaultClient.Do(proxyRequest)
	if err != nil {
		writeServeErrorFromError(w, WrapError(ErrorInternal, "call ai proxy target", 5, err))
		return
	}
	defer response.Body.Close()
	copyAIProxyResponseHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func (api *serveAPI) selectAIProxyChannel(r *http.Request) (ProfileChannel, error) {
	profileID := strings.TrimSpace(r.URL.Query().Get("profileId"))
	channelID := strings.TrimSpace(r.URL.Query().Get("channelId"))
	if profileID == "" {
		profileID = strings.TrimSpace(r.Header.Get("X-Opsc-Profile-Id"))
	}
	if channelID == "" {
		channelID = strings.TrimSpace(r.Header.Get("X-Opsc-Channel-Id"))
	}
	return selectLocalAIChannel(api.workspace, profileID, channelID)
}

func selectLocalAIChannel(workspace Workspace, profileID string, channelID string) (ProfileChannel, error) {
	profiles, err := ListProfiles(workspace)
	if err != nil {
		return ProfileChannel{}, err
	}
	if profileID == "" {
		profileID = strings.TrimSpace(workspace.Document.Data.DefaultProfileID)
	}
	if profileID != "" {
		for _, profile := range profiles {
			if profile.ID == profileID {
				if channel, ok := firstAIProxyChannel(profile.Data.Channels, channelID); ok {
					return channel, nil
				}
				return ProfileChannel{}, NewError(ErrorWorkspaceInvalid, "profile has no usable ai channel", 2, nil)
			}
		}
		return ProfileChannel{}, NewError(ErrorWorkspaceNotFound, "profile not found", 2, nil)
	}
	for _, profile := range profiles {
		if channel, ok := firstAIProxyChannel(profile.Data.Channels, channelID); ok {
			return channel, nil
		}
	}
	return ProfileChannel{}, NewError(ErrorWorkspaceInvalid, "no usable ai profile channel", 2, nil)
}

func firstAIProxyChannel(channels []ProfileChannel, channelID string) (ProfileChannel, bool) {
	for _, channel := range channels {
		if channelID != "" && channel.ID != channelID {
			continue
		}
		if !channel.Enabled {
			continue
		}
		if strings.TrimSpace(channel.BaseURL) == "" || channel.SecretRef == nil {
			continue
		}
		return channel, true
	}
	return ProfileChannel{}, false
}

func resolveAIProxySecret(ref *SecretRef) (string, error) {
	if ref == nil {
		return "", NewError(ErrorWorkspaceInvalid, "ai channel secretRef is missing", 2, nil)
	}
	if err := ref.Validate(); err != nil {
		return "", err
	}
	switch strings.TrimSpace(ref.Type) {
	case SecretRefTypeEnv:
		value := strings.TrimSpace(os.Getenv(strings.TrimSpace(ref.Name)))
		if value == "" {
			return "", NewError(ErrorWorkspaceInvalid, "ai channel env secret is not configured", 2, nil)
		}
		return value, nil
	case SecretRefTypeFile:
		path := strings.TrimSpace(ref.Path)
		if !filepath.IsAbs(path) {
			return "", NewError(ErrorWorkspaceInvalid, "ai channel file secret path must be absolute", 2, nil)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", NewError(ErrorWorkspaceInvalid, "read ai channel file secret failed", 2, nil)
		}
		value := strings.TrimSpace(string(data))
		if value == "" {
			return "", NewError(ErrorWorkspaceInvalid, "ai channel file secret is empty", 2, nil)
		}
		return value, nil
	default:
		return "", NewError(ErrorWorkspaceInvalid, "ai channel secretRef type is not supported by serve proxy", 2, map[string]string{"type": ref.Type})
	}
}

func buildAIProxyTargetURL(baseURL string, localPath string, rawQuery string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, NewError(ErrorWorkspaceInvalid, "ai channel baseUrl is invalid", 2, nil)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, NewError(ErrorWorkspaceInvalid, "ai channel baseUrl scheme is not allowed", 2, nil)
	}
	rest := strings.TrimPrefix(localPath, "/ai/v1")
	if rest == "" {
		rest = "/"
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(basePath, "/v1") {
		basePath += "/v1"
	}
	parsed.Path = singleJoiningSlash(basePath, rest)
	parsed.RawPath = ""
	parsed.RawQuery = rawQuery
	return parsed, nil
}

func aiProxyRawQuery(values url.Values) string {
	query := url.Values{}
	for key, value := range values {
		if key == "profileId" || key == "channelId" {
			continue
		}
		query[key] = append([]string{}, value...)
	}
	return query.Encode()
}

func singleJoiningSlash(a string, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	default:
		return a + b
	}
}

func copyAIProxyRequestHeaders(dst http.Header, src http.Header) {
	for _, key := range []string{"Content-Type", "Accept"} {
		if value := src.Values(key); len(value) > 0 {
			dst[key] = append([]string{}, value...)
		}
	}
}

func copyAIProxyResponseHeaders(dst http.Header, src http.Header) {
	for _, key := range []string{"Content-Type", "Cache-Control"} {
		if value := src.Values(key); len(value) > 0 {
			dst[key] = append([]string{}, value...)
		}
	}
}
