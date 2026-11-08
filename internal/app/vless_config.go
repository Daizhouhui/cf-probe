package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// rawGithubBase 单独抽出来，测试时可以指向 httptest。
var rawGithubBase = "https://raw.githubusercontent.com"

// maxConfigBytes 限制拉取的 config.js 大小，避免异常仓库拖垮程序。
const maxConfigBytes = 2 << 20

func validRepo(repo string) bool {
	parts := strings.Split(strings.Trim(repo, "/"), "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}

func validGitPath(path string) bool {
	path = strings.TrimSpace(path)
	return path != "" && !strings.HasPrefix(path, "/") && !strings.Contains(path, "..")
}

// fetchNodeWebConfig 优先走 RAW 地址，失败时退回 GitHub Contents API。
func fetchNodeWebConfig(ctx context.Context, repo, branch, path, token string) ([]byte, error) {
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	branch = strings.TrimSpace(branch)
	path = strings.TrimSpace(path)
	if branch == "" {
		branch = "main"
	}
	if !validRepo(repo) {
		return nil, fmt.Errorf("配置仓库格式应为 owner/repo")
	}
	if !validGitPath(path) {
		return nil, fmt.Errorf("配置文件路径不合法")
	}

	rawURL := strings.TrimSuffix(rawGithubBase, "/") + "/" + repo + "/" + branch + "/" + githubEscapePath(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err == nil {
		// raw.githubusercontent.com 不接受 Bearer；按 actions/checkout 的方式用 Basic。
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString(
			[]byte("x-access-token:"+normalizeGitHubToken(token))))
		req.Header.Set("Accept", "text/plain")
		req.Header.Set("User-Agent", browserUA)
		if resp, err := httpClient.Do(req); err == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, maxConfigBytes+1))
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if len(body) > maxConfigBytes {
					return nil, fmt.Errorf("config.js 超过 2MB 限制")
				}
				if len(body) > 0 {
					return body, nil
				}
			}
		}
	}

	// RAW 失败或为空：退回 Contents API
	data, _, apiErr := githubGetFile(ctx, repo, branch, path, token)
	if len(data) > maxConfigBytes {
		return nil, fmt.Errorf("config.js 超过 2MB 限制")
	}
	if len(data) > 0 {
		return data, nil
	}
	if apiErr != nil {
		return nil, fmt.Errorf("拉取 config.js 失败: %w", apiErr)
	}
	return nil, fmt.Errorf("拉取 config.js 失败：RAW 与 Contents API 都没有返回内容")
}

type vlessDefaults struct {
	UUID          string `json:"uuid"`
	Host          string `json:"host"`
	SNI           string `json:"sni"`
	Path          string `json:"path"`
	FP            string `json:"fp"`
	ECH           string `json:"ech"`
	Port          int    `json:"port"`
	Prefix        string `json:"prefix"`
	Encryption    string `json:"encryption"`
	Security      string `json:"security"`
	Network       string `json:"network"`
	Insecure      bool   `json:"insecure"`
	AllowInsecure bool   `json:"allowInsecure"`
	ForcePort443  bool   `json:"forcePort443"`
}

type vlessPresets struct {
	UUID []string `json:"uuid"`
	Host []string `json:"host"`
	SNI  []string `json:"sni"`
	Path []string `json:"path"`
	FP   []string `json:"fp"`
}

type nodeWebConfig struct {
	DefaultVless vlessDefaults `json:"defaultVlessConfig"`
	Github       struct {
		Repo   string `json:"repo"`
		Branch string `json:"branch"`
		Worker struct {
			BaseURL             string `json:"baseUrl"`
			Repo                string `json:"repo"`
			SubscriptionAPIPath string `json:"subscriptionApiPath"`
		} `json:"worker"`
	} `json:"github"`
	VlessPresets vlessPresets `json:"vlessPresets"`
}

// parseNodeWebConfig 在隔离的 goja VM 里执行 config.js，再取出 NODE_WEB_CONFIG。
// 这样注释、单引号、尾逗号等 JS 写法都不需要自己解析。
func parseNodeWebConfig(raw []byte) (*nodeWebConfig, error) {
	vm := goja.New()
	if err := vm.Set("window", vm.NewObject()); err != nil {
		return nil, err
	}
	timer := time.AfterFunc(2*time.Second, func() { vm.Interrupt("timeout") })
	defer timer.Stop()

	if _, err := vm.RunString(string(raw)); err != nil {
		return nil, fmt.Errorf("执行 config.js 失败: %w", err)
	}
	out, err := vm.RunString(`typeof window.NODE_WEB_CONFIG === "undefined" ? "" : JSON.stringify(window.NODE_WEB_CONFIG)`)
	if err != nil {
		return nil, fmt.Errorf("读取 NODE_WEB_CONFIG 失败: %w", err)
	}
	if out == nil || out.String() == "" {
		return nil, fmt.Errorf("config.js 里没有 window.NODE_WEB_CONFIG")
	}
	var cfg nodeWebConfig
	if err := json.Unmarshal([]byte(out.String()), &cfg); err != nil {
		return nil, fmt.Errorf("解析 NODE_WEB_CONFIG 失败: %w", err)
	}
	if strings.TrimSpace(cfg.DefaultVless.UUID) == "" ||
		strings.TrimSpace(cfg.DefaultVless.Host) == "" ||
		strings.TrimSpace(cfg.DefaultVless.SNI) == "" {
		return nil, fmt.Errorf("config.js 的 defaultVlessConfig 缺少 uuid/host/sni")
	}
	return &cfg, nil
}
