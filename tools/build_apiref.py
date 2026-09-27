#!/usr/bin/env python3
"""Build assets/apiref/api.json, the offline API reference that `cav api` searches.

Inputs:
  --types DIR   a checkout of https://github.com/scenery-io/cavalry-types (MIT)
  --live FILE   optional JSON {"api": [...], "cavalry": [...], "ui": [...]} with the
                names that exist in a running Cavalry. Make it with:
                cav run --json --no-helpers -e "<list keys of api/cavalry/ui>"
  --notes FILE  tools/api-notes.json: our own notes and gotchas per function

Only names, parameter lists and return types are taken from cavalry-types. Its JSDoc
text repeats the Cavalry documentation, which is not ours to redistribute, so it is
not copied. `cav docs update` builds a local docs index on the user's machine instead.
"""

from __future__ import annotations

import argparse
import json
import re
from pathlib import Path

DECL = re.compile(r"^\s*(function|const|class)\s+(\w+)")
NAMESPACES = ["api", "cavalry", "ctx", "def", "render", "ui", "console"]
DOC_URL = {
    "api": "https://cavalry.studio/docs/tech-info/scripting/api-module/",
    "cavalry": "https://cavalry.studio/docs/tech-info/scripting/cavalry-module/",
    "ctx": "https://cavalry.studio/docs/tech-info/scripting/context-module/",
    "def": "https://cavalry.studio/docs/tech-info/scripting/deformer-module/",
    "render": "https://cavalry.studio/docs/tech-info/scripting/render-scripts/",
    "ui": "https://cavalry.studio/docs/tech-info/scripting/script-uis/",
    "console": "https://cavalry.studio/docs/tech-info/scripting/scripting-getting-started/",
}


def parse(path: Path, ns: str) -> list[dict]:
    lines = path.read_text(encoding="utf-8").splitlines()
    out = []
    i = 0
    depth_class = None
    while i < len(lines):
        line = lines[i]
        m = DECL.match(line)
        if not m or line.startswith("    "):  # skip class members (deeper indent)
            i += 1
            continue
        kind, name = m.group(1), m.group(2)
        if kind == "class":
            sig = [line.strip().rstrip("{").strip()]
            members = []
            i += 1
            while i < len(lines) and not re.match(r"^  \}", lines[i]):
                mm = re.match(r"^\s{4}(\w+)\s*\(([^)]*)\)\s*:\s*([^;]+);", lines[i])
                if mm:
                    members.append(f"{mm.group(1)}({mm.group(2)}): {mm.group(3).strip()}")
                mc = re.match(r"^\s{4}constructor\s*\(([^)]*)\)", lines[i])
                if mc:
                    members.insert(0, f"constructor({mc.group(1)})")
                i += 1
            out.append({"ns": ns, "kind": "class", "name": name, "sig": sig[0], "members": members})
            continue
        sig = [line.strip()]

        def balanced(parts: list[str]) -> bool:
            text = " ".join(parts)
            return text.count("(") == text.count(")") and text.count("{") == text.count("}")

        while not (sig[-1].endswith(";") and balanced(sig)) and i + 1 < len(lines):
            i += 1
            sig.append(lines[i].strip())
        s = re.sub(r"\s+", " ", " ".join(sig)).strip().rstrip(";")
        s = re.sub(r"^(function|const)\s+", "", s)
        out.append({"ns": ns, "kind": kind, "name": name, "sig": s})
        i += 1
    return out


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--types", required=True)
    ap.add_argument("--live")
    ap.add_argument("--notes", default=str(Path(__file__).with_name("api-notes.json")))
    ap.add_argument("--out", default=str(Path(__file__).parent.parent / "assets/apiref/api.json"))
    args = ap.parse_args()

    types_dir = Path(args.types) / "types" / "namespaces"
    live = json.loads(Path(args.live).read_text()) if args.live else {}
    if "value" in live:
        live = live["value"]
    notes = json.loads(Path(args.notes).read_text()) if Path(args.notes).exists() else {}

    entries = []
    seen = set()
    for ns in NAMESPACES:
        p = types_dir / f"{ns}.d.ts"
        if not p.exists():
            continue
        for e in parse(p, ns):
            key = f"{ns}.{e['name']}"
            if key in seen:
                # overloads: keep every signature
                prev = next(x for x in entries if f"{x['ns']}.{x['name']}" == key)
                prev.setdefault("overloads", []).append(e["sig"])
                continue
            seen.add(key)
            if ns in live:
                e["live"] = e["name"] in live[ns]
            if key in notes:
                e["note"] = notes[key]
            e["url"] = DOC_URL[ns]
            entries.append(e)
    # Names that exist in the running app but are missing from the types.
    for ns, names in live.items():
        for n in names:
            key = f"{ns}.{n}"
            if key not in seen:
                e = {"ns": ns, "kind": "function", "name": n, "sig": f"{n}(...)", "live": True, "untyped": True, "url": DOC_URL.get(ns, "")}
                if key in notes:
                    e["note"] = notes[key]
                entries.append(e)
    payload = {
        "source": "names and signatures: https://github.com/scenery-io/cavalry-types (MIT, (c) 2022 Remco Janssen); live check: Cavalry 2.7.2",
        "count": len(entries),
        "entries": entries,
    }
    Path(args.out).write_text(json.dumps(payload, indent=0, ensure_ascii=False))
    print(f"wrote {len(entries)} entries to {args.out}")


if __name__ == "__main__":
    main()
