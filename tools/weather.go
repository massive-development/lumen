package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

var weatherCodes = map[int]string{
	0: "Clear sky", 1: "Mainly clear", 2: "Partly cloudy", 3: "Overcast",
	45: "Fog", 48: "Depositing rime fog",
	51: "Light drizzle", 53: "Moderate drizzle", 55: "Dense drizzle",
	61: "Slight rain", 63: "Moderate rain", 65: "Heavy rain",
	71: "Slight snow", 73: "Moderate snow", 75: "Heavy snow",
	77: "Snow grains", 80: "Slight showers", 81: "Moderate showers", 82: "Violent showers",
	85: "Slight snow showers", 86: "Heavy snow showers",
	95: "Thunderstorm", 96: "Thunderstorm with slight hail", 99: "Thunderstorm with heavy hail",
}

func geocode(ctx context.Context, location string) (lat, lon float64, name string, err error) {
	apiURL := "https://geocoding-api.open-meteo.com/v1/search?name=" + url.QueryEscape(location) + "&count=1&format=json"
	body, err := fetchURL(ctx, apiURL)
	if err != nil {
		return 0, 0, "", err
	}
	var resp struct {
		Results []struct {
			Name      string  `json:"name"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Country   string  `json:"country"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Results) == 0 {
		return 0, 0, "", fmt.Errorf("location not found: %s", location)
	}
	r := resp.Results[0]
	return r.Latitude, r.Longitude, r.Name + ", " + r.Country, nil
}

func handleWeather(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	units := q.Get("units")
	if units != "fahrenheit" {
		units = "celsius"
	}

	var lat, lon float64
	var locationName string

	switch {
	case q.Get("lat") != "" && q.Get("lon") != "":
		var errLat, errLon error
		lat, errLat = strconv.ParseFloat(q.Get("lat"), 64)
		lon, errLon = strconv.ParseFloat(q.Get("lon"), 64)
		if errLat != nil || errLon != nil {
			errJSON(w, http.StatusBadRequest, "invalid lat/lon")
			return
		}
		locationName = fmt.Sprintf("%.4f, %.4f", lat, lon)
	case q.Get("location") != "":
		var err error
		lat, lon, locationName, err = geocode(r.Context(), q.Get("location"))
		if err != nil {
			errJSON(w, http.StatusBadRequest, err.Error())
			return
		}
	default:
		errJSON(w, http.StatusBadRequest, "provide location or lat/lon")
		return
	}

	apiURL := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f"+
			"&current=temperature_2m,apparent_temperature,relative_humidity_2m,wind_speed_10m,weather_code"+
			"&daily=weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum"+
			"&temperature_unit=%s&wind_speed_unit=kmh&forecast_days=4&timezone=auto",
		lat, lon, units,
	)
	body, err := fetchURL(r.Context(), apiURL)
	if err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	var om struct {
		Current struct {
			Temperature float64 `json:"temperature_2m"`
			FeelsLike   float64 `json:"apparent_temperature"`
			Humidity    float64 `json:"relative_humidity_2m"`
			WindSpeed   float64 `json:"wind_speed_10m"`
			WeatherCode int     `json:"weather_code"`
		} `json:"current"`
		Daily struct {
			Time        []string  `json:"time"`
			WeatherCode []int     `json:"weather_code"`
			TempMax     []float64 `json:"temperature_2m_max"`
			TempMin     []float64 `json:"temperature_2m_min"`
			Precip      []float64 `json:"precipitation_sum"`
		} `json:"daily"`
	}
	if err := json.Unmarshal(body, &om); err != nil {
		errJSON(w, http.StatusBadGateway, "failed to parse weather response")
		return
	}

	type dayForecast struct {
		Date      string  `json:"date"`
		Condition string  `json:"condition"`
		TempMax   float64 `json:"temp_max"`
		TempMin   float64 `json:"temp_min"`
		Precip    float64 `json:"precipitation_mm"`
	}
	forecast := []dayForecast{}
	for i := 1; i < len(om.Daily.Time); i++ {
		cond := weatherCodes[om.Daily.WeatherCode[i]]
		if cond == "" {
			cond = "Unknown"
		}
		forecast = append(forecast, dayForecast{
			Date: om.Daily.Time[i], Condition: cond,
			TempMax: om.Daily.TempMax[i], TempMin: om.Daily.TempMin[i],
			Precip: om.Daily.Precip[i],
		})
	}

	condition := weatherCodes[om.Current.WeatherCode]
	if condition == "" {
		condition = "Unknown"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"location":    locationName,
		"temperature": om.Current.Temperature,
		"feels_like":  om.Current.FeelsLike,
		"condition":   condition,
		"humidity":    om.Current.Humidity,
		"wind_speed":  om.Current.WindSpeed,
		"forecast":    forecast,
	})
}
