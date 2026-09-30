"""Combine scores and vision grades of one iteration into results.json and summary.md.

  python summary.py --iter 1

Vision grades live in runs/iter<N>/grades.json:
  {"<task>/<arm>__<model>": {"brief": 1-5, "design": 1-5, "motion": 1-5, "polish": 1-5, "notes": "..."}}
A run passes when every automatic gate passes, the grade average is at least 3.0 and brief >= 3.
"""

import argparse
import collections
import json
from pathlib import Path

from evalpaths import DATA as EVALS


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--iter", required=True)
    args = ap.parse_args()
    root = EVALS / "runs" / f"iter{args.iter}"
    grades = json.loads((root / "grades.json").read_text()) if (root / "grades.json").exists() else {}
    rows = []
    for sc in sorted(root.glob("*/*/score.json")):
        s = json.loads(sc.read_text())
        key = f"{sc.parent.parent.name}/{sc.parent.name}"
        g = grades.get(key)
        vis = None
        if g:
            vis = round(sum(g[k] for k in ("brief", "design", "motion", "polish")) / 4, 2)
        passed = s["autoPass"] and g is not None and vis >= 3.0 and g["brief"] >= 3
        m = s["metrics"]
        rows.append({
            "key": key, "task": s["task"], "arm": s["arm"], "model": s["model"], "autoPass": s["autoPass"],
            "failedGates": [k for k, v in s["gates"].items() if not v], "vision": vis, "grade": g, "pass": passed,
            "wall": m.get("wallSeconds"), "tokens": (m.get("tokens") or {}).get("total"),
            "scriptErrors": m.get("scriptErrors"), "cavCalls": m.get("cavCalls"),
            "eased": (m.get("scene") or {}).get("easedRatio"), "onBeat": m.get("onsetsOnBeat"),
            "downbeats": m.get("downbeatsHit"), "native": (m.get("scene") or {}).get("native"),
        })
    (root / "results.json").write_text(json.dumps(rows, indent=2))
    by = collections.defaultdict(list)
    for r in rows:
        by[(r["model"], r["arm"])].append(r)
    lines = [f"# Iteration {args.iter}", "", "| Model | Arm | Runs | Passed | Auto gates passed | Mean vision | Mean wall (s) | Mean tokens | Script errors |", "|---|---|---|---|---|---|---|---|---|"]
    for (model, arm), rs in sorted(by.items()):
        vis = [r["vision"] for r in rs if r["vision"] is not None]
        lines.append(f"| {model} | {arm} | {len(rs)} | {sum(r['pass'] for r in rs)} | {sum(r['autoPass'] for r in rs)} | "
                     f"{(sum(vis) / len(vis)) if vis else float('nan'):.2f} | {sum(r['wall'] or 0 for r in rs) / len(rs):.0f} | "
                     f"{sum(r['tokens'] or 0 for r in rs) / len(rs):.0f} | {sum(r['scriptErrors'] or 0 for r in rs)} |")
    lines += ["", "| Run | Pass | Failed gates | Vision | Eased | On beat | Downbeats | Notes |", "|---|---|---|---|---|---|---|---|"]
    for r in rows:
        notes = (r["grade"] or {}).get("notes", "")
        lines.append(f"| {r['key']} | {'yes' if r['pass'] else 'no'} | {', '.join(r['failedGates']) or '-'} | {r['vision']} | "
                     f"{r['eased']} | {r['onBeat']} | {r['downbeats']} | {notes} |")
    (root / "summary.md").write_text("\n".join(lines) + "\n")
    print("\n".join(lines))


if __name__ == "__main__":
    main()
