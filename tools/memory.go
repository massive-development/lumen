package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

func handleMemorySearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "5"
	}
	params := url.Values{"limit": {limit}}
	if q != "" {
		params.Set("q", q)
	}
	body, err := fetchURL(r.Context(), memoryURL+"/memories?"+params.Encode())
	if err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func handleMemorySave(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid json")
		return
	}
	if _, ok := body["content"]; !ok {
		errJSON(w, http.StatusBadRequest, "content required")
		return
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, memoryURL+"/memories", bytes.NewReader(data))
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}
