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

	"github.com/gin-gonic/gin"

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
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		store:     st,
		cache:     linkCache,
		publisher: publisher,
		baseURL:   strings.TrimRight(strings.TrimSpace(baseURL), "/"),
	}

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/", s.dashboard)
	router.GET("/favicon.ico", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET("/healthz", s.health)
	router.POST("/api/links", s.createLink)
	router.GET("/api/links", s.listLinks)
	router.GET("/api/analytics/:code", s.analytics)

	graphQLHandler := newGraphQLHandler(st)
	router.GET("/graphql", gin.WrapH(graphQLHandler))
	router.POST("/graphql", gin.WrapH(graphQLHandler))

	// Short links are handled by NoRoute so Gin does not have to mix a
	// root-level wildcard with the /api routes.
	router.NoRoute(s.redirect)

	return router
}

func (s *Server) dashboard(c *gin.Context) {
	page, err := dashboardFS.ReadFile("web/index.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "dashboard unavailable")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/html; charset=utf-8", page)
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) createLink(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)

	var request createLinkRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid JSON body")
		return
	}

	longURL, err := normalizeLongURL(request.LongURL)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	link, err := s.store.CreateLink(c.Request.Context(), longURL)
	if err != nil {
		log.Printf("create link: %v", err)
		writeError(c, http.StatusInternalServerError, "could not create link")
		return
	}
	s.cache.Set(link)

	c.JSON(http.StatusCreated, createLinkResponse{
		Code:      link.Code,
		ShortURL:  s.shortURL(c, link.Code),
		LongURL:   link.LongURL,
		CreatedAt: link.CreatedAt,
	})
}

func (s *Server) listLinks(c *gin.Context) {
	links, err := s.store.ListLinks(c.Request.Context())
	if err != nil {
		log.Printf("list links: %v", err)
		writeError(c, http.StatusInternalServerError, "could not load links")
		return
	}
	c.JSON(http.StatusOK, gin.H{"links": links})
}

func (s *Server) analytics(c *gin.Context) {
	code := c.Param("code")
	analytics, err := s.store.GetAnalytics(c.Request.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		writeError(c, http.StatusNotFound, "link not found")
		return
	}
	if err != nil {
		log.Printf("analytics: %v", err)
		writeError(c, http.StatusInternalServerError, "could not load analytics")
		return
	}
	c.JSON(http.StatusOK, analytics)
}

func (s *Server) redirect(c *gin.Context) {
	code := strings.TrimPrefix(c.Request.URL.Path, "/")
	if strings.Contains(code, "/") || !validCode(code) {
		writeError(c, http.StatusNotFound, "link not found")
		return
	}

	link, ok := s.cache.Get(code)
	if !ok {
		var err error
		link, err = s.store.GetLink(c.Request.Context(), code)
		if errors.Is(err, store.ErrNotFound) {
			writeError(c, http.StatusNotFound, "link not found")
			return
		}
		if err != nil {
			log.Printf("get link: %v", err)
			writeError(c, http.StatusInternalServerError, "could not load link")
			return
		}
		s.cache.Set(link)
	}

	if link.Disabled {
		writeError(c, http.StatusGone, "link disabled")
		return
	}

	// The redirect does not wait for analytics. Publishing to the queue keeps
	// the user-facing path fast, just like SQS would in the AWS version.
	s.publisher.Publish(store.ClickEvent{
		Code:      code,
		ClickedAt: time.Now().UTC(),
		Referrer:  truncate(c.Request.Referer(), 512),
		UserAgent: truncate(c.Request.UserAgent(), 512),
		IPHash:    hashIP(c.Request),
	})

	c.Redirect(http.StatusFound, link.LongURL)
}

func (s *Server) shortURL(c *gin.Context, code string) string {
	if s.baseURL != "" {
		return s.baseURL + "/" + code
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(c.Request.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = forwarded
	}
	return scheme + "://" + c.Request.Host + "/" + code
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

func writeError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}
