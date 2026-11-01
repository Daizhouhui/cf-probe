package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunCanSkipDelayAndMeasureDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("CF-RAY", "0000000000000000-TST")
		_, _ = w.Write(make([]byte, 128*1024))
	}))
	defer server.Close()

	_, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}

	var stages []string
	results, err := Run(context.Background(), Options{
		Proxy:      true,
		SkipDelay:  true,
		DisableDL:  true,
		Count:      1,
		SpeedLimit: 0,
		TestURL:    server.URL,
		IPText:     fmt.Sprintf("127.0.0.1:%d", port),
		DLTimeout:  1,
	}, func(p Progress) {
		if p.Stage != "" {
			stages = append(stages, p.Stage)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 candidate result, got %d", len(results))
	}
	if results[0].DelayTested || !results[0].SpeedTested {
		t.Fatalf("download-only result should have only speed tested: %+v", results[0])
	}
	for _, stage := range stages {
		if stage == "ping" {
			t.Fatalf("skip-delay run emitted a ping stage: %v", stages)
		}
	}
}
