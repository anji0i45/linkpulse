package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestFileStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "linkpulse.json")

	st, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	link, err := st.CreateLink(ctx, CreateLinkInput{LongURL: "https://example.com/a/long/path"})
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	if len(link.Code) != 7 {
		t.Fatalf("code length = %d, want 7", len(link.Code))
	}

	got, err := st.GetLink(ctx, link.Code)
	if err != nil {
		t.Fatalf("GetLink: %v", err)
	}
	if got.LongURL != link.LongURL {
		t.Fatalf("LongURL = %q, want %q", got.LongURL, link.LongURL)
	}

	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		err := st.RecordClick(ctx, ClickEvent{
			Code:      link.Code,
			ClickedAt: base.Add(time.Duration(i) * time.Hour),
			Referrer:  "https://news.ycombinator.com/item?id=1",
			UserAgent: "test-browser",
			IPHash:    fmt.Sprintf("visitor-%d", i%2),
		})
		if err != nil {
			t.Fatalf("RecordClick %d: %v", i, err)
		}
	}

	links, err := st.ListLinks(ctx)
	if err != nil {
		t.Fatalf("ListLinks: %v", err)
	}
	if len(links) != 1 || links[0].Clicks != 3 {
		t.Fatalf("links = %+v, want one link with 3 clicks", links)
	}

	analytics, err := st.GetAnalytics(ctx, link.Code)
	if err != nil {
		t.Fatalf("GetAnalytics: %v", err)
	}
	if analytics.TotalClicks != 3 {
		t.Fatalf("TotalClicks = %d, want 3", analytics.TotalClicks)
	}
	if analytics.UniqueClicks != 2 {
		t.Fatalf("UniqueClicks = %d, want 2", analytics.UniqueClicks)
	}
	if len(analytics.ClicksByDay) != 1 || analytics.ClicksByDay[0].Clicks != 3 {
		t.Fatalf("ClicksByDay = %+v, want one day with 3 clicks", analytics.ClicksByDay)
	}
	if len(analytics.TopReferrers) != 1 || analytics.TopReferrers[0].Name != "news.ycombinator.com" {
		t.Fatalf("TopReferrers = %+v", analytics.TopReferrers)
	}

	// A new store instance must read the saved JSON file.
	reloaded, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("reload NewFileStore: %v", err)
	}
	reloadedLinks, err := reloaded.ListLinks(ctx)
	if err != nil {
		t.Fatalf("reloaded ListLinks: %v", err)
	}
	if len(reloadedLinks) != 1 || reloadedLinks[0].Clicks != 3 {
		t.Fatalf("reloaded links = %+v, want one link with 3 clicks", reloadedLinks)
	}
}

func TestFileStoreCustomCodeUpdateDelete(t *testing.T) {
	ctx := context.Background()
	st, err := NewFileStore(filepath.Join(t.TempDir(), "linkpulse.json"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	expiresAt := time.Now().UTC().Add(time.Hour)
	link, err := st.CreateLink(ctx, CreateLinkInput{
		LongURL:   "https://example.com/custom",
		Code:      "summer-sale",
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateLink custom: %v", err)
	}
	if link.Code != "summer-sale" || link.ExpiresAt == nil {
		t.Fatalf("unexpected custom link: %+v", link)
	}

	if _, err := st.CreateLink(ctx, CreateLinkInput{
		LongURL: "https://example.com/duplicate",
		Code:    "summer-sale",
	}); !errors.Is(err, ErrCodeExists) {
		t.Fatalf("duplicate code error = %v, want ErrCodeExists", err)
	}

	updated, err := st.UpdateLink(ctx, link.Code, true)
	if err != nil {
		t.Fatalf("UpdateLink: %v", err)
	}
	if !updated.Disabled {
		t.Fatal("link should be disabled")
	}

	if err := st.RecordClick(ctx, ClickEvent{Code: link.Code, ClickedAt: time.Now().UTC(), IPHash: "x"}); err != nil {
		t.Fatalf("RecordClick: %v", err)
	}
	if err := st.DeleteLink(ctx, link.Code); err != nil {
		t.Fatalf("DeleteLink: %v", err)
	}
	if _, err := st.GetLink(ctx, link.Code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetLink after delete = %v, want ErrNotFound", err)
	}
	links, err := st.ListLinks(ctx)
	if err != nil {
		t.Fatalf("ListLinks: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("links after delete = %+v, want empty", links)
	}
}

func TestFileStoreNotFound(t *testing.T) {
	ctx := context.Background()
	st, err := NewFileStore(filepath.Join(t.TempDir(), "linkpulse.json"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	if _, err := st.GetLink(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("GetLink error = %v, want ErrNotFound", err)
	}
	if _, err := st.GetAnalytics(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("GetAnalytics error = %v, want ErrNotFound", err)
	}
	if err := st.RecordClick(ctx, ClickEvent{Code: "missing"}); err != ErrNotFound {
		t.Fatalf("RecordClick error = %v, want ErrNotFound", err)
	}
}
