#!/usr/bin/env python3
"""Generate models/reference_specs.go from Wikidata.

Run when the reference tables in models/reference.go gain entries:

    python scripts/build-reference-specs.py

Why Wikidata rather than Wikipedia: Wikidata is CC0, so there is no attribution
or share-alike obligation on an MIT codebase, and its claims are typed with units
rather than needing to be parsed out of prose. Fetching the Wikipedia article and
reading the specifications table is unreliable -- the articles are long enough
that a fetch truncates before reaching it.

QIDs are resolved from the curated Wikipedia article titles already in
reference.go, not by searching for the name. Searching is ambiguous: "F-15 Eagle"
also matches "F-15 Eagle fleet" and "F-15 Eagle prototype". Title lookup is exact.

Coverage is patchy by nature -- mass is absent for the F-15, for instance -- so
the generated table stores only what exists and the UI omits the rest.
"""

import json
import re
import sys
import time
import urllib.parse
import urllib.request
from pathlib import Path

API = "https://www.wikidata.org/w/api.php"
UA = "overlord-reference-builder/1.0 (github.com/MartinHell/overlord)"

ROOT = Path(__file__).resolve().parent.parent
REFERENCE = ROOT / "models" / "reference.go"
OUT = ROOT / "models" / "reference_specs.go"

# Wikidata property -> field on the generated struct.
PROPS = {
    "P2043": "LengthM",
    "P2050": "WingspanM",
    "P2048": "HeightM",
    "P2067": "MassKg",
    "P606": "FirstFlight",
    "P729": "ServiceEntry",
    "P1092": "TotalProduced",
    "P176": "Makers",
    "P2078": None,  # user manual, ignored; listed so it is obviously excluded
}

# Units we accept per numeric property, so a value in the wrong unit is dropped
# rather than silently mis-stated.
METRE = "Q11573"
KILOGRAM = "Q11570"
TONNE = "Q191118"

EXPECTED_UNITS = {
    "P2043": {METRE},
    "P2050": {METRE},
    "P2048": {METRE},
    "P2067": {KILOGRAM, TONNE},
}


def get(params):
    params = dict(params)
    params["format"] = "json"
    url = API + "?" + urllib.parse.urlencode(params)
    req = urllib.request.Request(url, headers={"User-Agent": UA})
    for attempt in range(4):
        try:
            with urllib.request.urlopen(req, timeout=40) as r:
                return json.load(r)
        except Exception as err:  # noqa: BLE001 - retry anything transient
            if attempt == 3:
                raise
            print(f"  retry after {err}", file=sys.stderr)
            time.sleep(2 * (attempt + 1))
    return {}


def chunks(items, n):
    for i in range(0, len(items), n):
        yield items[i : i + n]


def parse_sources(text, var_name):
    """Pull "DCS type": "Wikipedia_title" pairs out of a Go map literal."""
    m = re.search(r"var " + var_name + r" = map\[string\]string\{(.*?)\n\}", text, re.S)
    if not m:
        sys.exit(f"could not find {var_name} in reference.go")

    pairs = re.findall(r'"((?:[^"\\]|\\.)*)":\s*"((?:[^"\\]|\\.)*)"', m.group(1))
    return {k: urllib.parse.unquote(v).replace("_", " ") for k, v in pairs}


def resolve_qids(titles):
    """Map Wikipedia title -> QID exactly, via the enwiki sitelink."""
    out = {}
    for batch in chunks(sorted(set(titles)), 40):
        data = get({
            "action": "wbgetentities",
            "sites": "enwiki",
            "titles": "|".join(batch),
            "props": "info",
        })
        for qid, ent in (data.get("entities") or {}).items():
            if qid.startswith("Q") and "missing" not in ent:
                title = (ent.get("sitelinks", {}).get("enwiki", {}) or {}).get("title")
                out[title or qid] = qid
        # sitelinks are not returned with props=info, so ask again per batch
        data = get({
            "action": "wbgetentities",
            "sites": "enwiki",
            "titles": "|".join(batch),
            "props": "sitelinks",
            "sitefilter": "enwiki",
        })
        for qid, ent in (data.get("entities") or {}).items():
            title = (ent.get("sitelinks", {}).get("enwiki", {}) or {}).get("title")
            if title:
                out[title] = qid
    return out


def best_amount(claims, prop):
    """Take the first claim with an acceptable unit, preferring preferred rank."""
    entries = claims.get(prop) or []
    entries.sort(key=lambda c: 0 if c.get("rank") == "preferred" else 1)

    for c in entries:
        val = (c.get("mainsnak", {}).get("datavalue") or {}).get("value")
        if not isinstance(val, dict) or "amount" not in val:
            continue

        unit = (val.get("unit") or "").rsplit("/", 1)[-1]
        allowed = EXPECTED_UNITS.get(prop)
        if allowed and unit not in allowed:
            continue

        amount = float(val["amount"])
        if prop == "P2067" and unit == TONNE:
            amount *= 1000
        return amount
    return None


def best_time(claims, prop):
    for c in claims.get(prop) or []:
        val = (c.get("mainsnak", {}).get("datavalue") or {}).get("value")
        if isinstance(val, dict) and val.get("time"):
            # "+1972-07-27T00:00:00Z" -> "1972-07-27", trimming unknown parts
            t = val["time"].lstrip("+")[:10]
            return t.replace("-00-00", "").replace("-00", "")
    return ""


def best_quantity(claims, prop):
    for c in claims.get(prop) or []:
        val = (c.get("mainsnak", {}).get("datavalue") or {}).get("value")
        if isinstance(val, dict) and "amount" in val:
            return int(float(val["amount"]))
    return 0


def entity_ids(claims, prop):
    ids = []
    for c in claims.get(prop) or []:
        val = (c.get("mainsnak", {}).get("datavalue") or {}).get("value")
        if isinstance(val, dict) and val.get("id"):
            ids.append(val["id"])
    return ids


def fetch_claims(qids):
    out = {}
    for batch in chunks(sorted(set(qids)), 25):
        data = get({"action": "wbgetentities", "ids": "|".join(batch), "props": "claims"})
        for qid, ent in (data.get("entities") or {}).items():
            out[qid] = ent.get("claims") or {}
    return out


def fetch_labels(qids):
    out = {}
    for batch in chunks(sorted(set(qids)), 45):
        data = get({
            "action": "wbgetentities",
            "ids": "|".join(batch),
            "props": "labels",
            "languages": "en",
        })
        for qid, ent in (data.get("entities") or {}).items():
            label = (ent.get("labels", {}).get("en") or {}).get("value")
            if label:
                out[qid] = label
    return out


def go_string(s):
    return '"' + s.replace("\\", "\\\\").replace('"', '\\"') + '"'


def build(kind, sources, qid_by_title, claims_by_qid, labels):
    rows = []
    for dcs_type, title in sorted(sources.items()):
        qid = qid_by_title.get(title)
        if not qid:
            print(f"  no QID for {kind} {dcs_type!r} (title {title!r})", file=sys.stderr)
            continue

        claims = claims_by_qid.get(qid) or {}
        makers = [labels[m] for m in entity_ids(claims, "P176") if m in labels]

        spec = {
            "QID": qid,
            "LengthM": best_amount(claims, "P2043"),
            "WingspanM": best_amount(claims, "P2050"),
            "HeightM": best_amount(claims, "P2048"),
            "MassKg": best_amount(claims, "P2067"),
            "FirstFlight": best_time(claims, "P606"),
            "ServiceEntry": best_time(claims, "P729"),
            "TotalProduced": best_quantity(claims, "P1092"),
            "Makers": makers,
        }

        if not any(v for k, v in spec.items() if k != "QID"):
            print(f"  {kind} {dcs_type!r} ({qid}) has no usable claims", file=sys.stderr)

        rows.append((dcs_type, spec))
    return rows


def emit(rows, var):
    lines = [f"var {var} = map[string]Specs{{"]
    for dcs_type, s in rows:
        parts = [f"QID: {go_string(s['QID'])}"]
        for field in ("LengthM", "WingspanM", "HeightM", "MassKg"):
            if s[field]:
                parts.append(f"{field}: {s[field]:g}")
        for field in ("FirstFlight", "ServiceEntry"):
            if s[field]:
                parts.append(f"{field}: {go_string(s[field])}")
        if s["TotalProduced"]:
            parts.append(f"TotalProduced: {s['TotalProduced']}")
        if s["Makers"]:
            joined = ", ".join(go_string(m) for m in s["Makers"])
            parts.append(f"Makers: []string{{{joined}}}")
        lines.append(f"\t{go_string(dcs_type)}: {{{', '.join(parts)}}},")
    lines.append("}")
    return "\n".join(lines)


def main():
    text = REFERENCE.read_text(encoding="utf-8")
    units = parse_sources(text, "unitSources")
    weapons = parse_sources(text, "weaponSources")
    print(f"{len(units)} unit titles, {len(weapons)} weapon titles")

    qid_by_title = resolve_qids(list(units.values()) + list(weapons.values()))
    print(f"resolved {len(qid_by_title)} QIDs")

    claims = fetch_claims(qid_by_title.values())
    print(f"fetched claims for {len(claims)} entities")

    maker_ids = {m for c in claims.values() for m in entity_ids(c, "P176")}
    labels = fetch_labels(maker_ids)
    print(f"resolved {len(labels)} manufacturer labels")

    unit_rows = build("unit", units, qid_by_title, claims, labels)
    weapon_rows = build("weapon", weapons, qid_by_title, claims, labels)

    header = '''// Code generated by scripts/build-reference-specs.py. DO NOT EDIT.
//
// Sourced from Wikidata, which is CC0, so these facts carry no attribution or
// share-alike obligation. Values are whatever Wikidata actually holds: coverage
// is uneven, absent fields are omitted rather than guessed, and numbers in an
// unexpected unit are dropped rather than converted blindly. QID is recorded so
// any value can be traced back to its entity.
//
// Regenerate with: python scripts/build-reference-specs.py

package models

// Specs holds the physical facts for one type. Zero values mean "not recorded".
type Specs struct {
	QID           string
	LengthM       float64
	WingspanM     float64
	HeightM       float64
	MassKg        float64
	FirstFlight   string
	ServiceEntry  string
	TotalProduced int
	Makers        []string
}

'''

    OUT.write_text(
        header + emit(unit_rows, "unitSpecs") + "\n\n" + emit(weapon_rows, "weaponSpecs") + "\n",
        encoding="utf-8",
        newline="\n",
    )
    print(f"wrote {OUT} with {len(unit_rows)} units and {len(weapon_rows)} weapons")


if __name__ == "__main__":
    main()
