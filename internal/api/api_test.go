package api

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestGraphQLAnalytics(t *testing.T) {
	handler, st, _ := newTestServer(t)

	link, err := st.CreateLink(context.Background(), store.CreateLinkInput{LongURL: "https://example.com/graphql"})
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	if err := st.RecordClick(context.Background(), store.ClickEvent{
		Code:      link.Code,
		ClickedAt: time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC),
		Referrer:  "https://google.com/search",
		UserAgent: "test",
		IPHash:    "visitor-1",
	}); err != nil {
		t.Fatalf("RecordClick: %v", err)
	}

	query := fmt.Sprintf(`{
		links { code longUrl clicks }
		analytics(code: %q) {
			totalClicks
			uniqueClicks
			clicksByDay { day clicks }
			topReferrers { name clicks }
		}
	}`, link.Code)
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatalf("marshal query: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("graphql status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	response := recorder.Body.String()
	for _, want := range []string{link.Code, "totalClicks", "uniqueClicks", "google.com"} {
		if !strings.Contains(response, want) {
			t.Fatalf("graphql response missing %q: %s", want, response)
		}
	}
}

func TestCustomCodeExpiryAndLifecycle(t *testing.T) {
	handler, _, _ := newTestServer(t)

	expiresAt := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	createBody := fmt.Sprintf(`{"long_url":"https://example.com/sale","custom_code":"winter-sale","expires_at":%q}`, expiresAt)
	createRequest := httptest.NewRequest(http.MethodPost, "/api/links", strings.NewReader(createBody))
	createRequest.Header.Set("Content-Type", "application/json")
	createRecorder := httptest.NewRecorder()
	handler.ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRecorder.Code, createRecorder.Body.String())
	}
	if !strings.Contains(createRecorder.Body.String(), `"code":"winter-sale"`) {
		t.Fatalf("create body = %s", createRecorder.Body.String())
	}

	duplicateRequest := httptest.NewRequest(http.MethodPost, "/api/links", strings.NewReader(createBody))
	duplicateRequest.Header.Set("Content-Type", "application/json")
	duplicateRecorder := httptest.NewRecorder()
	handler.ServeHTTP(duplicateRecorder, duplicateRequest)
	if duplicateRecorder.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409", duplicateRecorder.Code)
	}

	redirect := httptest.NewRecorder()
	handler.ServeHTTP(redirect, httptest.NewRequest(http.MethodGet, "/winter-sale", nil))
	if redirect.Code != http.StatusFound {
		t.Fatalf("redirect status = %d", redirect.Code)
	}

	qr := httptest.NewRecorder()
	handler.ServeHTTP(qr, httptest.NewRequest(http.MethodGet, "/api/qr/winter-sale", nil))
	if qr.Code != http.StatusOK {
		t.Fatalf("qr status = %d", qr.Code)
	}
	if got := qr.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("qr content type = %q", got)
	}

	disable := httptest.NewRecorder()
	disableRequest := httptest.NewRequest(http.MethodPatch, "/api/links/winter-sale", strings.NewReader(`{"disabled":true}`))
	disableRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(disable, disableRequest)
	if disable.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", disable.Code, disable.Body.String())
	}
	disabledRedirect := httptest.NewRecorder()
	handler.ServeHTTP(disabledRedirect, httptest.NewRequest(http.MethodGet, "/winter-sale", nil))
	if disabledRedirect.Code != http.StatusGone {
		t.Fatalf("disabled redirect status = %d, want 410", disabledRedirect.Code)
	}

	enable := httptest.NewRecorder()
	enableRequest := httptest.NewRequest(http.MethodPatch, "/api/links/winter-sale", strings.NewReader(`{"disabled":false}`))
	enableRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(enable, enableRequest)
	if enable.Code != http.StatusOK {
		t.Fatalf("enable status = %d", enable.Code)
	}

	deleteRecorder := httptest.NewRecorder()
	handler.ServeHTTP(deleteRecorder, httptest.NewRequest(http.MethodDelete, "/api/links/winter-sale", nil))
	if deleteRecorder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", deleteRecorder.Code)
	}
	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/winter-sale", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("deleted redirect status = %d, want 404", missing.Code)
	}
}

func TestExpiredLinkRedirect(t *testing.T) {
	handler, st, _ := newTestServer(t)
	past := time.Now().UTC().Add(-time.Hour)
	if _, err := st.CreateLink(context.Background(), store.CreateLinkInput{
		LongURL:   "https://example.com/expired",
		Code:      "expired-code",
		ExpiresAt: &past,
	}); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/expired-code", nil))
	if recorder.Code != http.StatusGone {
		t.Fatalf("expired redirect status = %d, want 410", recorder.Code)
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

	appJS := httptest.NewRecorder()
	handler.ServeHTTP(appJS, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	if appJS.Code != http.StatusOK {
		t.Fatalf("app.js status = %d", appJS.Code)
	}
	if !strings.Contains(appJS.Body.String(), "search-input") {
		t.Fatal("app.js is missing the search UI")
	}
}
