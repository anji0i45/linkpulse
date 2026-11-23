package api

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"linkpulse/internal/cache"
	"linkpulse/internal/queue"
	"linkpulse/internal/store"
)

//go:embed web/index.html
var dashboardFS embed.FS

type Server struct {
	store     store.Store
	cache     cache.LinkCache
	publisher queue.Publisher
	baseURL   string
}

type createLinkRequest struct {
	LongURL string `json:"long_url"`
}

type createLinkResponse struct {
	Code      string    `json:"code"`
	ShortURL  string    `json:"short_url"`
	LongURL   string    `json:"long_url"`
	CreatedAt time.Time `json:"created_at"`
}

func NewServer(st store.Store, linkCache cache.LinkCache, publisher queue.Publisher, baseURL string) http.Handler {
	s := &Server{
		store:     st,
		cache:     linkCache,
		publisher: publisher,
		baseURL:   strings.TrimRight(strings.TrimSpace(baseURL), "/"),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.dashboard)
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /api/links", s.createLink)
	mux.HandleFunc("GET /api/links", s.listLinks)
	mux.HandleFunc("GET /api/analytics/{code}", s.analytics)
	mux.HandleFunc("GET /{code}", s.redirect)

	return withLogging(mux)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	page, err := dashboardFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "dashboard unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(page)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var request createLinkRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	longURL, err := normalizeLongURL(request.LongURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	link, err := s.store.CreateLink(r.Context(), longURL)
	if err != nil {
		log.Printf("create link: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create link")
		return
	}
	s.cache.Set(link)

	writeJSON(w, http.StatusCreated, createLinkResponse{
		Code:      link.Code,
		ShortURL:  s.shortURL(r, link.Code),
		LongURL:   link.LongURL,
		CreatedAt: link.CreatedAt,
	})
}

func (s *Server) listLinks(w http.ResponseWriter, r *http.Request) {
	links, err := s.store.ListLinks(r.Context())
	if err != nil {
		log.Printf("list links: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load links")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": links})
}

func (s *Server) analytics(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	analytics, err := s.store.GetAnalytics(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "link not found")
		return
	}
	if err != nil {
		log.Printf("analytics: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load analytics")
		return
	}
	writeJSON(w, http.StatusOK, analytics)
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if !validCode(code) {
		http.NotFound(w, r)
		return
	}

	link, ok := s.cache.Get(code)
	if !ok {
		var err error
		link, err = s.store.GetLink(r.Context(), code)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("get link: %v", err)
			http.Error(w, "could not load link", http.StatusInternalServerError)
			return
		}
		s.cache.Set(link)
	}

	if link.Disabled {
		http.Error(w, "link disabled", http.StatusGone)
		return
	}

	// The redirect does not wait for analytics. Publishing to the queue keeps
	// the user-facing path fast, just like SQS would in the AWS version.
	s.publisher.Publish(store.ClickEvent{
		Code:      code,
		ClickedAt: time.Now().UTC(),
		Referrer:  truncate(r.Referer(), 512),
		UserAgent: truncate(r.UserAgent(), 512),
		IPHash:    hashIP(r),
	})

	http.Redirect(w, r, link.LongURL, http.StatusFound)
}

func (s *Server) shortURL(r *http.Request, code string) string {
	if s.baseURL != "" {
		return s.baseURL + "/" + code
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = forwarded
	}
	return scheme + "://" + r.Host + "/" + code
}

func normalizeLongURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("long_url is required")
	}
	if len(raw) > 2048 {
		return "", errors.New("long_url is too long")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("long_url is not a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("only http and https URLs are allowed")
	}
	if parsed.Host == "" {
		return "", errors.New("long_url must include a host")
	}
	return parsed.String(), nil
}

func validCode(code string) bool {
	if len(code) < 4 || len(code) > 12 {
		return false
	}
	for _, r := range code {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func hashIP(r *http.Request) string {
	host := r.RemoteAddr
	if parsed, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = parsed
	}
	sum := sha256.Sum256([]byte(host))
	return hex.EncodeToString(sum[:])[:16]
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
