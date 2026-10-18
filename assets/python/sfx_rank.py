# Ranks local sound files against a description with earshot's CLAP model, for
# cav sfx search --by-sound. Run by cav through `uv run`, never directly.
#   args: <earshot home> <query> <paths file> — one path per line (a library of a
#   thousand paths would overflow the OS command-line limit)
# Prints one JSON object:
#   {"results": [{"path", "z", "cosine", "verdict", "soundsMostLike", "nearSilent"}],
#    "skipped": [<basename> ...]}
# The first run downloads the models (~1.4 GB) into the earshot home; after that it works
# offline. Embeddings are cached there per file (path + size + mtime + content sample), so
# only new or changed files cost time.
import json
import os
import sys

os.environ["EARSHOT_HOME"] = sys.argv[1]  # earshot reads it when it is first imported


def main():
    query = sys.argv[2]
    with open(sys.argv[3]) as f:
        paths = [line.rstrip("\n") for line in f if line.strip()]
    from earshot.search import rank

    r = rank(paths, query)
    json.dump(
        {
            "results": [
                {
                    "path": e["path"],
                    "z": e["z"],
                    "cosine": e["cosine"],
                    "verdict": e["verdict"],
                    "soundsMostLike": e["sounds_most_like"],
                    "nearSilent": e["near_silent"],
                }
                for e in r["results"]
            ],
            "skipped": [os.path.basename(b.get("path", "?")) for b in r["skipped"]],
        },
        sys.stdout,
    )


main()
