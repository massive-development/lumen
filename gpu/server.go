// OpenAI-compatible HTTP server for BitNet GPU inference.
// Manages worker.py (PyTorch) as a subprocess; communicates via newline-delimited JSON.
//
// Protocol:
//
//	→ stdin:  {"id":"...","messages":[...],"max_tokens":N,"temperature":F,"top_p":F}
//	← stdout: {"id":"...","text":"..."} (one per decoded chunk)
//	          {"id":"...","done":true,"finish_reason":"stop"|"length","prompt_tokens":N,"completion_tokens":N}
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	defaultTemperature = 0.7
	defaultTopP        = 0.95
	defaultMaxTokens   = 512
)

var (
	workerCmd *exec.Cmd
	workerIn  io.Writer
	workerOut *bufio.Scanner
	workerMu  sync.Mutex
	ready     atomic.Bool

	modelAlias = envOr("MODEL_ALIAS", "bitnet-b1.58-2b-4t")
	port       = envOr("PORT", "8082")
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type workerMsg struct {
	ID               string `json:"id"`
	Text             string `json:"text,omitempty"`
	Done             bool   `json:"done,omitempty"`
	FinishReason     string `json:"finish_reason,omitempty"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
	Status           string `json:"status,omitempty"`
	Error            string `json:"error,omitempty"`
}

type workerReq struct {
	ID          string       `json:"id"`
	Messages    []oaiMessage `json:"messages"`
	MaxTokens   int          `json:"max_tokens"`
	Temperature float64      `json:"temperature"`
	TopP        float64      `json:"top_p"`
}

func startWorker() {
	cmd := exec.Command("python3", "worker.py")
	workerCmd = cmd
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	workerIn = stdin
	workerOut = bufio.NewScanner(stdout)
	workerOut.Buffer(make([]byte, 256*1024), 256*1024)

	for workerOut.Scan() {
		var msg workerMsg
		if err := json.Unmarshal(workerOut.Bytes(), &msg); err != nil {
			continue
		}
		if msg.Status != "" {
			log.Printf("worker: %s", msg.Status)
		}
		if msg.Status == "ready" {
			ready.Store(true)
			return
		}
	}
	log.Fatal("worker process exited before ready")
}

// send writes req to the worker and collects responses.
// If w is non-nil, text chunks are streamed as SSE while holding workerMu.
func send(req workerReq, w http.ResponseWriter) ([]workerMsg, error) {
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	workerMu.Lock()
	defer workerMu.Unlock()

	fmt.Fprintf(workerIn, "%s\n", line)

	var chunks []workerMsg
	var flusher http.Flusher
	var gotDone bool
	if w != nil {
		flusher, _ = w.(http.Flusher)
	}

	for workerOut.Scan() {
		var msg workerMsg
		if err := json.Unmarshal(workerOut.Bytes(), &msg); err != nil {
			continue
		}
		if msg.Error != "" {
			return nil, fmt.Errorf("%s", msg.Error)
		}
		chunks = append(chunks, msg)
		if w != nil && msg.Text != "" && flusher != nil {
			sseChunk(w, req.ID, msg.Text, "")
			flusher.Flush()
		}
		if msg.Done {
			gotDone = true
			break
		}
	}
	if !gotDone {
		// Worker exited or crashed without sending a done message.
		// Mark unhealthy so Olla routes to CPU until the container restarts.
		ready.Store(false)
		if err := workerOut.Err(); err != nil {
			return nil, fmt.Errorf("worker read error: %w", err)
		}
		return nil, fmt.Errorf("worker exited unexpectedly")
	}
	return chunks, nil
}

func sseChunk(w io.Writer, id, text, finishReason string) {
	var finishJSON interface{} = nil
	if finishReason != "" {
		finishJSON = finishReason
	}
	delta := map[string]string{"role": "assistant", "content": text}
	if finishReason != "" {
		delta = map[string]string{}
	}
	data, _ := json.Marshal(map[string]any{
		"id": id, "object": "chat.completion.chunk",
		"created": time.Now().Unix(), "model": modelAlias,
		"choices": []map[string]any{{
			"index": 0, "delta": delta, "finish_reason": finishJSON,
		}},
	})
	fmt.Fprintf(w, "data: %s\n\n", data)
}

type oaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "loading"})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data": []map[string]any{{
			"id": modelAlias, "object": "model",
			"created": time.Now().Unix(), "owned_by": "bitnet",
		}},
	})
}

func handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if !ready.Load() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "model loading"}})
		return
	}

	var body struct {
		Messages    []oaiMessage `json:"messages"`
		MaxTokens   int          `json:"max_tokens"`
		Stream      bool         `json:"stream"`
		Temperature *float64     `json:"temperature"`
		TopP        *float64     `json:"top_p"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	temperature, topP := defaultTemperature, defaultTopP
	if body.Temperature != nil {
		temperature = *body.Temperature
	}
	if body.TopP != nil {
		topP = *body.TopP
	}
	maxTokens := body.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	reqID := fmt.Sprintf("chatcmpl-%x", time.Now().UnixNano())
	req := workerReq{
		ID: reqID, Messages: body.Messages,
		MaxTokens: maxTokens, Temperature: temperature, TopP: topP,
	}

	if body.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")

		chunks, err := send(req, w)
		if err != nil {
			return
		}

		finishReason := "stop"
		for _, c := range chunks {
			if c.Done && c.FinishReason != "" {
				finishReason = c.FinishReason
			}
		}
		sseChunk(w, reqID, "", finishReason)
		fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}

	chunks, err := send(req, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var sb strings.Builder
	var promptTokens, completionTokens int
	finishReason := "stop"
	for _, c := range chunks {
		sb.WriteString(c.Text)
		if c.Done {
			finishReason = c.FinishReason
			promptTokens = c.PromptTokens
			completionTokens = c.CompletionTokens
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id": reqID, "object": "chat.completion",
		"created": time.Now().Unix(), "model": modelAlias,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]string{"role": "assistant", "content": sb.String()},
			"finish_reason": finishReason,
		}},
		"usage": map[string]int{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      promptTokens + completionTokens,
		},
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		// Skip health check noise.
		if r.URL.Path == "/health" {
			return
		}
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /v1/models", handleModels)
	mux.HandleFunc("POST /v1/chat/completions", handleChatCompletions)

	go func() {
		log.Printf("GPU server listening on :%s (model: %s)", port, modelAlias)
		log.Fatal(http.ListenAndServe(":"+port, logMiddleware(mux)))
	}()

	startWorker()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig
	if workerCmd != nil && workerCmd.Process != nil {
		workerCmd.Process.Signal(syscall.SIGTERM)
	}
}
