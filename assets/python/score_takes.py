# Scores music takes for cav listen --score. Run by cav through `uv run`, never directly.
#   args: <prompt> <wav48k> [<wav48k> ...]   (cav converts every take to 48 kHz mono WAV)
# Prints one JSON object: {"takes": [{"path", "aesthetics": {CE, CU, PC, PQ}, "clap"}]}
#   aesthetics: Meta's Audiobox Aesthetics (0-10): CE content enjoyment, CU content usefulness,
#     PC production complexity, PQ production quality.
#   clap: cosine similarity between the prompt and the audio in LAION CLAP's space (-1..1);
#     only the ranking between takes means something, not the absolute value.
import json
import sys
import wave

import numpy as np


def load(path):
    with wave.open(path, "rb") as w:
        frames = w.readframes(w.getnframes())
    return np.frombuffer(frames, dtype="<i2").astype(np.float32) / 32768.0


def vec(x, *names):
    """A projected embedding: older transformers return a tensor, newer an output object."""
    import torch

    if torch.is_tensor(x):
        return x
    for n in names + ("pooler_output",):
        v = getattr(x, n, None)
        if v is not None:
            return v
    raise TypeError(f"no embedding in {type(x).__name__}")


def main():
    prompt, paths = sys.argv[1], sys.argv[2:]
    out = [{"path": p} for p in paths]

    import torch
    from audiobox_aesthetics.infer import initialize_predictor

    # Pass the audio in memory: torchaudio's file loader now needs torchcodec.
    predictor = initialize_predictor()
    batch = [{"path": torch.from_numpy(load(p).copy())[None, :], "sample_rate": 48000} for p in paths]
    for o, s in zip(out, predictor.forward(batch)):
        o["aesthetics"] = {k: round(float(v), 3) for k, v in s.items()}

    if prompt:
        from transformers import ClapModel, ClapProcessor

        name = "laion/clap-htsat-unfused"
        model, proc = ClapModel.from_pretrained(name), ClapProcessor.from_pretrained(name)
        with torch.no_grad():
            text = vec(model.get_text_features(**proc(text=[prompt], return_tensors="pt")), "text_embeds")
            text = text / text.norm(dim=-1, keepdim=True)
            for o in out:
                audio = load(o["path"])[: 48000 * 30]  # CLAP looks at the first 10 s; 30 s is plenty
                try:
                    inputs = proc(audio=[audio], sampling_rate=48000, return_tensors="pt")
                except TypeError:
                    inputs = proc(audios=[audio], sampling_rate=48000, return_tensors="pt")
                emb = vec(model.get_audio_features(**inputs), "audio_embeds")
                emb = emb / emb.norm(dim=-1, keepdim=True)
                o["clap"] = round(float((emb @ text.T).item()), 4)

    json.dump({"takes": out}, sys.stdout)


main()
