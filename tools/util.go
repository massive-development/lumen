package main

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"
)

func handleUUID(w http.ResponseWriter, r *http.Request) {
	count := clampInt(r.URL.Query().Get("count"), 1, 1, 20)
	ids := make([]string, count)
	for i := range ids {
		ids[i] = uuid.New().String()
	}
	writeJSON(w, http.StatusOK, map[string]any{"uuids": ids})
}

func handleHash(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	algorithm := strings.ToLower(r.URL.Query().Get("algorithm"))
	if text == "" {
		errJSON(w, http.StatusBadRequest, "text is required")
		return
	}
	if algorithm == "" {
		algorithm = "sha256"
	}
	var hash string
	switch algorithm {
	case "md5":
		h := md5.Sum([]byte(text))
		hash = fmt.Sprintf("%x", h)
	case "sha1":
		h := sha1.Sum([]byte(text))
		hash = fmt.Sprintf("%x", h)
	case "sha256":
		h := sha256.Sum256([]byte(text))
		hash = fmt.Sprintf("%x", h)
	default:
		errJSON(w, http.StatusBadRequest, "unknown algorithm: "+algorithm)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"input": text, "algorithm": algorithm, "hash": hash})
}

func handleBase64(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	mode := r.URL.Query().Get("mode")
	if text == "" {
		errJSON(w, http.StatusBadRequest, "text is required")
		return
	}
	if mode == "" {
		mode = "encode"
	}
	var output string
	switch mode {
	case "encode":
		output = base64.StdEncoding.EncodeToString([]byte(text))
	case "decode":
		b, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			b, err = base64.URLEncoding.DecodeString(text)
		}
		if err != nil {
			errJSON(w, http.StatusBadRequest, "invalid base64: "+err.Error())
			return
		}
		output = string(b)
	default:
		errJSON(w, http.StatusBadRequest, "mode must be encode or decode")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"input": text, "output": output, "mode": mode})
}

func handleJSONFormat(w http.ResponseWriter, r *http.Request) {
	input := r.URL.Query().Get("json")
	if input == "" {
		errJSON(w, http.StatusBadRequest, "json is required")
		return
	}
	var parsed any
	if err := json.Unmarshal([]byte(input), &parsed); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "formatted": "", "error": err.Error()})
		return
	}
	formatted, _ := json.MarshalIndent(parsed, "", "  ")
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "formatted": string(formatted), "error": ""})
}

func handleQR(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	if text == "" {
		errJSON(w, http.StatusBadRequest, "text is required")
		return
	}
	size := clampInt(r.URL.Query().Get("size"), 256, 64, 1024)
	png, err := qrcode.Encode(text, qrcode.Medium, size)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "failed to generate QR code: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data_uri": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		"text":     text,
	})
}

const (
	charLower   = "abcdefghijklmnopqrstuvwxyz"
	charUpper   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	charNumbers = "0123456789"
	charSymbols = "!@#$%^&*()-_=+[]{}|;:,.<>?"
)

func handlePassword(w http.ResponseWriter, r *http.Request) {
	length := clampInt(r.URL.Query().Get("length"), 20, 8, 128)
	useSymbols := r.URL.Query().Get("symbols") != "false"
	useNumbers := r.URL.Query().Get("numbers") != "false"
	useUpper := r.URL.Query().Get("upper") != "false"

	charset := charLower
	if useUpper {
		charset += charUpper
	}
	if useNumbers {
		charset += charNumbers
	}
	if useSymbols {
		charset += charSymbols
	}

	pw := make([]byte, length)
	for i := range pw {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "crypto random failed")
			return
		}
		pw[i] = charset[n.Int64()]
	}

	var log2cs float64
	for cs := len(charset); cs > 1; cs >>= 1 {
		log2cs++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"password": string(pw),
		"length":   length,
		"entropy":  float64(length) * log2cs,
	})
}

func handleRandom(w http.ResponseWriter, r *http.Request) {
	min := parseInt64(r.URL.Query().Get("min"), 1)
	max := parseInt64(r.URL.Query().Get("max"), 100)
	count := clampInt(r.URL.Query().Get("count"), 1, 1, 20)
	if max <= min {
		errJSON(w, http.StatusBadRequest, "max must be greater than min")
		return
	}
	rang := big.NewInt(max - min + 1)
	numbers := make([]int64, count)
	for i := range numbers {
		n, err := rand.Int(rand.Reader, rang)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "crypto random failed")
			return
		}
		numbers[i] = min + n.Int64()
	}
	writeJSON(w, http.StatusOK, map[string]any{"numbers": numbers, "min": min, "max": max})
}

var reSentence = regexp.MustCompile(`[.!?]+`)

func handleWordCount(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	if text == "" {
		errJSON(w, http.StatusBadRequest, "text is required")
		return
	}
	words := len(strings.Fields(text))
	chars := utf8.RuneCountInString(text)
	charsNoSp := utf8.RuneCountInString(strings.ReplaceAll(text, " ", ""))
	lines := len(strings.Split(text, "\n"))
	sentences := len(reSentence.FindAllString(text, -1))

	const wpm = 200
	mins := words / wpm
	secs := (words % wpm) * 60 / wpm
	readingTime := fmt.Sprintf("%dm %ds", mins, secs)
	if mins == 0 {
		readingTime = fmt.Sprintf("%ds", secs)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"words":        words,
		"characters":   chars,
		"chars_no_sp":  charsNoSp,
		"lines":        lines,
		"sentences":    sentences,
		"reading_time": readingTime,
	})
}
