// Anthropic Messages API → OpenAI chat/completions proxy.
// All unknown Anthropic fields (context_management, betas, metadata, etc.) are
// silently dropped because we reconstruct requests from scratch using only the
// fields llama-server understands.
//
// Augments every request with:
//   - Per-user profile fetched from the memory service
//   - Relevant memories fetched from the memory service (keyword-scored)
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultModel = "bitnet-b1.58-2b-4t"

var backend = envOr("BACKEND_URL", "http://bitnet-cpu:8081")
var memoryURL = envOr("MEMORY_URL", "http://memory:6000")

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// httpClient has no timeout — model inference on CPU can take many minutes.
var httpClient = &http.Client{}

// memClient times out quickly so a slow memory service never blocks inference.
var memClient = &http.Client{Timeout: 500 * time.Millisecond}

// ── Personalization ───────────────────────────────────────────────────────────

type Profile struct {
	Name       string `json:"name"`
	Style      string `json:"style"`
	Background string `json:"background"`
	Context    string `json:"context"`
}

func fetchProfile(userID string) Profile {
	params := url.Values{"user_id": {userID}}
	resp, err := memClient.Get(memoryURL + "/profile?" + params.Encode())
	if err != nil {
		return Profile{}
	}
	defer resp.Body.Close()
	var p Profile
	json.NewDecoder(resp.Body).Decode(&p)
	return p
}

// ── Memory client ─────────────────────────────────────────────────────────────

type Memory struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Category string `json:"category"`
}

func fetchMemories(query, userID string) []Memory {
	// Cap length to stay well within reverse-proxy URL limits (~8 KB).
	if len(query) > 512 {
		query = query[:512]
	}
	params := url.Values{"q": {query}, "limit": {"6"}, "user_id": {userID}}
	resp, err := memClient.Get(memoryURL + "/memories?" + params.Encode())
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var memories []Memory
	json.NewDecoder(resp.Body).Decode(&memories)
	return memories
}

// ── Preamble builder ──────────────────────────────────────────────────────────

func buildPreamble(query, userID string) string {
	var sb strings.Builder

	p := fetchProfile(userID)
	if p.Name != "" || p.Background != "" || p.Style != "" || p.Context != "" {
		sb.WriteString("## User profile\n")
		if p.Name != "" {
			fmt.Fprintf(&sb, "Name: %s\n", p.Name)
		}
		if p.Background != "" {
			fmt.Fprintf(&sb, "Background: %s\n", p.Background)
		}
		if p.Style != "" {
			fmt.Fprintf(&sb, "Communication style: %s\n", p.Style)
		}
		if p.Context != "" {
			fmt.Fprintf(&sb, "Context: %s\n", p.Context)
		}
	}

	memories := fetchMemories(query, userID)
	if len(memories) > 0 {
		sb.WriteString("\n## Memories\n")
		for _, m := range memories {
			fmt.Fprintf(&sb, "[%s] %s\n", m.Category, m.Content)
		}
	}

	if sb.Len() == 0 {
		return ""
	}
	sb.WriteString("\n---\n")
	return sb.String()
}

// ── Anthropic request types ───────────────────────────────────────────────────

type anthropicReq struct {
	Model       string          `json:"model"`
	System      json.RawMessage `json:"system"`
	Messages    []anthropicMsg  `json:"messages"`
	MaxTokens   int             `json:"max_tokens"`
	Stream      bool            `json:"stream"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	Metadata    struct {
		UserID string `json:"user_id"`
	} `json:"metadata"`
}

type anthropicMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func flattenContent(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		parts := make([]string, 0, len(blocks))
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// ── OpenAI request/response types ────────────────────────────────────────────

type oaiRequest struct {
	Model       string       `json:"model"`
	Messages    []oaiMessage `json:"messages"`
	MaxTokens   int          `json:"max_tokens"`
	Stream      bool         `json:"stream"`
	Temperature *float64     `json:"temperature,omitempty"`
	TopP        *float64     `json:"top_p,omitempty"`
}

type oaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type oaiResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message      struct{ Content string `json:"content"` } `json:"message"`
		FinishReason string                                    `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// ── Translation ───────────────────────────────────────────────────────────────

func toOAI(req anthropicReq, preamble string) oaiRequest {
	msgs := make([]oaiMessage, 0, len(req.Messages)+1)

	systemText := flattenContent(req.System)
	if preamble != "" {
		if systemText != "" {
			systemText = preamble + systemText
		} else {
			systemText = preamble
		}
	}
	if systemText != "" {
		msgs = append(msgs, oaiMessage{Role: "system", Content: systemText})
	}

	for _, m := range req.Messages {
		msgs = append(msgs, oaiMessage{Role: m.Role, Content: flattenContent(m.Content)})
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 1024
	}
	model := req.Model
	if model == "" {
		model = defaultModel
	}
	return oaiRequest{
		Model: model, Messages: msgs, MaxTokens: maxTokens,
		Stream: req.Stream, Temperature: req.Temperature, TopP: req.TopP,
	}
}

func toAnthropic(oai oaiResponse, model string) map[string]any {
	text, stopReason := "", "max_tokens"
	if len(oai.Choices) > 0 {
		text = oai.Choices[0].Message.Content
		if oai.Choices[0].FinishReason == "stop" {
			stopReason = "end_turn"
		}
	}
	msgID := "msg_" + strings.TrimPrefix(oai.ID, "chatcmpl-")
	if msgID == "msg_" {
		msgID = fmt.Sprintf("msg_%x", time.Now().UnixNano())
	}
	return map[string]any{
		"id": msgID, "type": "message", "role": "assistant", "model": model,
		"content":       []map[string]string{{"type": "text", "text": text}},
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]int{
			"input_tokens":  oai.Usage.PromptTokens,
			"output_tokens": oai.Usage.CompletionTokens,
		},
	}
}

func lastUserMessage(messages []anthropicMsg) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return flattenContent(messages[i].Content)
		}
	}
	return ""
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func handleModels(w http.ResponseWriter, r *http.Request) {
	resp, err := httpClient.Get(backend + "/v1/models")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	var upstream struct {
		Data []struct{ ID string `json:"id"` } `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&upstream); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	type entry struct {
		ID          string `json:"id"`
		Object      string `json:"object"`
		Created     int64  `json:"created"`
		OwnedBy     string `json:"owned_by"`
		DisplayName string `json:"display_name"`
		Type        string `json:"type"`
	}
	now := time.Now().Unix()
	models := make([]entry, len(upstream.Data))
	for i, m := range upstream.Data {
		models[i] = entry{ID: m.ID, Object: "model", Created: now, OwnedBy: "bitnet", DisplayName: m.ID, Type: "model"}
	}

	var firstID, lastID string
	if len(models) > 0 {
		firstID, lastID = models[0].ID, models[len(models)-1].ID
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"data": models, "object": "list", "has_more": false,
		"first_id": firstID, "last_id": lastID,
	})
}

func handleMessages(w http.ResponseWriter, r *http.Request) {
	var req anthropicReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID := req.Metadata.UserID
	if userID == "" {
		userID = "default"
	}
	preamble := buildPreamble(lastUserMessage(req.Messages), userID)
	oai := toOAI(req, preamble)
	model := oai.Model // already defaulted by toOAI

	body, err := json.Marshal(oai)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if oai.Stream {
		streamResponse(w, body, model)
		return
	}

	resp, err := httpClient.Post(backend+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	var oaiResp oaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&oaiResp); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toAnthropic(oaiResp, model))
}

func streamResponse(w http.ResponseWriter, body []byte, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	emit := func(event, data string) {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	msgID := fmt.Sprintf("msg_%x", time.Now().UnixNano())

	startMsg, _ := json.Marshal(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": msgID, "type": "message", "role": "assistant", "model": model,
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]int{"input_tokens": 0, "output_tokens": 0},
		},
	})
	blockStart, _ := json.Marshal(map[string]any{
		"type": "content_block_start", "index": 0,
		"content_block": map[string]string{"type": "text", "text": ""},
	})

	emit("message_start", string(startMsg))
	emit("content_block_start", string(blockStart))
	emit("ping", `{"type":"ping"}`)

	resp, err := httpClient.Post(backend+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		raw := line[6:]
		if raw == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct{ Content string `json:"content"` } `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(raw), &chunk); err != nil || len(chunk.Choices) == 0 {
			continue
		}
		if text := chunk.Choices[0].Delta.Content; text != "" {
			delta, _ := json.Marshal(map[string]any{
				"type": "content_block_delta", "index": 0,
				"delta": map[string]string{"type": "text_delta", "text": text},
			})
			emit("content_block_delta", string(delta))
		}
	}

	blockStop, _ := json.Marshal(map[string]any{"type": "content_block_stop", "index": 0})
	msgDelta, _ := json.Marshal(map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": 0},
	})
	emit("content_block_stop", string(blockStop))
	emit("message_delta", string(msgDelta))
	emit("message_stop", `{"type":"message_stop"}`)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /v1/models", handleModels)
	mux.HandleFunc("POST /v1/messages", handleMessages)

	addr := ":5000"
	log.Printf("proxy listening on %s → %s (memory: %s)", addr, backend, memoryURL)
	log.Fatal(http.ListenAndServe(addr, mux))
}
