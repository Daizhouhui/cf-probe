package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerDisablesStaleAssetAndResultCaching(t *testing.T) {
	server := NewServer()
	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/app.js", want: "no-cache"},
		{path: "/api/results", want: "no-store"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if got := recorder.Header().Get("Cache-Control"); got != tc.want {
				t.Fatalf("Cache-Control = %q, want %q", got, tc.want)
			}
		})
	}
}
