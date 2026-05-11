package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, spec) })
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /time", handleTime)
	mux.HandleFunc("GET /time/convert", handleTimeConvert)
	mux.HandleFunc("GET /time/diff", handleTimeDiff)

	mux.HandleFunc("GET /web/fetch", handleWebFetch)
	mux.HandleFunc("GET /web/search", handleWebSearch)
	mux.HandleFunc("GET /web/wikipedia", handleWikipedia)

	mux.HandleFunc("GET /weather", handleWeather)

	mux.HandleFunc("GET /finance/exchange", handleExchange)
	mux.HandleFunc("GET /finance/currencies", handleCurrencies)

	mux.HandleFunc("GET /memory/search", handleMemorySearch)
	mux.HandleFunc("POST /memory/save", handleMemorySave)
	mux.HandleFunc("GET /profile", handleProfileGet)
	mux.HandleFunc("POST /profile", handleProfileUpdate)
	mux.HandleFunc("POST /profile/update", handleProfileUpdate)

	mux.HandleFunc("GET /math/eval", handleMathEval)
	mux.HandleFunc("GET /math/convert", handleUnitConvert)

	mux.HandleFunc("GET /network/ip", handlePublicIP)
	mux.HandleFunc("GET /network/dns", handleDNS)

	mux.HandleFunc("GET /system/info", handleSystemInfo)

	mux.HandleFunc("GET /util/uuid", handleUUID)
	mux.HandleFunc("GET /util/hash", handleHash)
	mux.HandleFunc("GET /util/base64", handleBase64)
	mux.HandleFunc("GET /util/json", handleJSONFormat)
	mux.HandleFunc("GET /util/qr", handleQR)
	mux.HandleFunc("GET /util/password", handlePassword)
	mux.HandleFunc("GET /util/random", handleRandom)
	mux.HandleFunc("GET /util/word_count", handleWordCount)

	mux.HandleFunc("GET /news/feed", handleRSSFeed)
	mux.HandleFunc("GET /news/hn", handleHackerNews)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("tool server listening on %s (memory: %s)", addr, memoryURL)
	log.Fatal(http.ListenAndServe(addr, logged(mux)))
}
