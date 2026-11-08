package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// githubAPIBase 单独抽出来，测试时可以指向 httptest。
var githubAPIBase = "https://api.github.com"

func githubEscapePath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func githubContentsURL(repo, path string) string {
	return githubAPIBase + "/repos/" + strings.Trim(repo, "/") + "/contents/" + githubEscapePath(path)
}

func githubSetHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+normalizeGitHubToken(token))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

type githubFileMeta struct {
	Content  string `json:"content"`
	SHA      string `json:"sha"`
	Encoding string `json:"encoding"`
}

// verifyGitHubToken 先确认令牌本身有效，避免把「令牌复制错」误报成仓库权限问题。
func verifyGitHubToken(ctx context.Context, token string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIBase+"/user", nil)
	if err != nil {
		return err
	}
	githubSetHeaders(req, token)
	req.Header.Set("User-Agent", browserUA)
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("校验 GitHub Token 失败: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("GitHub Token 无效、已过期或复制不完整（fine-grained 通常约 93 位，经典令牌 40 位）")
	}
	return nil
}

// githubGetFile 读取文件内容与 sha；文件不存在返回 (nil, "", nil)。
func githubGetFile(ctx context.Context, repo, branch, path, token string) ([]byte, string, error) {
	u := githubContentsURL(repo, path)
	if branch != "" {
		u += "?ref=" + url.QueryEscape(branch)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	githubSetHeaders(req, token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if hint := githubAuthHint(resp.StatusCode, body); hint != nil {
			return nil, "", hint
		}
		return nil, "", fmt.Errorf("读取 GitHub 文件失败 HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var meta githubFileMeta
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, "", err
	}
	// Contents API 的 content 字段带换行，先去掉再解码。
	data, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(meta.Content, "\n", ""))
	if err != nil {
		return nil, "", err
	}
	return data, meta.SHA, nil
}

// githubPutFile 创建或更新文件；已存在时自动带 sha。
func githubPutFile(ctx context.Context, repo, branch, path, token, message string, data []byte) error {
	_, sha, err := githubGetFile(ctx, repo, branch, path, token)
	if err != nil {
		return err
	}
	payload := map[string]string{
		"message": message,
		"content": base64.StdEncoding.EncodeToString(data),
	}
	if branch != "" {
		payload["branch"] = branch
	}
	if sha != "" {
		payload["sha"] = sha
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, githubContentsURL(repo, path), bytes.NewReader(body))
	if err != nil {
		return err
	}
	githubSetHeaders(req, token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if hint := githubAuthHint(resp.StatusCode, rb); hint != nil {
			return hint
		}
		return fmt.Errorf("GitHub 写入失败 HTTP %d: %s", resp.StatusCode, truncate(string(rb), 200))
	}
	return nil
}
