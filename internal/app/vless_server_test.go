package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalOnlyMiddleware(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
	handler := localOnly(ok)

	cases := []struct {
		remote string
		want   int
	}{
		{"127.0.0.1:5000", http.StatusOK},
		{"[::1]:5000", http.StatusOK},
		{"192.168.1.5:5000", http.StatusForbidden},
		{"10.0.0.2:5000", http.StatusForbidden},
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", "/api/vless/upload", nil)
		req.RemoteAddr = c.remote
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != c.want {
			t.Errorf("remote %s: got %d want %d", c.remote, rec.Code, c.want)
		}
	}
}

func withTestConfig(t *testing.T, c *Config) {
	t.Helper()
	oldCfg := cfg
	cfg = c
	t.Cleanup(func() { cfg = oldCfg })
	oldSave := vlessSaveConfig
	// 模拟 SaveConfig：更新内存里的 cfg，但不落盘
	vlessSaveConfig = func(c *Config) error { cfg = c; return nil }
	t.Cleanup(func() { vlessSaveConfig = oldSave })
}

func TestVlessEnableStatusAndRefresh(t *testing.T) {
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testConfigJS))
	}))
	defer raw.Close()
	oldRaw := rawGithubBase
	rawGithubBase = raw.URL
	defer func() { rawGithubBase = oldRaw }()

	withTestConfig(t, DefaultConfig())
	s := NewServer()

	body := `{"token":"tok","repo":"Owner/NodeWeb","branch":"main","path":"config.js"}`
	req := httptest.NewRequest("POST", "/api/vless/enable", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("enable failed: %d %s", rec.Code, rec.Body.String())
	}
	if !cfg.VlessEnabled || cfg.VlessConfigCache != testConfigJS || cfg.VlessUploadRepo != "Owner/Sub" {
		t.Fatalf("enable did not persist: %+v", cfg)
	}

	statusReq := httptest.NewRequest("GET", "/api/vless/status", nil)
	statusReq.RemoteAddr = "127.0.0.1:5000"
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, statusReq)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"enabled":true`) ||
		!strings.Contains(rec.Body.String(), `"uuid":"uuid-1"`) {
		t.Fatalf("status wrong: %d %s", rec.Code, rec.Body.String())
	}

	refreshReq := httptest.NewRequest("POST", "/api/vless/config/refresh", nil)
	refreshReq.RemoteAddr = "127.0.0.1:5000"
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, refreshReq)
	if rec.Code != 200 {
		t.Fatalf("refresh failed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestVlessEndpointsRejectRemote(t *testing.T) {
	withTestConfig(t, DefaultConfig())
	s := NewServer()
	req := httptest.NewRequest("GET", "/api/vless/status", nil)
	req.RemoteAddr = "192.168.1.9:5000"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("remote status should be 403, got %d", rec.Code)
	}
}

func TestVlessStatusDisabled(t *testing.T) {
	withTestConfig(t, DefaultConfig())
	s := NewServer()
	req := httptest.NewRequest("GET", "/api/vless/status", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}
