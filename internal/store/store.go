package store

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a short code does not exist.
var ErrNotFound = errors.New("link not found")

// Link is one short link created by a user.
type Link struct {
	Code      string    `json:"code"`
	LongURL   string    `json:"long_url"`
	CreatedAt time.Time `json:"created_at"`
	Disabled  bool      `json:"disabled"`
}

// ClickEvent is one redirect that happened through a short link.
type ClickEvent struct {
	Code      string    `json:"code"`
	ClickedAt time.Time `json:"clicked_at"`
	Referrer  string    `json:"referrer"`
	UserAgent string    `json:"user_agent"`
	IPHash    string    `json:"ip_hash"`
}

// LinkSummary is a link plus its total click count.
type LinkSummary struct {
	Link
	Clicks int `json:"clicks"`
}

// DayCount is the number of clicks on one calendar day.
type DayCount struct {
	Day    string `json:"day"`
	Clicks int    `json:"clicks"`
}

// NameCount is a count grouped by a string, such as a referrer.
type NameCount struct {
	Name   string `json:"name"`
	Clicks int    `json:"clicks"`
}

// Analytics is the read model used by the dashboard.
type Analytics struct {
	Code         string      `json:"code"`
	LongURL      string      `json:"long_url"`
	TotalClicks  int         `json:"total_clicks"`
	UniqueClicks int         `json:"unique_clicks"`
	ClicksByDay  []DayCount  `json:"clicks_by_day"`
	TopReferrers []NameCount `json:"top_referrers"`
}

// Store is the persistence boundary. The local version uses a JSON file.
// The AWS version can implement the same interface with DynamoDB and MongoDB.
type Store interface {
	CreateLink(ctx context.Context, longURL string) (Link, error)
	GetLink(ctx context.Context, code string) (Link, error)
	RecordClick(ctx context.Context, event ClickEvent) error
	ListLinks(ctx context.Context) ([]LinkSummary, error)
	GetAnalytics(ctx context.Context, code string) (Analytics, error)
}
