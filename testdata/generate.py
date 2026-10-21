#!/usr/bin/env python3
"""Regenerate the sample logs in this repo. Deterministic: same seed, same bytes.

    python3 testdata/generate.py
"""
import json
import os
import random
import re

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
random.seed(6767)

ips = ["93.184.216.34", "10.0.4.19", "172.16.8.203", "66.249.66.1",
       "192.168.1.44", "45.155.205.233", "104.28.14.77", "8.8.4.4"]
methods = ["GET", "GET", "GET", "POST", "PUT", "DELETE", "HEAD"]
paths = ["/api/v1/items/42", "/api/v1/items", "/health", "/login",
         "/static/app.js", "/api/v1/orders/9182", "/favicon.ico", "/metrics"]
statuses = [200, 200, 200, 200, 201, 204, 301, 304, 400, 401, 403, 404, 429, 500, 502]
agents = [
    "curl/8.1",
    "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/126.0 Safari/537.36",
    "python-requests/2.32.3",
    "kube-probe/1.30",
    "Go-http-client/2.0",
    "sqlmap/1.8",
]
refs = ["-", "-", "-", "https://example.com/docs", "https://news.ycombinator.com/item?id=41"]
days = ["10/Oct/2026", "11/Oct/2026", "12/Oct/2026"]
levels = ["info", "info", "info", "debug", "warn", "error"]
msgs = ["request handled", "cache miss", "order created", "token refreshed",
        "slow query", "payment failed"]

nginx = []
for _ in range(120):
    status = random.choice(statuses)
    body = 0 if status in (204, 304) else random.randint(120, 48000)
    nginx.append('%s - - [%s:%02d:%02d:%02d +0700] "%s %s HTTP/1.1" %d %d "%s" "%s"' % (
        random.choice(ips), random.choice(days), random.randint(0, 23),
        random.randint(0, 59), random.randint(0, 59), random.choice(methods),
        random.choice(paths), status, body, random.choice(refs), random.choice(agents)))
# Pinned line: examples/nginx/access_test.go asserts these exact values.
nginx[7] = ('93.184.216.34 - - [10/Oct/2026:04:11:02 +0700] '
            '"GET /api/v1/items/42 HTTP/1.1" 200 512 "-" "curl/8.1"')

kv = []
for _ in range(80):
    kv.append('time="2026-10-%02dT%02d:%02d:%02d+07:00" level=%s msg="%s" '
              'service=checkout status=%d duration_ms=%.2f trace_id=%08x-%04x-%04x-%04x-%012x' % (
                  random.randint(10, 12), random.randint(0, 23), random.randint(0, 59),
                  random.randint(0, 59), random.choice(levels), random.choice(msgs),
                  random.choice([200, 200, 201, 204, 400, 401, 404, 500, 503]),
                  random.uniform(0.4, 180.0), random.getrandbits(32),
                  random.getrandbits(16), random.getrandbits(16),
                  random.getrandbits(16), random.getrandbits(48)))

events = []
for _ in range(60):
    events.append(json.dumps({
        "ts": "2026-10-%02dT%02d:%02d:%02d.123Z" % (
            random.randint(10, 12), random.randint(0, 23),
            random.randint(0, 59), random.randint(0, 59)),
        "level": random.choice(levels),
        "service": "gateway",
        "http": {
            "method": random.choice(["GET", "POST"]),
            "path": random.choice(paths),
            "status": random.choice([200, 404, 500]),
        },
        "latency_ms": round(random.uniform(0.5, 95.0), 3),
        "trace_id": "%032x" % random.getrandbits(128),
    }, separators=(",", ":")))

# Drift fixture: same shape as access.log, but body_bytes widens to float and one
# extra quoted field appears. `logsynth drift` must report exactly that.
drift_re = re.compile(
    r'^(?P<pre>.*?) (?P<status>\d+) (?P<bytes>\d+) "(?P<ref>[^"]*)" "(?P<ua>[^"]*)"$')
drift = []
for line in nginx[:40]:
    match = drift_re.match(line)
    assert match, line
    drift.append('%s %s %s.5 "%s" "%s" "request_time=0.004"' % (
        match.group("pre"), match.group("status"), match.group("bytes"),
        match.group("ref"), match.group("ua")))

outputs = {
    "examples/nginx/access.log": nginx,
    "examples/app/app.log": kv,
    "examples/events/events.jsonl": events,
    "testdata/nginx_drift.log": drift,
}
for rel, lines in outputs.items():
    path = os.path.join(ROOT, rel)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as handle:
        handle.write("\n".join(lines) + "\n")
    print("wrote %s (%d lines)" % (rel, len(lines)))
