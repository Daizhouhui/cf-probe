// Package androidlib wraps the cf-probe speed test service as a mobile entry
// point for gomobile bind. The Android shell calls three functions:
// Start boots the local server and returns the port, Stop shuts it down,
// Version reports the version string.
//
// NOTE: doc comments in this package are copied verbatim by gobind into
// generated Java sources, which plain javac reads with the platform default
// charset (GBK on Chinese Windows). Keep them ASCII to keep the bind green.
package androidlib

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DaiZhouHui/cf-probe/internal/app"
)

var (
	mu       sync.Mutex
	httpSrv  *http.Server
	listener net.Listener
	port     int
)

// Start boots the local speed test server on a random 127.0.0.1 port and
// returns that port. dataDir is a writable app-private directory (the
// Android shell passes filesDir); results, config and the IP range cache
// all live there. YX_DATA_DIR must be set before the first DataDir call:
// paths.go resolves the data directory exactly once.
func Start(dataDir string) (int, error) {
	mu.Lock()
	defer mu.Unlock()
	if httpSrv != nil && listener != nil {
		return port, nil
	}
	abs, err := filepath.Abs(dataDir)
	if err == nil {
		dataDir = abs
	}
	if dataDir != "" {
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			return 0, fmt.Errorf("create data dir: %w", err)
		}
		os.Setenv("YX_DATA_DIR", dataDir)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	s := &http.Server{
		Handler:           app.NewServer(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	httpSrv = s
	listener = ln
	port = ln.Addr().(*net.TCPAddr).Port
	go func() {
		// Serve returning ErrServerClosed is the normal shutdown path;
		// the shell controls the server lifetime via Start/Stop.
		_ = s.Serve(ln)
	}()
	return port, nil
}

// Stop shuts the local server down; a running speed test is cancelled
// through its context and wraps up gracefully.
func Stop() {
	mu.Lock()
	s := httpSrv
	httpSrv = nil
	listener = nil
	port = 0
	mu.Unlock()
	if s == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.Shutdown(ctx)
}

// Version returns the version string.
func Version() string {
	return "1.0.6-android"
}
