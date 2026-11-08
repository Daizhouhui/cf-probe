package app

import (
	"encoding/base64"
	"encoding/json"
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

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"login":"test"}`))
	}))
	defer api.Close()
	oldAPI := githubAPIBase
	githubAPIBase = api.URL
	defer func() { githubAPIBase = oldAPI }()

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

type ghMock struct {
	existing string
	put      string
	putPath  string
	putMsg   string
}

func newGHMock(t *testing.T) (*ghMock, func()) {
	t.Helper()
	m := &ghMock{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if m.existing == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"content":  base64.StdEncoding.EncodeToString([]byte(m.existing)),
				"sha":      "sha1",
				"encoding": "base64",
			})
		case http.MethodPut:
			var body struct {
				Content, Message, Branch, Sha string
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			raw, _ := base64.StdEncoding.DecodeString(body.Content)
			m.put, m.putPath, m.putMsg = string(raw), r.URL.Path, body.Message
			w.WriteHeader(http.StatusOK)
		}
	}))
	old := githubAPIBase
	githubAPIBase = srv.URL
	return m, func() { srv.Close(); githubAPIBase = old }
}

func doVlessUpload(s *Server, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/vless/upload", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

type uploadFixture struct {
	srv   *Server
	gh    *ghMock
	close func()
}

func setupUploadTest(t *testing.T, existing string, rawStatus int, subHandler http.HandlerFunc) *uploadFixture {
	t.Helper()
	sub := httptest.NewServer(subHandler)
	js := strings.Replace(testConfigJS, "https://manager.example", sub.URL, 1)
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rawStatus != http.StatusOK {
			w.WriteHeader(rawStatus)
			return
		}
		_, _ = w.Write([]byte(js))
	}))
	oldRaw := rawGithubBase
	rawGithubBase = raw.URL
	gh, closeGH := newGHMock(t)
	gh.existing = existing

	c := DefaultConfig()
	c.VlessEnabled = true
	c.VlessConfigToken = "cfg-token"
	c.VlessConfigCache = js
	c.VlessUploadRepo = "Owner/Sub"
	c.VlessUploadToken = "up-token"
	c.VlessFile = "vlesssub"
	c.VlessMerge = true
	withTestConfig(t, c)

	s := NewServer()
	s.runner.results = []Result{
		{IP: "1.1.1.1", Port: 443, Speed: 20, SpeedQualified: true, SpeedTested: true, Colo: "HKG"},
		{IP: "2.2.2.2", Port: 443, Speed: 5, SpeedQualified: true, SpeedTested: true, Colo: "SIN"},
		{IP: "3.3.3.3", Port: 443, Speed: 1, SpeedQualified: false, SpeedTested: true},
	}
	return &uploadFixture{srv: s, gh: gh, close: func() {
		closeGH()
		raw.Close()
		sub.Close()
		rawGithubBase = oldRaw
	}}
}

func TestVlessUploadNewFile(t *testing.T) {
	f := setupUploadTest(t, "", http.StatusOK, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"https://sub.example/vlesssub"}`))
	})
	defer f.close()

	rec := doVlessUpload(f.srv, `{"count":0,"merge":true}`)
	if rec.Code != 200 {
		t.Fatalf("upload failed: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"url":"https://sub.example/vlesssub"`) {
		t.Fatalf("url missing: %s", rec.Body.String())
	}
	if f.gh.putPath != "/repos/Owner/Sub/contents/vlesssub" {
		t.Fatalf("wrong put path: %s", f.gh.putPath)
	}
	if strings.Count(f.gh.put, "vless://") != 2 ||
		!strings.Contains(f.gh.put, "@1.1.1.1:443") ||
		!strings.Contains(f.gh.put, "@2.2.2.2:443") {
		t.Fatalf("expected 2 qualified nodes, got:\n%s", f.gh.put)
	}
}

func TestVlessUploadMergesExisting(t *testing.T) {
	existing := "vless://u@1.1.1.1:443?old#old\nvless://u@9.9.9.9:443?keep#keep\n"
	f := setupUploadTest(t, existing, http.StatusOK, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"https://sub.example/x"}`))
	})
	defer f.close()

	rec := doVlessUpload(f.srv, `{"count":0,"merge":true}`)
	if rec.Code != 200 {
		t.Fatalf("upload failed: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Count(f.gh.put, "@1.1.1.1:443") != 1 || strings.Contains(f.gh.put, "#old") {
		t.Fatalf("existing node should be replaced in place:\n%s", f.gh.put)
	}
	if !strings.Contains(f.gh.put, "9.9.9.9") || !strings.Contains(f.gh.put, "2.2.2.2") {
		t.Fatalf("merge lost nodes:\n%s", f.gh.put)
	}
	if strings.Count(f.gh.put, "vless://") != 3 {
		t.Fatalf("expected 3 unique nodes:\n%s", f.gh.put)
	}
	if !strings.Contains(f.gh.putMsg, "更新 VLESS 订阅 (2 个)(总共3节点)") {
		t.Fatalf("commit message missing total: %q", f.gh.putMsg)
	}
}

func TestVlessUploadRejectsEmptyResults(t *testing.T) {
	f := setupUploadTest(t, "", http.StatusOK, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"https://sub.example/x"}`))
	})
	defer f.close()
	f.srv.runner.results = []Result{{IP: "3.3.3.3", Port: 443, Speed: 1, SpeedTested: true, SpeedQualified: false}}

	rec := doVlessUpload(f.srv, `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d %s", rec.Code, rec.Body.String())
	}
	if f.gh.put != "" {
		t.Fatalf("must not PUT when there are no qualified nodes:\n%s", f.gh.put)
	}
}

func TestVlessUploadUsesCacheOnFetchFailure(t *testing.T) {
	f := setupUploadTest(t, "", http.StatusInternalServerError, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"https://sub.example/x"}`))
	})
	defer f.close()

	rec := doVlessUpload(f.srv, `{"count":0,"merge":true}`)
	if rec.Code != 200 {
		t.Fatalf("cache fallback failed: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"warning"`) || f.gh.put == "" {
		t.Fatalf("expected warning + upload, got %s\nput=%q", rec.Body.String(), f.gh.put)
	}
}

func TestVlessUploadSubscriptionFailure(t *testing.T) {
	f := setupUploadTest(t, "", http.StatusOK, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"manager down"}`))
	})
	defer f.close()

	rec := doVlessUpload(f.srv, `{"count":0,"merge":true}`)
	if rec.Code != 200 {
		t.Fatalf("upload should succeed: %d %s", rec.Code, rec.Body.String())
	}
	if f.gh.put == "" {
		t.Fatal("upload content missing")
	}
	if !strings.Contains(rec.Body.String(), `"subscription_error"`) || strings.Contains(rec.Body.String(), `"url":"`) {
		t.Fatalf("subscription failure must be reported separately: %s", rec.Body.String())
	}
}

func TestVlessSubscriptionEndpoint(t *testing.T) {
	f := setupUploadTest(t, "", http.StatusOK, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"https://sub.example/retry"}`))
	})
	defer f.close()

	req := httptest.NewRequest("POST", "/api/vless/subscription", strings.NewReader(`{"file":"vlesssub"}`))
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"url":"https://sub.example/retry"`) {
		t.Fatalf("subscription retry failed: %d %s", rec.Code, rec.Body.String())
	}
}
