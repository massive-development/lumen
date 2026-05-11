// Memory service backed by PostgreSQL.
// Retrieval uses PostgreSQL full-text search (tsvector/tsquery) with ts_rank scoring,
// falling back to recency order when the query matches nothing.
// All memory and profile operations are scoped to a user_id (default: "default").
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	CategoryFact       = "fact"
	CategoryPreference = "preference"
	CategorySummary    = "summary"
	DefaultUser        = "default"
)

type Memory struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id,omitempty"`
	Content   string    `json:"content"`
	Category  string    `json:"category"`
	CreatedAt time.Time `json:"created_at"`
}

type Profile struct {
	UserID     string    `json:"user_id"`
	Name       string    `json:"name"`
	Style      string    `json:"style"`
	Background string    `json:"background"`
	Context    string    `json:"context"`
	UpdatedAt  time.Time `json:"updated_at"`
}

var pool *pgxpool.Pool

// ── DB init ───────────────────────────────────────────────────────────────────

func initDB(ctx context.Context) error {
	dsn := envOr("DATABASE_URL", "postgres://memory:memory@postgres:5432/bitnet_memory?sslmode=disable")

	var err error
	for i := range 10 {
		pool, err = pgxpool.New(ctx, dsn)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				break
			}
		}
		log.Printf("waiting for postgres (%d/10): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("postgres unavailable: %w", err)
	}

	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS memories (
			id         TEXT PRIMARY KEY,
			content    TEXT NOT NULL,
			category   TEXT NOT NULL DEFAULT 'fact',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			search_vec TSVECTOR GENERATED ALWAYS AS (to_tsvector('english', content)) STORED
		);
		ALTER TABLE memories ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT 'default';
		CREATE INDEX IF NOT EXISTS memories_search_idx ON memories USING GIN (search_vec);
		CREATE INDEX IF NOT EXISTS memories_created_idx ON memories (created_at DESC);
		CREATE INDEX IF NOT EXISTS memories_user_idx   ON memories (user_id);

		CREATE TABLE IF NOT EXISTS personalization (
			user_id    TEXT PRIMARY KEY,
			name       TEXT NOT NULL DEFAULT '',
			style      TEXT NOT NULL DEFAULT '',
			background TEXT NOT NULL DEFAULT '',
			context    TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	return err
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func userID(r *http.Request) string {
	if u := r.URL.Query().Get("user_id"); u != "" {
		return u
	}
	return DefaultUser
}

// ── Memory CRUD ───────────────────────────────────────────────────────────────

func addMemory(ctx context.Context, m Memory) (Memory, error) {
	m.ID = newID()
	m.CreatedAt = time.Now()
	if m.Category == "" {
		m.Category = CategoryFact
	}
	if m.UserID == "" {
		m.UserID = DefaultUser
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO memories (id, user_id, content, category, created_at) VALUES ($1, $2, $3, $4, $5)`,
		m.ID, m.UserID, m.Content, m.Category, m.CreatedAt,
	)
	return m, err
}

func removeMemory(ctx context.Context, id, uid string) (bool, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM memories WHERE id = $1 AND user_id = $2`, id, uid)
	return tag.RowsAffected() > 0, err
}

func listMemories(ctx context.Context, uid string, limit int) ([]Memory, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, user_id, content, category, created_at FROM memories
		 WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`,
		uid, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMemories(rows)
}

func searchMemories(ctx context.Context, uid, query string, limit int) ([]Memory, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, user_id, content, category, created_at
		FROM memories
		WHERE user_id = $1 AND search_vec @@ plainto_tsquery('english', $2)
		ORDER BY ts_rank(search_vec, plainto_tsquery('english', $2)) DESC, created_at DESC
		LIMIT $3`,
		uid, query, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results, err := scanMemories(rows)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return listMemories(ctx, uid, limit)
	}
	return results, nil
}

type scanner interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func scanMemories(rows scanner) ([]Memory, error) {
	var out []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.UserID, &m.Content, &m.Category, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ── Profile CRUD ──────────────────────────────────────────────────────────────

func getProfile(ctx context.Context, uid string) (Profile, error) {
	var p Profile
	err := pool.QueryRow(ctx,
		`SELECT user_id, name, style, background, context, updated_at
		 FROM personalization WHERE user_id = $1`, uid,
	).Scan(&p.UserID, &p.Name, &p.Style, &p.Background, &p.Context, &p.UpdatedAt)
	if err != nil {
		// No row is fine — return empty profile
		p.UserID = uid
		return p, nil
	}
	return p, nil
}

func upsertProfile(ctx context.Context, p Profile) (Profile, error) {
	if p.UserID == "" {
		p.UserID = DefaultUser
	}
	p.UpdatedAt = time.Now()
	_, err := pool.Exec(ctx, `
		INSERT INTO personalization (user_id, name, style, background, context, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id) DO UPDATE SET
			name       = EXCLUDED.name,
			style      = EXCLUDED.style,
			background = EXCLUDED.background,
			context    = EXCLUDED.context,
			updated_at = EXCLUDED.updated_at`,
		p.UserID, p.Name, p.Style, p.Background, p.Context, p.UpdatedAt,
	)
	return p, err
}

// ── HTTP ──────────────────────────────────────────────────────────────────────

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func main() {
	ctx := context.Background()
	if err := initDB(ctx); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// ── Memory endpoints ──────────────────────────────────────────────────────

	mux.HandleFunc("GET /memories", func(w http.ResponseWriter, r *http.Request) {
		uid := userID(r)
		q := r.URL.Query().Get("q")
		limit := 8
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
			}
		}

		var (
			memories []Memory
			err      error
		)
		if q != "" {
			memories, err = searchMemories(r.Context(), uid, q, limit)
		} else {
			memories, err = listMemories(r.Context(), uid, limit)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if memories == nil {
			memories = []Memory{}
		}
		writeJSON(w, http.StatusOK, memories)
	})

	mux.HandleFunc("POST /memories", func(w http.ResponseWriter, r *http.Request) {
		var m Memory
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(m.Content) == "" {
			http.Error(w, "content required", http.StatusBadRequest)
			return
		}
		if m.UserID == "" {
			m.UserID = userID(r)
		}
		created, err := addMemory(r.Context(), m)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	})

	mux.HandleFunc("DELETE /memories/{id}", func(w http.ResponseWriter, r *http.Request) {
		found, err := removeMemory(r.Context(), r.PathValue("id"), userID(r))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !found {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// ── Profile endpoints ─────────────────────────────────────────────────────

	mux.HandleFunc("GET /profile", func(w http.ResponseWriter, r *http.Request) {
		uid := userID(r)
		p, err := getProfile(r.Context(), uid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, p)
	})

	mux.HandleFunc("POST /profile", func(w http.ResponseWriter, r *http.Request) {
		var p Profile
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if p.UserID == "" {
			p.UserID = userID(r)
		}
		saved, err := upsertProfile(r.Context(), p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, saved)
	})

	addr := fmt.Sprintf(":%s", envOr("PORT", "6000"))
	log.Printf("memory service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, cors(mux)))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
