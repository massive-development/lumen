package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
)

func handleRSSFeed(w http.ResponseWriter, r *http.Request) {
	feedURL := r.URL.Query().Get("url")
	if feedURL == "" {
		errJSON(w, http.StatusBadRequest, "url is required")
		return
	}
	limit := clampInt(r.URL.Query().Get("limit"), 10, 1, 30)

	fp := gofeed.NewParser()
	feed, err := fp.ParseURLWithContext(feedURL, r.Context())
	if err != nil {
		errJSON(w, http.StatusBadGateway, "failed to parse feed: "+err.Error())
		return
	}

	type item struct {
		Title     string `json:"title"`
		Link      string `json:"link"`
		Published string `json:"published"`
		Summary   string `json:"summary"`
	}
	items := []item{}
	for i, entry := range feed.Items {
		if i >= limit {
			break
		}
		pub := entry.Published
		if entry.PublishedParsed != nil {
			pub = entry.PublishedParsed.Format(time.RFC3339)
		}
		summary := stripHTML(entry.Description)
		if summary == "" {
			summary = stripHTML(entry.Content)
		}
		if len(summary) > 300 {
			summary = summary[:300] + "..."
		}
		items = append(items, item{entry.Title, entry.Link, pub, summary})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"feed_title": feed.Title,
		"items":      items,
	})
}

func handleHackerNews(w http.ResponseWriter, r *http.Request) {
	storyType := r.URL.Query().Get("type")
	if storyType == "" {
		storyType = "top"
	}
	limit := clampInt(r.URL.Query().Get("limit"), 10, 1, 30)
	ctx := r.Context()

	body, err := fetchURL(ctx, fmt.Sprintf("https://hacker-news.firebaseio.com/v0/%sstories.json", storyType))
	if err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	var ids []int
	if err := json.Unmarshal(body, &ids); err != nil {
		errJSON(w, http.StatusBadGateway, "invalid HN response")
		return
	}
	if len(ids) > limit {
		ids = ids[:limit]
	}

	type story struct {
		Title    string `json:"title"`
		URL      string `json:"url"`
		Score    int    `json:"score"`
		Comments int    `json:"comments"`
		By       string `json:"by"`
		Time     string `json:"time"`
	}
	stories := make([]story, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i, id int) {
			defer wg.Done()
			itemBody, err := fetchURL(ctx, fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", id))
			if err != nil {
				return
			}
			var item struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Score       int    `json:"score"`
				Descendants int    `json:"descendants"`
				By          string `json:"by"`
				Time        int64  `json:"time"`
			}
			if err := json.Unmarshal(itemBody, &item); err != nil {
				return
			}
			stories[i] = story{
				Title:    item.Title,
				URL:      item.URL,
				Score:    item.Score,
				Comments: item.Descendants,
				By:       item.By,
				Time:     time.Unix(item.Time, 0).Format(time.RFC3339),
			}
		}(i, id)
	}
	wg.Wait()

	out := []story{}
	for _, s := range stories {
		if s.Title != "" {
			out = append(out, s)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"stories": out})
}
