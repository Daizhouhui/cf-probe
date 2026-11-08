package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// requestSubscription 调用 Worker 的订阅链接接口，返回可用的订阅 URL。
func requestSubscription(ctx context.Context, baseURL, apiPath, token, file string) (string, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = "https://manager.knny.dpdns.org"
	}
	api := strings.TrimSpace(apiPath)
	if api == "" {
		api = "/api/subscription-link"
	}
	if !strings.HasPrefix(api, "/") {
		api = "/" + api
	}
	endpoint := strings.TrimRight(base, "/") + api

	payload, err := json.Marshal(map[string]string{"file": file})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+normalizeGitHubToken(token))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		msg := strings.TrimSpace(e.Error)
		if msg == "" {
			msg = truncate(string(body), 160)
		}
		return "", fmt.Errorf("获取订阅链接失败 HTTP %d: %s", resp.StatusCode, msg)
	}

	var out struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.URL) == "" {
		return "", fmt.Errorf("订阅接口没有返回链接")
	}
	return strings.TrimSpace(out.URL), nil
}
