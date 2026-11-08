package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testConfigJS = `window.NODE_WEB_CONFIG = {
  defaultVlessConfig: { uuid: "uuid-1", host: "h.example", sni: "s.example", path: "/?ed=2048", fp: "chrome", ech: "ech.example+https://dns.example/q", port: 443, encryption: "none", security: "tls", network: "ws" },
  github: { repo: "Owner/Sub", branch: "main", worker: { baseUrl: "https://manager.example", repo: "Owner/Sub", subscriptionApiPath: "/api/subscription-link" } },
  vlessPresets: { uuid: ["uuid-1"], host: ["h.example"], sni: ["s.example"], path: ["/?ed=2048"], fp: ["chrome"] }
};`

func TestFetchNodeWebConfigRawFirst(t *testing.T) {
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.URL.Path != "/Owner/NodeWeb/main/config.js" {
			t.Fatalf("raw request wrong: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(testConfigJS))
	}))
	defer raw.Close()
	oldRaw := rawGithubBase
	rawGithubBase = raw.URL
	defer func() { rawGithubBase = oldRaw }()

	data, err := fetchNodeWebConfig(context.Background(), "Owner/NodeWeb", "main", "config.js", "tok")
	if err != nil || string(data) != testConfigJS {
		t.Fatalf("got %q, %v", data, err)
	}
}

func TestFetchNodeWebConfigFallsBackToContentsAPI(t *testing.T) {
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer raw.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":"` + base64.StdEncoding.EncodeToString([]byte(testConfigJS)) + `","sha":"s","encoding":"base64"}`))
	}))
	defer api.Close()
	oldRaw, oldAPI := rawGithubBase, githubAPIBase
	rawGithubBase, githubAPIBase = raw.URL, api.URL
	defer func() { rawGithubBase, githubAPIBase = oldRaw, oldAPI }()

	data, err := fetchNodeWebConfig(context.Background(), "Owner/NodeWeb", "main", "config.js", "tok")
	if err != nil || string(data) != testConfigJS {
		t.Fatalf("fallback failed: %q, %v", data, err)
	}
}

func TestFetchNodeWebConfigBothFail(t *testing.T) {
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer raw.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer api.Close()
	oldRaw, oldAPI := rawGithubBase, githubAPIBase
	rawGithubBase, githubAPIBase = raw.URL, api.URL
	defer func() { rawGithubBase, githubAPIBase = oldRaw, oldAPI }()

	if _, err := fetchNodeWebConfig(context.Background(), "Owner/NodeWeb", "main", "config.js", "tok"); err == nil {
		t.Fatal("both sources failing must return an error")
	}
}

func TestFetchNodeWebConfigRejectsOversize(t *testing.T) {
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), maxConfigBytes+1))
	}))
	defer raw.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer api.Close()
	oldRaw, oldAPI := rawGithubBase, githubAPIBase
	rawGithubBase, githubAPIBase = raw.URL, api.URL
	defer func() { rawGithubBase, githubAPIBase = oldRaw, oldAPI }()

	if _, err := fetchNodeWebConfig(context.Background(), "Owner/NodeWeb", "main", "config.js", "tok"); err == nil {
		t.Fatal("oversize config must fail")
	}
}
