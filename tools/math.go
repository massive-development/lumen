package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/expr-lang/expr"
)

func handleMathEval(w http.ResponseWriter, r *http.Request) {
	exprStr := r.URL.Query().Get("expr")
	if exprStr == "" {
		errJSON(w, http.StatusBadRequest, "expr is required")
		return
	}
	env := map[string]any{
		"pi": 3.141592653589793,
		"e":  2.718281828459045,
	}
	program, err := expr.Compile(exprStr, expr.Env(env))
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid expression: "+err.Error())
		return
	}
	result, err := expr.Run(program, env)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "evaluation error: "+err.Error())
		return
	}
	var num float64
	switch v := result.(type) {
	case float64:
		num = v
	case int:
		num = float64(v)
	case int64:
		num = float64(v)
	default:
		errJSON(w, http.StatusBadRequest, fmt.Sprintf("result is not a number: %v", result))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"expression": exprStr, "result": num})
}

// unitEntry maps a unit name to its base category and conversion factor to that base.
type unitEntry struct {
	base   string
	factor float64
}

var unitTable = map[string]unitEntry{
	// Length (base: meters)
	"m": {"length", 1}, "meter": {"length", 1}, "meters": {"length", 1},
	"km": {"length", 1000}, "kilometer": {"length", 1000}, "kilometers": {"length", 1000},
	"cm": {"length", 0.01}, "mm": {"length", 0.001},
	"mile": {"length", 1609.344}, "miles": {"length", 1609.344},
	"yard": {"length", 0.9144}, "yards": {"length", 0.9144},
	"foot": {"length", 0.3048}, "feet": {"length", 0.3048}, "ft": {"length", 0.3048},
	"inch": {"length", 0.0254}, "inches": {"length", 0.0254}, "in": {"length", 0.0254},
	"nautical_mile": {"length", 1852},
	// Mass (base: kg)
	"kg": {"mass", 1}, "kilogram": {"mass", 1}, "kilograms": {"mass", 1},
	"g": {"mass", 0.001}, "gram": {"mass", 0.001}, "grams": {"mass", 0.001},
	"mg": {"mass", 0.000001},
	"lb": {"mass", 0.453592}, "lbs": {"mass", 0.453592}, "pound": {"mass", 0.453592}, "pounds": {"mass", 0.453592},
	"oz": {"mass", 0.0283495}, "ounce": {"mass", 0.0283495}, "ounces": {"mass", 0.0283495},
	"ton": {"mass", 907.185}, "tonne": {"mass", 1000},
	// Speed (base: km/h)
	"kmh": {"speed", 1}, "km/h": {"speed", 1}, "kph": {"speed", 1},
	"mph": {"speed", 1.60934},
	"ms": {"speed", 3.6}, "m/s": {"speed", 3.6},
	"knot": {"speed", 1.852}, "knots": {"speed", 1.852},
	// Area (base: m²)
	"m2": {"area", 1}, "sqm": {"area", 1},
	"km2": {"area", 1000000}, "sqkm": {"area", 1000000},
	"ft2": {"area", 0.092903}, "sqft": {"area", 0.092903},
	"acre": {"area", 4046.86}, "acres": {"area", 4046.86},
	"hectare": {"area", 10000}, "hectares": {"area", 10000},
	// Volume (base: liters)
	"l": {"volume", 1}, "liter": {"volume", 1}, "liters": {"volume", 1},
	"ml": {"volume", 0.001}, "milliliter": {"volume", 0.001},
	"gal": {"volume", 3.78541}, "gallon": {"volume", 3.78541}, "gallons": {"volume", 3.78541},
	"fl_oz": {"volume", 0.0295735},
	"cup": {"volume", 0.236588},
	"pint": {"volume", 0.473176},
	"quart": {"volume", 0.946353},
	// Data (base: bytes)
	"byte": {"data", 1}, "bytes": {"data", 1}, "b": {"data", 1},
	"kb": {"data", 1024}, "kilobyte": {"data", 1024},
	"mb": {"data", 1048576}, "megabyte": {"data", 1048576},
	"gb": {"data", 1073741824}, "gigabyte": {"data", 1073741824},
	"tb": {"data", 1099511627776}, "terabyte": {"data", 1099511627776},
}

var tempUnits = map[string]bool{
	"celsius": true, "c": true, "fahrenheit": true, "f": true, "kelvin": true, "k": true,
}

func handleUnitConvert(w http.ResponseWriter, r *http.Request) {
	valueStr := r.URL.Query().Get("value")
	from := strings.ToLower(r.URL.Query().Get("from"))
	to := strings.ToLower(r.URL.Query().Get("to"))
	if valueStr == "" || from == "" || to == "" {
		errJSON(w, http.StatusBadRequest, "value, from, and to are required")
		return
	}
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "invalid value")
		return
	}

	if tempUnits[from] || tempUnits[to] {
		result, err := convertTemp(value, from, to)
		if err != nil {
			errJSON(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"value": value, "from": from, "result": result, "to": to})
		return
	}

	fromEntry, ok1 := unitTable[from]
	toEntry, ok2 := unitTable[to]
	if !ok1 {
		errJSON(w, http.StatusBadRequest, "unknown unit: "+from)
		return
	}
	if !ok2 {
		errJSON(w, http.StatusBadRequest, "unknown unit: "+to)
		return
	}
	if fromEntry.base != toEntry.base {
		errJSON(w, http.StatusBadRequest, fmt.Sprintf("cannot convert %s (%s) to %s (%s)", from, fromEntry.base, to, toEntry.base))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"value":  value,
		"from":   from,
		"result": value * fromEntry.factor / toEntry.factor,
		"to":     to,
	})
}

func convertTemp(value float64, from, to string) (float64, error) {
	var celsius float64
	switch from {
	case "celsius", "c":
		celsius = value
	case "fahrenheit", "f":
		celsius = (value - 32) * 5 / 9
	case "kelvin", "k":
		celsius = value - 273.15
	default:
		return 0, fmt.Errorf("unknown temperature unit: %s", from)
	}
	switch to {
	case "celsius", "c":
		return celsius, nil
	case "fahrenheit", "f":
		return celsius*9/5 + 32, nil
	case "kelvin", "k":
		return celsius + 273.15, nil
	default:
		return 0, fmt.Errorf("unknown temperature unit: %s", to)
	}
}
