// Semantic response cache sitting between OpenWebUI and Olla.
// Checks exact hash then cosine-similarity against pgvector before forwarding.
// On cache miss: proxies to BACKEND_URL, taps the response, and stores it.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	port                = envOr("CACHE_PORT", "8090")
	backendURL          = envOr("BACKEND_URL", "http://olla:40114/olla/openai")
	ollamaURL           = envOr("OLLAMA_BASE_URL", "http://ollama:11434")
	embeddingModel      = envOr("EMBEDDING_MODEL", "nomic-embed-text")
	databaseURL         = os.Getenv("DATABASE_URL")
	similarityThreshold = parseFloat(envOr("SIMILARITY_THRESHOLD", "0.92"))
	cacheTTLHours       = parseInt(envOr("CACHE_TTL_HOURS", "168"))
	pool                *pgxpool.Pool
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0.92
	}
	return v
}

func parseInt(s string) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 168
	}
	return v
}

type oaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string       `json:"model"`
	Messages    []oaiMessage `json:"messages"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
	TopP        *float64     `json:"top_p,omitempty"`
	Stream      bool         `json:"stream,omitempty"`
}

func initDB(ctx context.Context) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS response_cache (
			id          UUID DEFAULT gen_random_uuid() PRIMARY KEY,
			prompt_hash TEXT UNIQUE NOT NULL,
			prompt_text TEXT NOT NULL,
			embedding   vector(768),
			content     TEXT NOT NULL,
			model       TEXT NOT NULL DEFAULT '',
			hit_count   INT NOT NULL DEFAULT 0,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS response_cache_embedding_idx
			ON response_cache USING hnsw (embedding vector_cosine_ops);
	`)
	return err
}

func embed(text string) ([]float64, error) {
	body, _ := json.Marshal(map[string]string{"model": embeddingModel, "prompt": text})
	resp, err := http.Post(ollamaURL+"/api/embeddings", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return r.Embedding, nil
}

func vecToSQL(v []float64) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatFloat(f, 'f', 8, 64))
	}
	sb.WriteByte(']')
	return sb.String()
}

func promptHash(messages []oaiMessage) string {
	b, _ := json.Marshal(messages)
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h)
}

func lastUserMessage(messages []oaiMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

func lookupExact(ctx context.Context, hash string) (content, model string, ok bool) {
	err := pool.QueryRow(ctx, `
		UPDATE response_cache SET hit_count = hit_count + 1
		WHERE prompt_hash = $1
		  AND created_at > NOW() - ($2 * INTERVAL '1 hour')
		RETURNING content, model
	`, hash, cacheTTLHours).Scan(&content, &model)
	return content, model, err == nil
}

func lookupSemantic(ctx context.Context, vec []float64) (content, model string, ok bool) {
	vecSQL := vecToSQL(vec)
	var similarity float64
	err := pool.QueryRow(ctx, `
		SELECT content, model, 1 - (embedding <=> $1::vector) AS similarity
		FROM response_cache
		WHERE created_at > NOW() - ($2 * INTERVAL '1 hour')
		  AND embedding IS NOT NULL
		ORDER BY embedding <=> $1::vector
		LIMIT 1
	`, vecSQL, cacheTTLHours).Scan(&content, &model, &similarity)
	if err != nil || similarity < similarityThreshold {
		return "", "", false
	}
	return content, model, true
}

func storeCache(hash, promptText, content, model string, vec []float64) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var vecSQL any
	if vec != nil {
		vecSQL = vecToSQL(vec)
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO response_cache (prompt_hash, prompt_text, embedding, content, model)
		VALUES ($1, $2, $3::vector, $4, $5)
		ON CONFLICT (prompt_hash) DO UPDATE
		SET content = EXCLUDED.content, model = EXCLUDED.model,
		    embedding = EXCLUDED.embedding, created_at = NOW()
	`, hash, promptText, vecSQL, content, model)
	if err != nil {
		log.Printf("cache store: %v", err)
	}
}

func cacheHitResponse(w http.ResponseWriter, r *http.Request, reqID, model, content, cacheHeader string) {
	var req chatRequest
	json.NewDecoder(r.Body).Decode(&req) // already read; body used only for stream flag below
	w.Header().Set("X-Cache", cacheHeader)
}

func serveFromCache(w http.ResponseWriter, stream bool, reqID, model, content string) {
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		writeSSECache(w, reqID, model, content)
	} else {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":      reqID,
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]string{"role": "assistant", "content": content},
				"finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
		})
	}
}

func writeSSECache(w http.ResponseWriter, id, model, content string) {
	flusher, _ := w.(http.Flusher)
	emit := func(v any) {
		data, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}
	emit(map[string]any{
		"id": id, "object": "chat.completion.chunk",
		"created": time.Now().Unix(), "model": model,
		"choices": []map[string]any{{
			"index": 0, "delta": map[string]string{"role": "assistant", "content": content},
			"finish_reason": nil,
		}},
	})
	emit(map[string]any{
		"id": id, "object": "chat.completion.chunk",
		"created": time.Now().Unix(), "model": model,
		"choices": []map[string]any{{
			"index": 0, "delta": map[string]string{}, "finish_reason": "stop",
		}},
	})
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	var req chatRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	reqID := fmt.Sprintf("chatcmpl-%x", time.Now().UnixNano())
	hash := promptHash(req.Messages)

	// Exact match
	if content, model, ok := lookupExact(ctx, hash); ok {
		log.Printf("HIT exact %s", hash[:8])
		w.Header().Set("X-Cache", "HIT")
		serveFromCache(w, req.Stream, reqID, model, content)
		return
	}

	// Semantic match
	userMsg := lastUserMessage(req.Messages)
	var vec []float64
	if userMsg != "" {
		if v, err := embed(userMsg); err == nil {
			vec = v
			if content, model, ok := lookupSemantic(ctx, vec); ok {
				log.Printf("HIT semantic %s", hash[:8])
				w.Header().Set("X-Cache", "HIT-SEMANTIC")
				serveFromCache(w, req.Stream, reqID, model, content)
				return
			}
		} else {
			log.Printf("embed: %v", err)
		}
	}

	// Miss — proxy to backend
	log.Printf("MISS %s", hash[:8])
	upReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		backendURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	upReq.Header.Set("Content-Type", "application/json")
	if auth := r.Header.Get("Authorization"); auth != "" {
		upReq.Header.Set("Authorization", auth)
	}

	upResp, err := http.DefaultClient.Do(upReq)
	if err != nil {
		http.Error(w, "upstream unreachable", http.StatusBadGateway)
		return
	}
	defer upResp.Body.Close()

	for k, vs := range upResp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(upResp.StatusCode)

	if upResp.StatusCode != http.StatusOK {
		io.Copy(w, upResp.Body)
		return
	}

	if req.Stream {
		var sb strings.Builder
		var modelName string
		flusher, _ := w.(http.Flusher)
		scanner := bufio.NewScanner(upResp.Body)
		scanner.Buffer(make([]byte, 256*1024), 256*1024)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintf(w, "%s\n", line)
			if flusher != nil {
				flusher.Flush()
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			var chunk map[string]any
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if modelName == "" {
				if m, ok := chunk["model"].(string); ok {
					modelName = m
				}
			}
			choices, _ := chunk["choices"].([]any)
			for _, c := range choices {
				if cm, ok := c.(map[string]any); ok {
					if delta, ok := cm["delta"].(map[string]any); ok {
						if text, ok := delta["content"].(string); ok {
							sb.WriteString(text)
						}
					}
				}
			}
		}
		if content := sb.String(); content != "" {
			capturedHash, capturedMsg, capturedModel, capturedVec :=
				hash, userMsg, modelName, vec
			go storeCache(capturedHash, capturedMsg, content, capturedModel, capturedVec)
		}
	} else {
		body, err := io.ReadAll(upResp.Body)
		if err != nil {
			return
		}
		w.Write(body)
		var resp map[string]any
		if err := json.Unmarshal(body, &resp); err != nil {
			return
		}
		var content, modelName string
		if m, ok := resp["model"].(string); ok {
			modelName = m
		}
		if choices, ok := resp["choices"].([]any); ok && len(choices) > 0 {
			if cm, ok := choices[0].(map[string]any); ok {
				if msg, ok := cm["message"].(map[string]any); ok {
					content, _ = msg["content"].(string)
				}
			}
		}
		if content != "" {
			go storeCache(hash, userMsg, content, modelName, vec)
		}
	}
}

func handleModels(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Get(backendURL + "/v1/models")
	if err != nil {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	ctx := context.Background()
	var err error
	pool, err = pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := initDB(ctx); err != nil {
		log.Fatalf("db init: %v", err)
	}
	log.Printf("cache ready  threshold=%.2f  ttl=%dh  backend=%s",
		similarityThreshold, cacheTTLHours, backendURL)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /v1/models", handleModels)
	mux.HandleFunc("POST /v1/chat/completions", handleChatCompletions)

	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
