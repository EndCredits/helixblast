package api

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/EndCredits/helixblast/embed"
)

func serveFrontendTest(t *testing.T, target string) *http.Response {
	t.Helper()
	sub, err := fs.Sub(embedded.Frontend, ".")
	if err != nil {
		t.Fatalf("fs.Sub: %v", err)
	}
	srv := httptest.NewServer(serveFrontend(sub))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + target)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestServeFrontendFallback(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantCode int
		wantHTML bool
	}{
		{"root serves index", "/", 200, true},
		{"client route falls back", "/settings", 200, true},
		{"client route with trailing slash", "/settings/", 200, true},
		{"unknown extensionless path falls back", "/some/future/route", 200, true},
		{"missing asset keeps 404", "/assets/definitely-missing.js", 404, false},
		{"missing extensioned file keeps 404", "/favicon.ico", 404, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := serveFrontendTest(t, tt.path)
			if resp.StatusCode != tt.wantCode {
				t.Fatalf("GET %s = %d, want %d", tt.path, resp.StatusCode, tt.wantCode)
			}
			ct := resp.Header.Get("Content-Type")
			isHTML := strings.Contains(ct, "text/html")
			if isHTML != tt.wantHTML {
				t.Fatalf("GET %s content-type %s, wantHTML=%v", tt.path, ct, tt.wantHTML)
			}
			if tt.wantHTML {
				body, _ := io.ReadAll(resp.Body)
				if !strings.Contains(string(body), `id="root"`) {
					t.Errorf("fallback response is not the app shell")
				}
			}
		})
	}
}
