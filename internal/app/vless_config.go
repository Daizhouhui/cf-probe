package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
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
		req.Header.Set("Authorization", "Bearer "+normalizeGitHubToken(token))
		req.Header.Set("Accept", "text/plain")
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
