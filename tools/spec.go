package main

func prop(typ, desc string) map[string]any { return map[string]any{"type": typ, "description": desc} }

func propArray(itemType, desc string) map[string]any {
	return map[string]any{
		"type": "array", "description": desc,
		"items": map[string]any{"type": itemType},
	}
}

func propObj(desc string) map[string]any { return map[string]any{"type": "object", "description": desc} }

func queryParam(name, typ string, required bool, desc string) map[string]any {
	return map[string]any{
		"name": name, "in": "query", "required": required,
		"description": desc, "schema": map[string]any{"type": typ},
	}
}

func queryParamEnum(name string, required bool, desc string, values []string) map[string]any {
	return map[string]any{
		"name": name, "in": "query", "required": required,
		"description": desc, "schema": map[string]any{"type": "string", "enum": values},
	}
}

func jsonResponse(desc string, props map[string]any) map[string]any {
	return map[string]any{
		"description": desc,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{"type": "object", "properties": props},
			},
		},
	}
}

func postBody(required []string, props map[string]any) map[string]any {
	return map[string]any{
		"required": true,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{"type": "object", "required": required, "properties": props},
			},
		},
	}
}

func getOp(opID, summary, desc string, params []any, resp map[string]any) map[string]any {
	if params == nil {
		params = []any{}
	}
	return map[string]any{"get": map[string]any{
		"operationId": opID, "summary": summary, "description": desc,
		"parameters": params, "responses": map[string]any{"200": resp},
	}}
}

var spec = map[string]any{
	"openapi": "3.1.0",
	"info":    map[string]any{"title": "BitNet Tools", "version": "2.0.0"},
	"paths": map[string]any{

		"/time": getOp("get_current_time",
			"Get the current date and time",
			"Returns today's date, current time, day of week, and timezone.",
			nil,
			jsonResponse("Current time", map[string]any{
				"datetime": prop("string", "ISO 8601 datetime"),
				"date":     prop("string", "YYYY-MM-DD"),
				"time":     prop("string", "HH:MM:SS"),
				"day":      prop("string", "Day of week"),
				"timezone": prop("string", "Timezone name"),
				"unix":     prop("integer", "Unix timestamp"),
			}),
		),
		"/time/convert": getOp("convert_timezone",
			"Convert time between timezones",
			"Converts a datetime string from one IANA timezone to another (e.g. America/New_York, Europe/London, UTC).",
			[]any{
				queryParam("datetime", "string", true, "ISO 8601 datetime (e.g. 2024-01-15T14:30:00)"),
				queryParam("from", "string", true, "Source IANA timezone"),
				queryParam("to", "string", true, "Target IANA timezone"),
			},
			jsonResponse("Converted time", map[string]any{
				"original":  prop("string", "Original datetime with timezone"),
				"converted": prop("string", "Converted datetime with timezone"),
				"from":      prop("string", "Source timezone"),
				"to":        prop("string", "Target timezone"),
			}),
		),
		"/time/diff": getOp("date_difference",
			"Calculate difference between two dates",
			"Computes the time difference between two dates in days, weeks, months, and years.",
			[]any{
				queryParam("from", "string", true, "Start date (YYYY-MM-DD or ISO 8601)"),
				queryParam("to", "string", false, "End date (defaults to now)"),
			},
			jsonResponse("Date difference", map[string]any{
				"days":   prop("integer", "Total difference in days"),
				"weeks":  prop("number", "Difference in weeks"),
				"months": prop("number", "Approximate months"),
				"years":  prop("number", "Approximate years"),
				"human":  prop("string", "Human-readable description"),
			}),
		),

		"/web/fetch": getOp("fetch_webpage",
			"Fetch and read a web page",
			"Fetches the text content of any URL with HTML stripped. Use for reading articles, docs, or pages the user references.",
			[]any{queryParam("url", "string", true, "The URL to fetch")},
			jsonResponse("Page content", map[string]any{
				"url":     prop("string", "Fetched URL"),
				"content": prop("string", "Plain text content"),
			}),
		),
		"/web/search": getOp("web_search",
			"Search the web with DuckDuckGo",
			"Returns instant answer and top results from DuckDuckGo. Use for general searches when the user asks to find or look up something online.",
			[]any{
				queryParam("q", "string", true, "Search query"),
				queryParam("limit", "integer", false, "Max results (default 5, max 10)"),
			},
			jsonResponse("Search results", map[string]any{
				"abstract":       prop("string", "Instant answer summary"),
				"abstract_url":   prop("string", "Source URL for abstract"),
				"results":        propArray("object", "Search results with title and url"),
				"related_topics": propArray("string", "Related topic strings"),
			}),
		),
		"/web/wikipedia": getOp("wikipedia_summary",
			"Get a Wikipedia article summary",
			"Fetches the introductory summary of a Wikipedia article for any topic.",
			[]any{
				queryParam("q", "string", true, "Topic to look up"),
				queryParam("lang", "string", false, "Language code (default: en)"),
			},
			jsonResponse("Wikipedia summary", map[string]any{
				"title":   prop("string", "Article title"),
				"summary": prop("string", "Article extract"),
				"url":     prop("string", "Link to full article"),
			}),
		),

		"/weather": getOp("get_weather",
			"Get current weather and forecast",
			"Returns current conditions and 3-day forecast using the free Open-Meteo API. Provide a city name or lat/lon.",
			[]any{
				queryParam("location", "string", false, "City name (e.g. 'London')"),
				queryParam("lat", "number", false, "Latitude (alternative to location)"),
				queryParam("lon", "number", false, "Longitude (alternative to location)"),
				queryParamEnum("units", false, "Temperature units (default: celsius)", []string{"celsius", "fahrenheit"}),
			},
			jsonResponse("Weather", map[string]any{
				"location":    prop("string", "Resolved location name"),
				"temperature": prop("number", "Current temperature"),
				"feels_like":  prop("number", "Feels like temperature"),
				"condition":   prop("string", "Weather condition"),
				"humidity":    prop("number", "Relative humidity %"),
				"wind_speed":  prop("number", "Wind speed km/h"),
				"forecast":    propArray("object", "Daily forecasts with date, condition, temp_max, temp_min, precipitation_mm"),
			}),
		),

		"/finance/exchange": getOp("currency_exchange",
			"Convert currency amounts",
			"Converts an amount between currencies using the free Frankfurter API with real-time rates.",
			[]any{
				queryParam("amount", "number", true, "Amount to convert"),
				queryParam("from", "string", true, "Source currency (e.g. USD)"),
				queryParam("to", "string", true, "Target currency (e.g. EUR), comma-separated for multiple"),
			},
			jsonResponse("Exchange result", map[string]any{
				"base":   prop("string", "Base currency"),
				"amount": prop("number", "Original amount"),
				"rates":  propObj("Map from currency code to converted amount"),
				"date":   prop("string", "Rate date"),
			}),
		),
		"/finance/currencies": getOp("list_currencies",
			"List supported currencies",
			"Returns all currency codes and names supported by the exchange rate service.",
			nil,
			jsonResponse("Currency list", map[string]any{
				"currencies": propObj("Map from currency code to name"),
			}),
		),

		"/profile": getOp("get_profile",
			"Get user profile",
			"Returns the personalization profile for a user: name, communication style, background, and context. Provide the user_id to fetch a specific user's profile (defaults to 'default').",
			[]any{
				queryParam("user_id", "string", false, "User identifier (default: 'default')"),
			},
			jsonResponse("User profile", map[string]any{
				"user_id":    prop("string", "User identifier"),
				"name":       prop("string", "User's name"),
				"style":      prop("string", "Preferred communication style"),
				"background": prop("string", "User's background or role"),
				"context":    prop("string", "Additional context about the user"),
				"updated_at": prop("string", "When the profile was last updated"),
			}),
		),

		"/memory/search": getOp("search_memories",
			"Search saved memories",
			"Searches the user's saved memories by keyword. Use when the user asks about something you might have been told before.",
			[]any{
				queryParam("q", "string", true, "Search query"),
				queryParam("limit", "integer", false, "Max results (default 5)"),
			},
			jsonResponse("Matching memories", map[string]any{
				"id":         prop("string", "Memory ID"),
				"content":    prop("string", "Memory content"),
				"category":   prop("string", "fact | preference | summary"),
				"created_at": prop("string", "When saved"),
			}),
		),
		"/profile/update": map[string]any{"post": map[string]any{
			"operationId": "update_profile",
			"summary":     "Update user profile",
			"description": "Creates or updates the personalization profile for a user. Set name, communication style, background, and context. All fields are optional — only provided fields are updated.",
			"requestBody": postBody([]string{"user_id"}, map[string]any{
				"user_id":    map[string]any{"type": "string", "description": "User identifier (default: 'default')"},
				"name":       map[string]any{"type": "string", "description": "User's name"},
				"style":      map[string]any{"type": "string", "description": "Preferred communication style (e.g. 'concise and technical')"},
				"background": map[string]any{"type": "string", "description": "User's role or background (e.g. 'software engineer')"},
				"context":    map[string]any{"type": "string", "description": "Additional context (e.g. 'working on a Go microservices project')"},
			}),
			"responses": map[string]any{"200": jsonResponse("Updated profile", map[string]any{
				"user_id":    prop("string", "User identifier"),
				"name":       prop("string", "User's name"),
				"style":      prop("string", "Communication style"),
				"background": prop("string", "User's background"),
				"context":    prop("string", "Additional context"),
				"updated_at": prop("string", "Timestamp"),
			})},
		}},

		"/memory/save": map[string]any{"post": map[string]any{
			"operationId": "save_memory",
			"summary":     "Save something to memory",
			"description": "Saves a fact, preference, or summary to persistent memory. Use when the user asks you to remember something.",
			"requestBody": postBody([]string{"content"}, map[string]any{
				"content":  map[string]any{"type": "string", "description": "What to remember"},
				"category": map[string]any{"type": "string", "enum": []string{"fact", "preference", "summary"}, "description": "Memory category"},
			}),
			"responses": map[string]any{"201": map[string]any{"description": "Saved"}},
		}},

		"/math/eval": getOp("evaluate_expression",
			"Evaluate a mathematical expression",
			"Evaluates arithmetic expressions. Supports +, -, *, /, **, sqrt, abs, ceil, floor, min, max, and built-in constants pi and e.",
			[]any{queryParam("expr", "string", true, "Expression (e.g. '2 + 3 * 4', 'sqrt(144)')")},
			jsonResponse("Result", map[string]any{
				"expression": prop("string", "Original expression"),
				"result":     prop("number", "Evaluated result"),
			}),
		),
		"/math/convert": getOp("convert_units",
			"Convert between units of measurement",
			"Converts values between common units: length, mass, temperature, speed, area, volume, data size.",
			[]any{
				queryParam("value", "number", true, "Value to convert"),
				queryParam("from", "string", true, "Source unit (e.g. km, kg, celsius, mph)"),
				queryParam("to", "string", true, "Target unit (e.g. miles, lb, fahrenheit, kph)"),
			},
			jsonResponse("Conversion", map[string]any{
				"value":  prop("number", "Original value"),
				"from":   prop("string", "Source unit"),
				"result": prop("number", "Converted value"),
				"to":     prop("string", "Target unit"),
			}),
		),

		"/network/ip": getOp("get_public_ip",
			"Get public IP and geolocation",
			"Returns the server's public IP address with country, region, city, and ISP.",
			nil,
			jsonResponse("IP info", map[string]any{
				"ip":       prop("string", "Public IP address"),
				"country":  prop("string", "Country name"),
				"region":   prop("string", "Region/state"),
				"city":     prop("string", "City"),
				"isp":      prop("string", "Internet service provider"),
				"timezone": prop("string", "Timezone"),
				"lat":      prop("number", "Latitude"),
				"lon":      prop("number", "Longitude"),
			}),
		),
		"/network/dns": getOp("dns_lookup",
			"Perform a DNS lookup",
			"Resolves a hostname to its IP addresses or looks up MX, NS, TXT, or CNAME records.",
			[]any{
				queryParam("host", "string", true, "Hostname to resolve"),
				queryParamEnum("type", false, "Record type (default: A)", []string{"A", "AAAA", "MX", "NS", "TXT", "CNAME"}),
			},
			jsonResponse("DNS records", map[string]any{
				"host":    prop("string", "Queried hostname"),
				"type":    prop("string", "Record type"),
				"records": propArray("string", "Record values"),
			}),
		),

		"/system/info": getOp("get_system_info",
			"Get system resource usage",
			"Returns OS, CPU, memory, and Go runtime info for the host running this service.",
			nil,
			jsonResponse("System info", map[string]any{
				"os":           prop("string", "Operating system"),
				"arch":         prop("string", "CPU architecture"),
				"cpus":         prop("integer", "Number of CPU cores"),
				"go_version":   prop("string", "Go runtime version"),
				"goroutines":   prop("integer", "Active goroutines"),
				"memory_alloc": prop("string", "Current heap allocation"),
				"memory_sys":   prop("string", "Total memory from OS"),
			}),
		),

		"/util/uuid": getOp("generate_uuid",
			"Generate UUIDs",
			"Generates one or more random UUID v4 values.",
			[]any{queryParam("count", "integer", false, "Number of UUIDs (default 1, max 20)")},
			jsonResponse("UUIDs", map[string]any{
				"uuids": propArray("string", "UUID v4 strings"),
			}),
		),
		"/util/hash": getOp("hash_text",
			"Hash text",
			"Computes a MD5, SHA-1, or SHA-256 hash of the input string.",
			[]any{
				queryParam("text", "string", true, "Text to hash"),
				queryParamEnum("algorithm", false, "Hash algorithm (default: sha256)", []string{"md5", "sha1", "sha256"}),
			},
			jsonResponse("Hash", map[string]any{
				"input":     prop("string", "Original text"),
				"algorithm": prop("string", "Algorithm used"),
				"hash":      prop("string", "Hex-encoded hash"),
			}),
		),
		"/util/base64": getOp("base64_encode_decode",
			"Encode or decode base64",
			"Encodes a string to base64 or decodes a base64 string back to text.",
			[]any{
				queryParam("text", "string", true, "Text to encode or base64 to decode"),
				queryParamEnum("mode", false, "Operation (default: encode)", []string{"encode", "decode"}),
			},
			jsonResponse("Base64 result", map[string]any{
				"input":  prop("string", "Input value"),
				"output": prop("string", "Result"),
				"mode":   prop("string", "encode or decode"),
			}),
		),
		"/util/json": getOp("format_json",
			"Validate and pretty-print JSON",
			"Parses and re-formats a JSON string with indentation. Returns error details if invalid.",
			[]any{queryParam("json", "string", true, "JSON string to format")},
			jsonResponse("Formatted JSON", map[string]any{
				"valid":     prop("boolean", "Whether input is valid JSON"),
				"formatted": prop("string", "Pretty-printed JSON (if valid)"),
				"error":     prop("string", "Parse error (if invalid)"),
			}),
		),
		"/util/qr": getOp("generate_qr_code",
			"Generate a QR code",
			"Creates a QR code PNG for any text or URL. Returns as a base64 data URI.",
			[]any{
				queryParam("text", "string", true, "Text or URL to encode"),
				queryParamEnum("size", false, "Image size in pixels (default: 256)", []string{"128", "256", "512"}),
			},
			jsonResponse("QR code", map[string]any{
				"data_uri": prop("string", "Base64 PNG data URI"),
				"text":     prop("string", "Encoded text"),
			}),
		),
		"/util/password": getOp("generate_password",
			"Generate a secure random password",
			"Generates a cryptographically random password with configurable length and character set.",
			[]any{
				queryParam("length", "integer", false, "Password length (default 20, max 128)"),
				queryParam("symbols", "boolean", false, "Include symbols (default true)"),
				queryParam("numbers", "boolean", false, "Include numbers (default true)"),
				queryParam("upper", "boolean", false, "Include uppercase (default true)"),
			},
			jsonResponse("Password", map[string]any{
				"password": prop("string", "Generated password"),
				"length":   prop("integer", "Password length"),
				"entropy":  prop("number", "Approximate bits of entropy"),
			}),
		),
		"/util/random": getOp("random_number",
			"Generate random numbers",
			"Returns cryptographically random integers within a specified range.",
			[]any{
				queryParam("min", "integer", false, "Minimum value (default 1)"),
				queryParam("max", "integer", false, "Maximum value (default 100)"),
				queryParam("count", "integer", false, "How many numbers (default 1, max 20)"),
			},
			jsonResponse("Random numbers", map[string]any{
				"numbers": propArray("integer", "Random integers"),
				"min":     prop("integer", "Range minimum"),
				"max":     prop("integer", "Range maximum"),
			}),
		),
		"/util/word_count": getOp("count_words",
			"Count words and characters",
			"Analyzes text and returns word count, character count, line count, and estimated reading time.",
			[]any{queryParam("text", "string", true, "Text to analyze")},
			jsonResponse("Word count", map[string]any{
				"words":        prop("integer", "Word count"),
				"characters":   prop("integer", "Character count (with spaces)"),
				"chars_no_sp":  prop("integer", "Character count (no spaces)"),
				"lines":        prop("integer", "Line count"),
				"sentences":    prop("integer", "Approximate sentence count"),
				"reading_time": prop("string", "Estimated reading time at 200 wpm"),
			}),
		),

		"/news/feed": getOp("read_rss_feed",
			"Read an RSS or Atom news feed",
			"Fetches and parses any RSS, Atom, or JSON feed URL. Returns latest articles with titles, links, and summaries.",
			[]any{
				queryParam("url", "string", true, "Feed URL"),
				queryParam("limit", "integer", false, "Max items (default 10, max 30)"),
			},
			jsonResponse("Feed items", map[string]any{
				"feed_title": prop("string", "Feed/publication name"),
				"items":      propArray("object", "Feed items with title, link, published, summary"),
			}),
		),
		"/news/hn": getOp("hacker_news_top",
			"Get Hacker News stories",
			"Fetches current top/new/best stories from Hacker News with scores and comment counts.",
			[]any{
				queryParamEnum("type", false, "Story type (default: top)", []string{"top", "new", "best", "ask", "show", "job"}),
				queryParam("limit", "integer", false, "Number of stories (default 10, max 30)"),
			},
			jsonResponse("HN stories", map[string]any{
				"stories": propArray("object", "HN stories with title, url, score, comments, by, time"),
			}),
		),
	},
}
