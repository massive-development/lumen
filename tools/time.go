package main

import (
	"fmt"
	"net/http"
	"time"
)

var dateFormats = []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"}

func handleTime(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	writeJSON(w, http.StatusOK, map[string]any{
		"datetime": now.Format(time.RFC3339),
		"date":     now.Format("2006-01-02"),
		"time":     now.Format("15:04:05"),
		"day":      now.Weekday().String(),
		"timezone": now.Location().String(),
		"unix":     now.Unix(),
	})
}

func handleTimeConvert(w http.ResponseWriter, r *http.Request) {
	dtStr := r.URL.Query().Get("datetime")
	fromTZ := r.URL.Query().Get("from")
	toTZ := r.URL.Query().Get("to")
	if dtStr == "" || fromTZ == "" || toTZ == "" {
		errJSON(w, http.StatusBadRequest, "datetime, from, and to are required")
		return
	}
	loc, err := time.LoadLocation(fromTZ)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid 'from' timezone: "+err.Error())
		return
	}
	toLoc, err := time.LoadLocation(toTZ)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid 'to' timezone: "+err.Error())
		return
	}
	var t time.Time
	for _, f := range dateFormats {
		if t, err = time.ParseInLocation(f, dtStr, loc); err == nil {
			break
		}
	}
	if err != nil {
		errJSON(w, http.StatusBadRequest, "cannot parse datetime: "+dtStr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"original":  t.Format(time.RFC3339),
		"converted": t.In(toLoc).Format(time.RFC3339),
		"from":      fromTZ,
		"to":        toTZ,
	})
}

func handleTimeDiff(w http.ResponseWriter, r *http.Request) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" {
		errJSON(w, http.StatusBadRequest, "from is required")
		return
	}
	parseDate := func(s string) (time.Time, error) {
		for _, f := range dateFormats {
			if t, err := time.Parse(f, s); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("cannot parse: %s", s)
	}
	from, err := parseDate(fromStr)
	if err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	to := time.Now()
	if toStr != "" {
		if to, err = parseDate(toStr); err != nil {
			errJSON(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	days := int(to.Sub(from).Hours() / 24)
	abs := days
	if abs < 0 {
		abs = -abs
	}
	human := fmt.Sprintf("%d days", abs)
	switch {
	case abs >= 365:
		human = fmt.Sprintf("%.1f years", float64(abs)/365.25)
	case abs >= 30:
		human = fmt.Sprintf("%.1f months", float64(abs)/30.44)
	case abs >= 7:
		human = fmt.Sprintf("%.1f weeks", float64(abs)/7)
	}
	switch {
	case days < 0:
		human += " ago"
	case days > 0:
		human = "in " + human
	default:
		human = "today"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days":   days,
		"weeks":  float64(days) / 7,
		"months": float64(days) / 30.44,
		"years":  float64(days) / 365.25,
		"human":  human,
	})
}
