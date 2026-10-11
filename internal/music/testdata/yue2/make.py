import sys, json
from pathlib import Path
S = Path(sys.argv[1]); out = Path(sys.argv[2])
sys.path.insert(0, sys.argv[3])  # tools/yue2-server
import yue2_server
H = yue2_server.helper_modules(S)

def group(section, vocal, ins, extra=""):
    lines = ([f"% {section}"] if section else [])
    lines += ["V: Vocal"] + ([extra] if extra else []) + ["|".join(vocal) + "|", "V: Ins"] + ([extra] if extra else []) + ["|".join(ins) + "|"]
    return lines

head = ["X:1", "T:", "M:4/4", "L:1/16", "Q:1/4=120",
        'V: Vocal clef=treble name="Vocal Melody" snm="Vocal"',
        'V: Ins clef=treble name="Ins Melody" snm="Inst."', "K:G"]
body = []
body += group("intro", ['"G"z16', '"C"z16', '"D"z16', '"G"z16'], ["G4B4d4g4", "c4e4g4e4", "d4^f4a4f4", "G16"])
V1 = ['"G"B4d4B4A4', '"Em"G4E4G8', '"C"c4B4A4G4', '"D"A8z8']
I1 = ["G,8z8", "Z", "C8E4G4", "D,16"]
body += group("verse", V1, I1)
body += group(None, ['"G"B2c2d4e4d4', '"Em"G4^F4E8-', 'E4"C"G4A4B4', '"D7"A8^c8'], ["Z", "E,16", "z8C8", "D8z8"])
# Ins notes are tied across a phrase boundary (chorus) and into the outro (bridge), so a
# cut there must keep the held note.
body += group("chorus", ['"C"e4d4c4B4', '"G"d8B8', '"Am"c4B4A4G4', '"D"A8z8'], ["c16", "B,16", "A,8C8", "D,16-"])
body += group(None, ['"C"z8g4e4', '"G"d4B4G4B4', '"D"A4d4^f4a4', '"G"g16'], ["D,8e8", "Z", "d16", "G,16"])
body += group("bridge", ['"Bm"f4e4d4c4', '"A"c8A8', '"G"B4A4G4F4', '"A"E16'], ["B,16", "A,16", "G,16", "A,8D,8-"], extra="K:D")
body += group("outro", ['"G"z16', '"C"c4e4g4e4', '"D"d16', '"G"G16'], ["D,8G,8", "C16", "D,16", "G,16"], extra="K:G")
text = "\n".join(head + body) + "\n"
H.parse_abc(text)
out.mkdir(exist_ok=True)
(out / "planned.abc").write_text(text)
conv, transfer = H.convert_score(text, overlap="vocal", keep_chords=True)
(out / "converted.abc").write_text(conv)
rep = H.report(H.validate_score(conv))
meta = {"planned_seconds": rep["nominal_duration_seconds"], "bpm": rep["bpm"], "vocal_notes_moved": transfer["vocal_notes_before"],
        "lyrics": H.lyric_tags(conv), "style": yue2_server.instrumental_style("warm synthwave, 112 BPM"), "fits": {}}
for sec in (5, 20, 30, 40, 45, 60, 90):
    fitted, nominal = yue2_server.fit_score(H, conv, sec)
    H.validate_score(fitted)
    (out / f"fit-{sec}.abc").write_text(fitted)
    meta["fits"][str(sec)] = nominal
# An instrumental score whose first phrase is silent: a short cut must still keep notes.
silent, _ = H.compile_events({"bpm": 120, "key": "C", "bars": [{"meter": "4/4", "section": "intro"}] + [{}] * 3
                              + [{"section": "verse"}] + [{}] * 7,
                              "notes": [[str(16 + 2 * i), "2", 60 + i % 5] for i in range(16)], "chords": []})
(out / "silent.abc").write_text(silent)
fitted, nominal = yue2_server.fit_score(H, silent, 8)
(out / "silent-fit-8.abc").write_text(fitted)
meta["silent_fit_8"] = nominal
(out / "expected.json").write_text(json.dumps(meta, indent=1) + "\n")
print(json.dumps(meta, indent=1))
