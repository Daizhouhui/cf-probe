package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGithubGetFile(t *testing.T) {
	var gotAuth, gotPath, gotRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotRef = r.Header.Get("Authorization"), r.URL.Path, r.URL.Query().Get("ref")
		_, _ = w.Write([]byte(`{"content":"aGVsbG8=","sha":"abc","encoding":"base64"}`))
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	data, sha, err := githubGetFile(context.Background(), "owner/repo", "main", "sub/a.txt", "tok")
	if err != nil || string(data) != "hello" || sha != "abc" {
		t.Fatalf("got data=%q sha=%q err=%v", data, sha, err)
	}
	if gotAuth != "Bearer tok" || gotPath != "/repos/owner/repo/contents/sub/a.txt" || gotRef != "main" {
		t.Fatalf("request wrong: auth=%q path=%q ref=%q", gotAuth, gotPath, gotRef)
	}
}

func TestGithubGetFileNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	data, sha, err := githubGetFile(context.Background(), "owner/repo", "main", "nope.txt", "tok")
	if err != nil || data != nil || sha != "" {
		t.Fatalf("404 should be (nil, \"\", nil), got data=%q sha=%q err=%v", data, sha, err)
	}
}

func TestGithubPutFileSendsShaAndContent(t *testing.T) {
	var method, path string
	var body map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"content":"b2xk","sha":"sha1","encoding":"base64"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	if err := githubPutFile(context.Background(), "owner/repo", "main", "a.txt", "tok", "msg", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPut || path != "/repos/owner/repo/contents/a.txt" {
		t.Fatalf("wrong request: %s %s", method, path)
	}
	if body["sha"] != "sha1" || body["branch"] != "main" || body["message"] != "msg" ||
		body["content"] != base64.StdEncoding.EncodeToString([]byte("new")) {
		t.Fatalf("payload wrong: %+v", body)
	}
}

func TestVerifyGitHubToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer good" {
			_, _ = w.Write([]byte(`{"login":"u"}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	if err := verifyGitHubToken(context.Background(), "good"); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if err := verifyGitHubToken(context.Background(), "bad"); err == nil {
		t.Fatal("invalid token must fail")
	}
}
