package app

import (
	"net/http"
	"net/http/httptest"
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
