package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestSubscription(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"url":"https://sub.example/x"}`))
	}))
	defer srv.Close()

	got, err := requestSubscription(context.Background(), srv.URL, "/api/subscription-link", "tok", "vlesssub")
	if err != nil || got != "https://sub.example/x" {
		t.Fatalf("got %q, %v", got, err)
	}
	if gotPath != "/api/subscription-link" || gotAuth != "Bearer tok" || gotBody["file"] != "vlesssub" {
		t.Fatalf("request wrong: path=%q auth=%q body=%+v", gotPath, gotAuth, gotBody)
	}
}

func TestRequestSubscriptionError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"no access"}`))
	}))
	defer srv.Close()
	_, err := requestSubscription(context.Background(), srv.URL, "/api/subscription-link", "tok", "vlesssub")
	if err == nil || !strings.Contains(err.Error(), "no access") {
		t.Fatalf("want error with server message, got %v", err)
	}
}
