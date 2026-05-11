package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

func handleProfileGet(w http.ResponseWriter, r *http.Request) {
	uid := r.URL.Query().Get("user_id")
	if uid == "" {
		uid = "default"
	}
	body, err := fetchURL(r.Context(), memoryURL+"/profile?"+url.Values{"user_id": {uid}}.Encode())
	if err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func handleProfileUpdate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid json")
		return
	}
	if _, ok := body["user_id"]; !ok {
		body["user_id"] = "default"
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, memoryURL+"/profile", bytes.NewReader(data))
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
