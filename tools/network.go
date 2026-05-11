package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
)

func handlePublicIP(w http.ResponseWriter, r *http.Request) {
	body, err := fetchURL(r.Context(), "https://ipapi.co/json/")
	if err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	var info struct {
		IP       string  `json:"ip"`
		Country  string  `json:"country_name"`
		Region   string  `json:"region"`
		City     string  `json:"city"`
		ISP      string  `json:"org"`
		Timezone string  `json:"timezone"`
		Lat      float64 `json:"latitude"`
		Lon      float64 `json:"longitude"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		errJSON(w, http.StatusBadGateway, "failed to parse IP response")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ip":       info.IP,
		"country":  info.Country,
		"region":   info.Region,
		"city":     info.City,
		"isp":      info.ISP,
		"timezone": info.Timezone,
		"lat":      info.Lat,
		"lon":      info.Lon,
	})
}

func handleDNS(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	recordType := strings.ToUpper(r.URL.Query().Get("type"))
	if host == "" {
		errJSON(w, http.StatusBadRequest, "host is required")
		return
	}
	if recordType == "" {
		recordType = "A"
	}

	records := []string{}
	var err error

	switch recordType {
	case "A":
		addrs, e := net.LookupHost(host)
		err = e
		for _, a := range addrs {
			if !strings.Contains(a, ":") {
				records = append(records, a)
			}
		}
	case "AAAA":
		addrs, e := net.LookupHost(host)
		err = e
		for _, a := range addrs {
			if strings.Contains(a, ":") {
				records = append(records, a)
			}
		}
	case "MX":
		mxs, e := net.LookupMX(host)
		err = e
		for _, mx := range mxs {
			records = append(records, fmt.Sprintf("%d %s", mx.Pref, mx.Host))
		}
	case "NS":
		nss, e := net.LookupNS(host)
		err = e
		for _, ns := range nss {
			records = append(records, ns.Host)
		}
	case "TXT":
		txts, e := net.LookupTXT(host)
		err = e
		records = append(records, txts...)
	case "CNAME":
		cname, e := net.LookupCNAME(host)
		err = e
		if e == nil {
			records = append(records, cname)
		}
	default:
		errJSON(w, http.StatusBadRequest, "unsupported record type: "+recordType)
		return
	}

	if err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"host":    host,
		"type":    recordType,
		"records": records,
	})
}
