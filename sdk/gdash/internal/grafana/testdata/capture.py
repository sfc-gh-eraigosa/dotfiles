"""Re-capture the lab Grafana fixtures (mono-gdash plan Task 0).

Usage: LOGIN=$USER python3 -I capture.py <out-dir> <dashboard-uid>...
Reads the Viewer token from ~/.config/gdash/lab.cred (never printed) and scrubs
node hostnames, the login, LAN addresses and token-looking strings."""
import collections, json, os, re, sys, time, urllib.error, urllib.request

BASE = "http://192.168.0.128:30300"
OUT = sys.argv[1]
UIDS = sys.argv[2:]
LOGIN = os.environ["LOGIN"]
NODE = {f"{LOGIN}-nano": "jetson1", f"{LOGIN}-spark": "spark1", f"{LOGIN}pi": "pi1", f"{LOGIN}gigabyte": "wslgiga1"}


def read_cred():
    with open(os.path.expanduser("~/.config/gdash/lab.cred")) as f:
        return f.read().strip()


def scrub(text):
    for k, v in NODE.items():
        text = text.replace(k, v)
    text = re.sub(re.escape(LOGIN), "operator", text, flags=re.I)
    text = re.sub(r"192\.168\.0\.(\d+)", r"192.0.2.\1", text)  # TEST-NET-1
    text = re.sub(r"glsa_[A-Za-z0-9_]+", "‹redacted›", text)
    return text


def call(method, path, body=None):
    req = urllib.request.Request(
        BASE + path, method=method,
        data=json.dumps(body).encode() if body is not None else None,
        headers={"Authorization": "Bearer " + read_cred(), "Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=20) as r:
        return json.load(r)


def save(name, obj):
    path = os.path.join(OUT, name)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as f:
        f.write(scrub(json.dumps(obj, indent=2, sort_keys=True, ensure_ascii=False)) + "\n")


def naive(expr):  # fixture queries only; gdash's real expansion is spec F2
    for a, b in (("${__rate_interval}", "1m"), ("$__rate_interval", "1m"),
                 ("${__interval}", "30s"), ("$__interval", "30s"), ("$__range", "1h")):
        expr = expr.replace(a, b)
    expr = re.sub(r'=~?\s*"\$\{?\w+(:\w+)?\}?"', '=~".*"', expr)
    return re.sub(r"\$\{?\w+(:\w+)?\}?", ".*", expr)


def ds_of(p, t):
    ds = t.get("datasource") or p.get("datasource") or {}
    return ds if isinstance(ds, dict) else {"type": "?", "uid": str(ds)}


save("health.json", call("GET", "/api/health"))
save("search.json", call("GET", "/api/search?type=dash-db"))
now = int(time.time() * 1000)
report = {}
for uid in UIDS:
    d = call("GET", f"/api/dashboards/uid/{uid}")
    save(f"dashboards/{uid}.json", d)
    dash = d["dashboard"]
    panels = []
    for p in dash.get("panels", []):
        panels.append(p)
        panels.extend(p.get("panels", []))  # collapsed rows hold their panels
    types = collections.Counter(p.get("type") for p in panels)
    dstypes = collections.Counter(ds_of(p, t).get("type", "?") for p in panels for t in p.get("targets", []))
    seen, queried = set(), 0
    for p in panels:
        kind = p.get("type")
        if kind in seen or kind == "row" or not p.get("targets"):
            continue
        targets = []
        for i, tg in enumerate(p["targets"]):
            ds = ds_of(p, tg)
            if ds.get("type") != "prometheus" or "expr" not in tg:
                continue
            dsuid = ds.get("uid") if "$" not in str(ds.get("uid")) else "prometheus"
            targets.append({"refId": tg.get("refId", chr(65 + i)), "datasource": {"type": "prometheus", "uid": dsuid},
                            "expr": naive(tg["expr"]), "range": True, "instant": False,
                            "intervalMs": 30000, "maxDataPoints": 240})
        if not targets:
            continue
        body = {"queries": targets, "from": str(now - 3600_000), "to": str(now)}
        try:
            resp = call("POST", "/api/ds/query", body)
        except urllib.error.HTTPError as e:
            resp = {"_http_error": e.code, "_body": e.read().decode()[:500]}
        save(f"queries/{uid}/{kind}-{p.get('id')}.json",
             {"panel": {"id": p.get("id"), "title": p.get("title"), "type": kind}, "request": body, "response": resp})
        seen.add(kind)
        queried += 1
    report[uid] = {"title": dash.get("title"), "schemaVersion": dash.get("schemaVersion"),
                   "panels": sum(types.values()), "types": dict(types), "datasources": dict(dstypes),
                   "vars": [v.get("name") + ":" + v.get("type", "") for v in dash.get("templating", {}).get("list", [])],
                   "queries_captured": queried}
print(scrub(json.dumps(report, indent=1, ensure_ascii=False)))
