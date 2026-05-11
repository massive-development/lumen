"""
Creates individual OpenWebUI native Python tools from the bitnet-tools REST API.
Run inside the OpenWebUI container:
  docker cp create_tools.py bitnet-openwebui:/tmp/create_tools.py
  docker exec bitnet-openwebui python3 /tmp/create_tools.py
"""
import importlib, importlib.util, json, sqlite3, sys, textwrap, time, types

DB_PATH = "/app/backend/data/webui.db"
ADMIN_USER_ID = None  # filled from DB

# ---------------------------------------------------------------------------
# Tool source definitions — each becomes one selectable entry in the UI
# ---------------------------------------------------------------------------

TOOLS = [
    {
        "id": "bitnet_weather",
        "name": "Weather",
        "description": "Current conditions and forecasts for any location.",
        "content": '''"""Get current weather conditions for any location."""
import requests
from typing import Optional
from pydantic import BaseModel, Field

class Tools:
    class Valves(BaseModel):
        tools_url: str = Field(default="http://bitnet-tools:8083")

    def __init__(self):
        self.valves = self.Valves()

    def get_weather(
        self,
        location: str,
        units: Optional[str] = "metric",
    ) -> str:
        """
        Get current weather for a city or location.

        :param location: City name or "lat,lon" coordinates.
        :param units: Unit system — "metric" (°C), "imperial" (°F), or "standard" (K).
        """
        r = requests.get(
            f"{self.valves.tools_url}/weather",
            params={"location": location, "units": units},
            timeout=10,
        )
        r.raise_for_status()
        return r.text
''',
    },
    {
        "id": "bitnet_web",
        "name": "Web",
        "description": "Fetch URLs, search the web, and look up Wikipedia articles.",
        "content": '''"""Browse the web, run searches, and read Wikipedia articles."""
import requests
from typing import Optional
from pydantic import BaseModel, Field

class Tools:
    class Valves(BaseModel):
        tools_url: str = Field(default="http://bitnet-tools:8083")

    def __init__(self):
        self.valves = self.Valves()

    def fetch_url(self, url: str) -> str:
        """
        Fetch and return the plain-text content of any URL.

        :param url: Full URL to fetch (e.g. https://example.com).
        """
        r = requests.get(
            f"{self.valves.tools_url}/web/fetch",
            params={"url": url},
            timeout=20,
        )
        r.raise_for_status()
        return r.text

    def search_web(self, query: str, limit: Optional[int] = 5) -> str:
        """
        Search the web and return a list of relevant results.

        :param query: Search query string.
        :param limit: Maximum number of results to return (default 5).
        """
        r = requests.get(
            f"{self.valves.tools_url}/web/search",
            params={"q": query, "limit": limit},
            timeout=15,
        )
        r.raise_for_status()
        return r.text

    def search_wikipedia(self, query: str, lang: Optional[str] = "en") -> str:
        """
        Search Wikipedia and return a summary of the best matching article.

        :param query: Topic or article name to look up.
        :param lang: Wikipedia language code (default "en").
        """
        r = requests.get(
            f"{self.valves.tools_url}/web/wikipedia",
            params={"q": query, "lang": lang},
            timeout=15,
        )
        r.raise_for_status()
        return r.text
''',
    },
    {
        "id": "bitnet_news",
        "name": "News",
        "description": "Read RSS/Atom feeds and browse Hacker News.",
        "content": '''"""Fetch news from RSS feeds and Hacker News."""
import requests
from typing import Optional
from pydantic import BaseModel, Field

class Tools:
    class Valves(BaseModel):
        tools_url: str = Field(default="http://bitnet-tools:8083")

    def __init__(self):
        self.valves = self.Valves()

    def get_rss_feed(self, url: str, limit: Optional[int] = 10) -> str:
        """
        Fetch and summarise an RSS or Atom news feed.

        :param url: Full URL of the RSS/Atom feed.
        :param limit: Maximum number of items to return (default 10).
        """
        r = requests.get(
            f"{self.valves.tools_url}/news/feed",
            params={"url": url, "limit": limit},
            timeout=15,
        )
        r.raise_for_status()
        return r.text

    def get_hacker_news(
        self,
        story_type: Optional[str] = "top",
        limit: Optional[int] = 10,
    ) -> str:
        """
        Fetch current stories from Hacker News.

        :param story_type: One of "top", "new", "best", "ask", "show", "job".
        :param limit: Number of stories to return (default 10).
        """
        r = requests.get(
            f"{self.valves.tools_url}/news/hn",
            params={"type": story_type, "limit": limit},
            timeout=15,
        )
        r.raise_for_status()
        return r.text
''',
    },
    {
        "id": "bitnet_time",
        "name": "Time & Timezone",
        "description": "Get the current time, convert between timezones, and calculate durations.",
        "content": '''"""Work with time, timezones, and date arithmetic."""
import requests
from typing import Optional
from pydantic import BaseModel, Field

class Tools:
    class Valves(BaseModel):
        tools_url: str = Field(default="http://bitnet-tools:8083")

    def __init__(self):
        self.valves = self.Valves()

    def get_current_time(self, timezone: Optional[str] = "") -> str:
        """
        Get the current date and time, optionally for a specific timezone.

        :param timezone: IANA timezone name, e.g. "America/New_York". Defaults to UTC.
        """
        params = {}
        if timezone:
            params["tz"] = timezone
        r = requests.get(f"{self.valves.tools_url}/time", params=params, timeout=5)
        r.raise_for_status()
        return r.text

    def convert_timezone(
        self,
        datetime_str: str,
        from_timezone: str,
        to_timezone: str,
    ) -> str:
        """
        Convert a datetime string from one timezone to another.

        :param datetime_str: ISO 8601 datetime string, e.g. "2024-06-15T14:30:00".
        :param from_timezone: Source IANA timezone, e.g. "Europe/London".
        :param to_timezone: Target IANA timezone, e.g. "Asia/Tokyo".
        """
        r = requests.get(
            f"{self.valves.tools_url}/time/convert",
            params={"datetime": datetime_str, "from": from_timezone, "to": to_timezone},
            timeout=5,
        )
        r.raise_for_status()
        return r.text

    def time_difference(self, from_time: str, to_time: Optional[str] = "") -> str:
        """
        Calculate the difference between two datetime strings.

        :param from_time: Start datetime in ISO 8601 format.
        :param to_time: End datetime in ISO 8601 format. Defaults to now.
        """
        params = {"from": from_time}
        if to_time:
            params["to"] = to_time
        r = requests.get(f"{self.valves.tools_url}/time/diff", params=params, timeout=5)
        r.raise_for_status()
        return r.text
''',
    },
    {
        "id": "bitnet_finance",
        "name": "Finance & Currency",
        "description": "Live exchange rates and currency conversion.",
        "content": '''"""Look up exchange rates and convert currencies."""
import requests
from typing import Optional
from pydantic import BaseModel, Field

class Tools:
    class Valves(BaseModel):
        tools_url: str = Field(default="http://bitnet-tools:8083")

    def __init__(self):
        self.valves = self.Valves()

    def get_exchange_rate(
        self,
        from_currency: str,
        to_currency: str,
        amount: Optional[float] = 1.0,
    ) -> str:
        """
        Get the current exchange rate and convert an amount between currencies.

        :param from_currency: Source currency code, e.g. "USD".
        :param to_currency: Target currency code, e.g. "EUR".
        :param amount: Amount to convert (default 1.0).
        """
        r = requests.get(
            f"{self.valves.tools_url}/finance/exchange",
            params={"from": from_currency, "to": to_currency, "amount": amount},
            timeout=10,
        )
        r.raise_for_status()
        return r.text

    def list_currencies(self) -> str:
        """List all supported currency codes and their names."""
        r = requests.get(f"{self.valves.tools_url}/finance/currencies", timeout=10)
        r.raise_for_status()
        return r.text
''',
    },
    {
        "id": "bitnet_math",
        "name": "Math & Units",
        "description": "Evaluate math expressions and convert between units of measurement.",
        "content": '''"""Evaluate mathematical expressions and convert units."""
import requests
from pydantic import BaseModel, Field

class Tools:
    class Valves(BaseModel):
        tools_url: str = Field(default="http://bitnet-tools:8083")

    def __init__(self):
        self.valves = self.Valves()

    def evaluate_expression(self, expression: str) -> str:
        """
        Evaluate a mathematical expression and return the result.

        :param expression: Math expression to evaluate, e.g. "sqrt(144) + 2^10".
        """
        r = requests.get(
            f"{self.valves.tools_url}/math/eval",
            params={"expr": expression},
            timeout=5,
        )
        r.raise_for_status()
        return r.text

    def convert_units(
        self,
        value: float,
        from_unit: str,
        to_unit: str,
    ) -> str:
        """
        Convert a value from one unit to another (length, mass, temperature, speed, etc.).

        :param value: Numeric value to convert.
        :param from_unit: Source unit, e.g. "miles", "kg", "fahrenheit".
        :param to_unit: Target unit, e.g. "km", "lbs", "celsius".
        """
        r = requests.get(
            f"{self.valves.tools_url}/math/convert",
            params={"value": value, "from": from_unit, "to": to_unit},
            timeout=5,
        )
        r.raise_for_status()
        return r.text
''',
    },
    {
        "id": "bitnet_system",
        "name": "System & Network",
        "description": "System information, public IP address, and DNS lookups.",
        "content": '''"""Query system info, public IP, and DNS records."""
import requests
from typing import Optional
from pydantic import BaseModel, Field

class Tools:
    class Valves(BaseModel):
        tools_url: str = Field(default="http://bitnet-tools:8083")

    def __init__(self):
        self.valves = self.Valves()

    def get_system_info(self) -> str:
        """Return CPU, memory, disk, and OS information for the host system."""
        r = requests.get(f"{self.valves.tools_url}/system/info", timeout=5)
        r.raise_for_status()
        return r.text

    def get_public_ip(self) -> str:
        """Return the public IP address of the server."""
        r = requests.get(f"{self.valves.tools_url}/network/ip", timeout=5)
        r.raise_for_status()
        return r.text

    def lookup_dns(self, hostname: str, record_type: Optional[str] = "A") -> str:
        """
        Perform a DNS lookup for a hostname.

        :param hostname: Domain to look up, e.g. "github.com".
        :param record_type: DNS record type: "A", "AAAA", "MX", "TXT", "CNAME", etc.
        """
        r = requests.get(
            f"{self.valves.tools_url}/network/dns",
            params={"host": hostname, "type": record_type},
            timeout=5,
        )
        r.raise_for_status()
        return r.text
''',
    },
    {
        "id": "bitnet_utilities",
        "name": "Utilities",
        "description": "Generate UUIDs, hash text, encode/decode Base64, format JSON, generate passwords, and more.",
        "content": '''"""General-purpose utilities: UUID, hash, base64, JSON, passwords, random numbers."""
import requests
from typing import Optional
from pydantic import BaseModel, Field

class Tools:
    class Valves(BaseModel):
        tools_url: str = Field(default="http://bitnet-tools:8083")

    def __init__(self):
        self.valves = self.Valves()

    def generate_uuid(self, count: Optional[int] = 1) -> str:
        """
        Generate one or more random UUIDs (v4).

        :param count: Number of UUIDs to generate (default 1).
        """
        r = requests.get(f"{self.valves.tools_url}/util/uuid", params={"count": count}, timeout=5)
        r.raise_for_status()
        return r.text

    def hash_text(self, text: str, algorithm: Optional[str] = "sha256") -> str:
        """
        Hash a string using a cryptographic hash algorithm.

        :param text: Input text to hash.
        :param algorithm: Hash algorithm — "md5", "sha1", "sha256", "sha512" (default "sha256").
        """
        r = requests.get(
            f"{self.valves.tools_url}/util/hash",
            params={"text": text, "algorithm": algorithm},
            timeout=5,
        )
        r.raise_for_status()
        return r.text

    def base64_encode_decode(self, text: str, mode: Optional[str] = "encode") -> str:
        """
        Encode or decode text using Base64.

        :param text: Input string to encode or decode.
        :param mode: "encode" to Base64-encode, "decode" to Base64-decode.
        """
        r = requests.get(
            f"{self.valves.tools_url}/util/base64",
            params={"text": text, "mode": mode},
            timeout=5,
        )
        r.raise_for_status()
        return r.text

    def format_json(self, json_string: str) -> str:
        """
        Pretty-print and validate a JSON string.

        :param json_string: Raw JSON text to format.
        """
        r = requests.get(
            f"{self.valves.tools_url}/util/json",
            params={"json": json_string},
            timeout=5,
        )
        r.raise_for_status()
        return r.text

    def generate_password(
        self,
        length: Optional[int] = 16,
        symbols: Optional[bool] = True,
        numbers: Optional[bool] = True,
        uppercase: Optional[bool] = True,
    ) -> str:
        """
        Generate a secure random password.

        :param length: Password length (default 16).
        :param symbols: Include special symbols (default True).
        :param numbers: Include digits (default True).
        :param uppercase: Include uppercase letters (default True).
        """
        r = requests.get(
            f"{self.valves.tools_url}/util/password",
            params={"length": length, "symbols": symbols, "numbers": numbers, "upper": uppercase},
            timeout=5,
        )
        r.raise_for_status()
        return r.text

    def random_number(
        self,
        min_value: Optional[int] = 0,
        max_value: Optional[int] = 100,
        count: Optional[int] = 1,
    ) -> str:
        """
        Generate one or more random integers within a range.

        :param min_value: Lower bound (inclusive, default 0).
        :param max_value: Upper bound (inclusive, default 100).
        :param count: How many random numbers to generate (default 1).
        """
        r = requests.get(
            f"{self.valves.tools_url}/util/random",
            params={"min": min_value, "max": max_value, "count": count},
            timeout=5,
        )
        r.raise_for_status()
        return r.text

    def count_words(self, text: str) -> str:
        """
        Count the words, characters, sentences, and paragraphs in a text.

        :param text: The text to analyse.
        """
        r = requests.get(
            f"{self.valves.tools_url}/util/word_count",
            params={"text": text},
            timeout=5,
        )
        r.raise_for_status()
        return r.text
''',
    },
    {
        "id": "bitnet_memory",
        "name": "Memory",
        "description": "Save and retrieve facts, preferences, and notes across conversations.",
        "content": '''"""Persistent memory — save and recall facts across conversations."""
import requests
from typing import Optional
from pydantic import BaseModel, Field

class Tools:
    class Valves(BaseModel):
        tools_url: str = Field(default="http://bitnet-tools:8083")

    def __init__(self):
        self.valves = self.Valves()

    def search_memory(self, query: str, limit: Optional[int] = 5) -> str:
        """
        Search saved memories for information relevant to a query.

        :param query: Natural-language search query.
        :param limit: Maximum number of results to return (default 5).
        """
        r = requests.get(
            f"{self.valves.tools_url}/memory/search",
            params={"q": query, "limit": limit},
            timeout=10,
        )
        r.raise_for_status()
        return r.text

    def save_memory(self, content: str, tags: Optional[str] = "") -> str:
        """
        Save a fact, note, or preference to long-term memory.

        :param content: The text to remember.
        :param tags: Comma-separated tags to categorise the memory (optional).
        """
        r = requests.post(
            f"{self.valves.tools_url}/memory/save",
            json={"content": content, "tags": tags},
            timeout=10,
        )
        r.raise_for_status()
        return r.text
''',
    },
]


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def load_module_from_source(source: str, module_name: str):
    spec = importlib.util.spec_from_loader(module_name, loader=None)
    mod = types.ModuleType(module_name)
    exec(compile(source, module_name, "exec"), mod.__dict__)
    return mod


def generate_specs(source: str, tool_id: str) -> list:
    from open_webui.utils.tools import get_tool_specs
    mod = load_module_from_source(source, tool_id)
    tool_instance = mod.Tools()
    return get_tool_specs(tool_instance)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    conn = sqlite3.connect(DB_PATH)
    cur = conn.cursor()

    # Get admin user
    cur.execute("SELECT id FROM user WHERE role='admin' LIMIT 1")
    row = cur.fetchone()
    if not row:
        print("ERROR: no admin user found")
        sys.exit(1)
    admin_id = row[0]
    print(f"Admin user: {admin_id}")

    now = int(time.time())
    created = 0
    updated = 0

    for tool in TOOLS:
        tid = tool["id"]
        name = tool["name"]
        source = tool["content"]

        try:
            specs = generate_specs(source, tid)
        except Exception as e:
            print(f"  WARN: spec generation failed for {tid}: {e}")
            specs = []

        meta = json.dumps({"description": tool["description"]})
        specs_json = json.dumps(specs)

        # Upsert
        cur.execute("SELECT id FROM tool WHERE id=?", (tid,))
        exists = cur.fetchone()
        if exists:
            cur.execute(
                """UPDATE tool SET name=?, content=?, specs=?, meta=?, updated_at=?
                   WHERE id=?""",
                (name, source, specs_json, meta, now, tid),
            )
            print(f"  updated  {tid} ({len(specs)} specs)")
            updated += 1
        else:
            cur.execute(
                """INSERT INTO tool (id, user_id, name, content, specs, meta, valves, created_at, updated_at)
                   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)""",
                (tid, admin_id, name, source, specs_json, meta, "{}", now, now),
            )
            print(f"  created  {tid} ({len(specs)} specs)")
            created += 1

    conn.commit()
    conn.close()
    print(f"\nDone — {created} created, {updated} updated.")
    print("Restart OpenWebUI to reload tools:  docker compose restart openwebui")


if __name__ == "__main__":
    main()
