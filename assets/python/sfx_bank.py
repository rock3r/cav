# Lists and renders earshot's shipped sound bank, for cav sfx search --service bank and
# cav sfx get bank:<id>. Run by cav through `uv run`, never directly. Rendering needs
# numpy / scipy only: no models, no network.
#   args: <earshot source dir> list
#     -> {"version": ..., "categories": {cat: {"target", "use"}}, "sounds": [{id,
#         category, member, type, dur_s, kinds, notes, source (fit entries), knobs}]}
#   args: <earshot source dir> render <id> <out.wav> [<dur seconds|->] [<seed|->]
#     -> {"path", "seconds", "checks": {"ok", "warnings", "notes", "summary"},
#         "source" (fit entries)}
import json
import sys

sys.path.insert(0, sys.argv[1])  # the earshot source checkout, ahead of any installed one


def _flat(items):
    # the audit's warnings and notes are {"code", "msg"} dicts; flatten them for cav
    out = []
    for it in items or []:
        if isinstance(it, dict):
            out.append(f"{it.get('code', '?')}: {it.get('msg', '')}")
        else:
            out.append(str(it))
    return out


def main():
    from earshot.forge import bank

    if sys.argv[2] == "list":
        cats = {}
        for s in bank.list():
            info = bank.category_info(s["category"])
            if s["category"] not in cats:
                cats[s["category"]] = {"target": info.get("target") or "", "use": info.get("use") or ""}
        json.dump({"version": bank.VERSION, "categories": cats, "sounds": bank.list()}, sys.stdout)
        return

    _, _, sid, out = sys.argv[1:5]
    dur = float(sys.argv[5]) if len(sys.argv) > 5 and sys.argv[5] != "-" else None
    seed = int(sys.argv[6]) if len(sys.argv) > 6 and sys.argv[6] != "-" else None
    y, meta = bank.render(sid, seed=seed, dur=dur)

    import numpy as np
    import scipy.io.wavfile as wav

    wav.write(out, 48000, np.asarray(y, np.float32))
    checks = meta.get("checks") or {}
    entry = bank.get(sid)
    json.dump(
        {
            "path": out,
            "seconds": round(len(y) / 48000, 3),
            "checks": {
                "ok": bool(checks.get("ok", True)),
                "warnings": _flat(checks.get("warnings")),
                "notes": _flat(checks.get("notes")),
                "summary": checks.get("summary") or "",
            },
            "source": entry.get("source"),
        },
        sys.stdout,
    )


main()
