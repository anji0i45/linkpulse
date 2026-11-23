package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"linkpulse/internal/cache"
	"linkpulse/internal/store"
)

type capturePublisher struct {
	mu     sync.Mutex
	events []store.ClickEvent
}

func (p *capturePublisher) Publish(event store.ClickEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
}

func (p *capturePublisher) last() (store.ClickEvent, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.events) == 0 {
		return store.ClickEvent{}, false
	}
	return p.events[len(p.events)-1], true
}

func newTestServer(t *testing.T) (http.Handler, *store.FileStore, *capturePublisher) {
	t.Helper()
	st, err := store.NewFileStore(filepath.Join(t.TempDir(), "linkpulse.json"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	publisher := &capturePublisher{}
	handler := NewServer(st, cache.NewMemory(time.Minute), publisher, "https://short.test")
	return handler, st, publisher
}

func TestCreateListRedirectAndAnalytics(t *testing.T) {
	handler, st, publisher := newTestServer(t)

	createBody := `{"long_url":"example.com/a/long/path"}`
	createRequest := httptest.NewRequest(http.MethodPost, "/api/links", strings.NewReader(createBody))
	createRequest.Header.Set("Content-Type", "application/json")
	createRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createRecorder, createRequest)

	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRecorder.Code, createRecorder.Body.String())
	}

	var created struct {
		Code     string `json:"code"`
		ShortURL string `json:"short_url"`
		LongURL  string `json:"long_url"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Code == "" || created.ShortURL != "https://short.test/"+created.Code {
		t.Fatalf("unexpected create response: %+v", created)
	}
	if created.LongURL != "https://example.com/a/long/path" {
		t.Fatalf("normalized URL = %q", created.LongURL)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/links", nil)
	listRecorder := httptest.NewRecorder()
	handler.ServeHTTP(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status = %d", listRecorder.Code)
	}
	if !strings.Contains(listRecorder.Body.String(), created.Code) {
		t.Fatalf("list body does not contain code: %s", listRecorder.Body.String())
	}

	redirectRequest := httptest.NewRequest(http.MethodGet, "/"+created.Code, nil)
	redirectRequest.Header.Set("Referer", "https://news.ycombinator.com/item?id=1")
	redirectRecorder := httptest.NewRecorder()
	handler.ServeHTTP(redirectRecorder, redirectRequest)
	if redirectRecorder.Code != http.StatusFound {
		t.Fatalf("redirect status = %d", redirectRecorder.Code)
	}
	if got := redirectRecorder.Header().Get("Location"); got != created.LongURL {
		t.Fatalf("redirect Location = %q, want %q", got, created.LongURL)
	}

	event, ok := publisher.last()
	if !ok {
		t.Fatal("redirect did not publish a click event")
	}
	if event.Code != created.Code || event.Referrer == "" || event.IPHash == "" {
		t.Fatalf("unexpected click event: %+v", event)
	}

	if err := st.RecordClick(context.Background(), event); err != nil {
		t.Fatalf("RecordClick: %v", err)
	}

	analyticsRequest := httptest.NewRequest(http.MethodGet, "/api/analytics/"+created.Code, nil)
	analyticsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(analyticsRecorder, analyticsRequest)
	if analyticsRecorder.Code != http.StatusOK {
		t.Fatalf("analytics status = %d, body = %s", analyticsRecorder.Code, analyticsRecorder.Body.String())
	}
	if !strings.Contains(analyticsRecorder.Body.String(), `"total_clicks":1`) {
		t.Fatalf("analytics body = %s", analyticsRecorder.Body.String())
	}
	if !strings.Contains(analyticsRecorder.Body.String(), "news.ycombinator.com") {
		t.Fatalf("analytics body missing referrer: %s", analyticsRecorder.Body.String())
	}
}

func TestInvalidCreateRequest(t *testing.T) {
	handler, _, _ := newTestServer(t)

	for _, body := range []string{
		`{"long_url":""}`,
		`{"long_url":"not a url with spaces"}`,
		`{"long_url":"ftp://example.com/file"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/links", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %q status = %d, want 400", body, recorder.Code)
		}
	}
}

func TestNotFoundAndDashboard(t *testing.T) {
	handler, _, _ := newTestServer(t)

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/doesnotexist", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", missing.Code)
	}

	analyticsMissing := httptest.NewRecorder()
	handler.ServeHTTP(analyticsMissing, httptest.NewRequest(http.MethodGet, "/api/analytics/doesnotexist", nil))
	if analyticsMissing.Code != http.StatusNotFound {
		t.Fatalf("missing analytics status = %d", analyticsMissing.Code)
	}

	dashboard := httptest.NewRecorder()
	handler.ServeHTTP(dashboard, httptest.NewRequest(http.MethodGet, "/", nil))
	if dashboard.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d", dashboard.Code)
	}
	if !strings.Contains(dashboard.Body.String(), "LinkPulse") {
		t.Fatal("dashboard is missing LinkPulse")
	}
}
