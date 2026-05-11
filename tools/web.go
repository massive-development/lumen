package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

func handleWebFetch(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		errJSON(w, http.StatusBadRequest, "url is required")
		return
	}
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid url")
		return
	}
	body, err := fetchURL(r.Context(), rawURL)
	if err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	content := stripHTML(string(body))
	if len(content) > 8000 {
		content = content[:8000] + "\n\n[content truncated]"
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": rawURL, "content": content})
}

func handleWebSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		errJSON(w, http.StatusBadRequest, "q is required")
		return
	}
	limit := clampInt(r.URL.Query().Get("limit"), 5, 1, 10)

	apiURL := "https://api.duckduckgo.com/?q=" + url.QueryEscape(q) + "&format=json&no_redirect=1&no_html=1&skip_disambig=1"
	body, err := fetchURL(r.Context(), apiURL)
	if err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	var ddg struct {
		Abstract      string `json:"Abstract"`
		AbstractURL   string `json:"AbstractURL"`
		RelatedTopics []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"RelatedTopics"`
		Results []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"Results"`
	}
	if err := json.Unmarshal(body, &ddg); err != nil {
		errJSON(w, http.StatusBadGateway, "failed to parse DDG response")
		return
	}

	type result struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	}
	results := []result{}
	for _, rt := range ddg.RelatedTopics {
		if len(results) >= limit {
			break
		}
		if rt.FirstURL != "" {
			results = append(results, result{rt.Text, rt.FirstURL})
		}
	}
	for _, res := range ddg.Results {
		if len(results) >= limit {
			break
		}
		if res.FirstURL != "" {
			results = append(results, result{res.Text, res.FirstURL})
		}
	}
	relTopics := []string{}
	for i, rt := range ddg.RelatedTopics {
		if i >= 5 {
			break
		}
		if rt.Text != "" {
			relTopics = append(relTopics, rt.Text)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"abstract":       ddg.Abstract,
		"abstract_url":   ddg.AbstractURL,
		"results":        results,
		"related_topics": relTopics,
	})
}

func handleWikipedia(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		errJSON(w, http.StatusBadRequest, "q is required")
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "en"
	}
	apiURL := fmt.Sprintf("https://%s.wikipedia.org/api/rest_v1/page/summary/%s", lang, url.PathEscape(q))
	body, err := fetchURL(r.Context(), apiURL)
	if err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	var wiki struct {
		Title       string `json:"title"`
		Extract     string `json:"extract"`
		ContentURLs struct {
			Desktop struct{ Page string `json:"page"` } `json:"desktop"`
		} `json:"content_urls"`
	}
	if err := json.Unmarshal(body, &wiki); err != nil {
		errJSON(w, http.StatusBadGateway, "failed to parse Wikipedia response")
		return
	}
	extract := wiki.Extract
	if len(extract) > 2000 {
		extract = extract[:2000] + "..."
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"title":   wiki.Title,
		"summary": extract,
		"url":     wiki.ContentURLs.Desktop.Page,
	})
}

// clampInt parses s as an int, returns def if empty or invalid, clamped to [min, max].
func clampInt(s string, def, min, max int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
