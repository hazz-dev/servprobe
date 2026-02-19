package dashboard_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hazz-dev/servprobe/internal/dashboard"
)

func TestHandler_ServesIndexHTML(t *testing.T) {
	h := dashboard.Handler()
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected Content-Type text/html, got %q", ct)
	}
	if !strings.Contains(w.Body.String(), "servprobe") {
		t.Error("expected index.html to contain 'servprobe'")
	}
}

func TestHandler_ServesCSS(t *testing.T) {
	h := dashboard.Handler()
	req := httptest.NewRequest("GET", "/style.css", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for style.css, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/css") {
		t.Errorf("expected Content-Type text/css, got %q", ct)
	}
}

func TestHandler_ServesJS(t *testing.T) {
	h := dashboard.Handler()
	req := httptest.NewRequest("GET", "/app.js", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for app.js, got %d", w.Code)
	}
}

func TestHandler_NotFound(t *testing.T) {
	h := dashboard.Handler()
	req := httptest.NewRequest("GET", "/does-not-exist.xyz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing asset, got %d", w.Code)
	}
}
