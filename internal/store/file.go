package store

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const codeAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// FileStore is a simple, zero-dependency local store. It keeps all links and
// click events in one JSON file. This is intentionally small for the local MVP.
// The Store interface is the seam for a DynamoDB/MongoDB implementation later.
type FileStore struct {
	mu   sync.RWMutex
	path string
	data fileData
}

type fileData struct {
	Links  map[string]Link `json:"links"`
	Clicks []ClickEvent    `json:"clicks"`
}

// NewFileStore opens or creates the JSON database at path.
func NewFileStore(path string) (*FileStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("data path is empty")
	}

	s := &FileStore{
		path: path,
		data: fileData{Links: make(map[string]Link)},
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read data file: %w", err)
	}
	if len(raw) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("decode data file: %w", err)
	}
	if s.data.Links == nil {
		s.data.Links = make(map[string]Link)
	}

	return s, nil
}

func (s *FileStore) CreateLink(ctx context.Context, longURL string) (Link, error) {
	if err := ctx.Err(); err != nil {
		return Link{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for attempt := 0; attempt < 10; attempt++ {
		code, err := randomCode(7)
		if err != nil {
			return Link{}, err
		}
		if _, exists := s.data.Links[code]; exists {
			continue
		}

		link := Link{
			Code:      code,
			LongURL:   longURL,
			CreatedAt: time.Now().UTC(),
		}
		s.data.Links[code] = link
		if err := s.saveLocked(); err != nil {
			delete(s.data.Links, code)
			return Link{}, err
		}
		return link, nil
	}

	return Link{}, errors.New("could not generate a unique short code")
}

func (s *FileStore) GetLink(ctx context.Context, code string) (Link, error) {
	if err := ctx.Err(); err != nil {
		return Link{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	link, ok := s.data.Links[code]
	if !ok {
		return Link{}, ErrNotFound
	}
	return link, nil
}

func (s *FileStore) RecordClick(ctx context.Context, event ClickEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.data.Links[event.Code]; !ok {
		return ErrNotFound
	}
	if event.ClickedAt.IsZero() {
		event.ClickedAt = time.Now().UTC()
	}
	s.data.Clicks = append(s.data.Clicks, event)
	return s.saveLocked()
}

func (s *FileStore) ListLinks(ctx context.Context) ([]LinkSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	clicks := make(map[string]int, len(s.data.Links))
	for _, event := range s.data.Clicks {
		clicks[event.Code]++
	}

	links := make([]LinkSummary, 0, len(s.data.Links))
	for _, link := range s.data.Links {
		links = append(links, LinkSummary{Link: link, Clicks: clicks[link.Code]})
	}
	sort.Slice(links, func(i, j int) bool {
		return links[i].CreatedAt.After(links[j].CreatedAt)
	})
	return links, nil
}

func (s *FileStore) GetAnalytics(ctx context.Context, code string) (Analytics, error) {
	if err := ctx.Err(); err != nil {
		return Analytics{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	link, ok := s.data.Links[code]
	if !ok {
		return Analytics{}, ErrNotFound
	}

	byDay := make(map[string]int)
	byReferrer := make(map[string]int)
	uniqueVisitors := make(map[string]struct{})
	total := 0

	for _, event := range s.data.Clicks {
		if event.Code != code {
			continue
		}
		total++
		byDay[event.ClickedAt.UTC().Format("2006-01-02")]++

		referrer := strings.TrimSpace(event.Referrer)
		if referrer == "" {
			referrer = "direct"
		}
		if parsed := referrerHost(referrer); parsed != "" {
			referrer = parsed
		}
		byReferrer[referrer]++

		if event.IPHash != "" {
			uniqueVisitors[event.IPHash] = struct{}{}
		}
	}

	days := make([]DayCount, 0, len(byDay))
	for day, count := range byDay {
		days = append(days, DayCount{Day: day, Clicks: count})
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Day < days[j].Day })

	referrers := make([]NameCount, 0, len(byReferrer))
	for name, count := range byReferrer {
		referrers = append(referrers, NameCount{Name: name, Clicks: count})
	}
	sort.Slice(referrers, func(i, j int) bool {
		if referrers[i].Clicks == referrers[j].Clicks {
			return referrers[i].Name < referrers[j].Name
		}
		return referrers[i].Clicks > referrers[j].Clicks
	})
	if len(referrers) > 5 {
		referrers = referrers[:5]
	}

	return Analytics{
		Code:         link.Code,
		LongURL:      link.LongURL,
		TotalClicks:  total,
		UniqueClicks: len(uniqueVisitors),
		ClicksByDay:  days,
		TopReferrers: referrers,
	}, nil
}

func (s *FileStore) saveLocked() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode data: %w", err)
	}

	// This local MVP writes the whole file. A real database would not do this.
	// Keeping the save inside the same mutex prevents overlapping writes.
	if err := os.WriteFile(s.path, raw, 0o644); err != nil {
		return fmt.Errorf("write data file: %w", err)
	}
	return nil
}

func randomCode(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}
	for i := range buf {
		buf[i] = codeAlphabet[int(buf[i])%len(codeAlphabet)]
	}
	return string(buf), nil
}

func referrerHost(referrer string) string {
	referrer = strings.TrimSpace(referrer)
	if referrer == "" {
		return ""
	}
	// Avoid importing net/url just for a display label; keep origin-ish text.
	referrer = strings.TrimPrefix(referrer, "https://")
	referrer = strings.TrimPrefix(referrer, "http://")
	referrer = strings.TrimPrefix(referrer, "www.")
	if i := strings.IndexAny(referrer, "/?#"); i >= 0 {
		referrer = referrer[:i]
	}
	if referrer == "" {
		return "direct"
	}
	return referrer
}
