package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func handleExchange(w http.ResponseWriter, r *http.Request) {
	amountStr := r.URL.Query().Get("amount")
	from := strings.ToUpper(r.URL.Query().Get("from"))
	to := strings.ToUpper(r.URL.Query().Get("to"))
	if amountStr == "" || from == "" || to == "" {
		errJSON(w, http.StatusBadRequest, "amount, from, and to are required")
		return
	}
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid amount")
		return
	}
	apiURL := fmt.Sprintf("https://api.frankfurter.app/latest?amount=%s&from=%s&to=%s",
		amountStr, from, strings.ReplaceAll(to, ",", "%2C"))
	body, err := fetchURL(r.Context(), apiURL)
	if err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	var resp struct {
		Date  string             `json:"date"`
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		errJSON(w, http.StatusBadGateway, "invalid response from exchange API")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"base":   from,
		"amount": amount,
		"rates":  resp.Rates,
		"date":   resp.Date,
	})
}

func handleCurrencies(w http.ResponseWriter, r *http.Request) {
	body, err := fetchURL(r.Context(), "https://api.frankfurter.app/currencies")
	if err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	var currencies map[string]string
	if err := json.Unmarshal(body, &currencies); err != nil {
		errJSON(w, http.StatusBadGateway, "failed to parse currencies response")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"currencies": currencies})
}
