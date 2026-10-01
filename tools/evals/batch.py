"""Run a batch of evals serially (one Cavalry), then score each run.

  python batch.py --iter 1 --arms plugin,baseline --models zai/glm-5.3-flash [--tasks a,b] [--skip-existing]
"""

import argparse
import hashlib
import json
import shutil
import subprocess
import sys
import time
from pathlib import Path

from evalpaths import CAV, CODE as HERE, DATA, REPO, TASKS


def slug(s):
    return s.replace("/", "_").replace(":", "_")


def freeze(iteration):
    """Freeze the CLI and the skill for this iteration, so edits made while it runs do not leak in."""
    snap = DATA / "snapshots" / f"iter{iteration}"
    if (snap / "bin" / "cav").exists():
        print(f"using snapshot {snap} (commit {(snap / 'commit').read_text().strip()})", flush=True)
        return
    (snap / "bin").mkdir(parents=True, exist_ok=True)
    shutil.copy2(CAV, snap / "bin" / "cav")
    shutil.copytree(REPO / "plugins" / "cavalry" / "skills" / "cavalry", snap / "skill", dirs_exist_ok=True)
    # Enabled Pi hooks can invoke build-brief even for a non-Gradle bash command.
    # Freeze the helper inside the runtime grant rather than expose Homebrew folders.
    helper = shutil.which("build-brief")
    if helper:
        runtime = snap / "runtime" / "bin"
        runtime.mkdir(parents=True)
        target = runtime / "build-brief"
        shutil.copy2(helper, target)
        (snap / "runtime.json").write_text(json.dumps({"build-brief": {
            "source": str(Path(helper).resolve()), "sha256": hashlib.sha256(target.read_bytes()).hexdigest()
        }}, indent=2) + "\n")
    commit = subprocess.run(["git", "-C", str(REPO), "rev-parse", "--short", "HEAD"], capture_output=True, text=True).stdout.strip()
    dirty = subprocess.run(["git", "-C", str(REPO), "status", "--porcelain", "--", "cmd", "internal", "assets", "plugins"],
                           capture_output=True, text=True).stdout.strip()
    (snap / "commit").write_text(commit + (" (with uncommitted changes)" if dirty else "") + "\n")
    print(f"froze {snap} at commit {commit}", flush=True)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--iter", required=True)
    ap.add_argument("--arms", default="plugin")
    ap.add_argument("--models", default="zai/glm-5.3-flash")
    ap.add_argument("--tasks", default="")
    ap.add_argument("--skip-existing", action="store_true")
    ap.add_argument("--timeout", type=int, default=1800)
    ap.add_argument("--via", default="pi", choices=["pi", "pioneer"])
    args = ap.parse_args()
    freeze(args.iter)
    spec = json.loads(TASKS.read_text())
    tasks = [t["id"] for t in spec["tasks"]]
    if args.tasks:
        tasks = [t for t in tasks if t in args.tasks.split(",")]
    for model in args.models.split(","):
        for task in tasks:
            for arm in args.arms.split(","):
                run = DATA / "runs" / f"iter{args.iter}" / task / f"{arm}__{slug(model)}"
                if args.skip_existing and (run / "score.json").exists():
                    meta = json.loads((run / "meta.json").read_text())
                    if "environmentChanged" in meta:
                        sys.exit(f"environment changed in {run}; stop and inspect meta.json")
                    continue
                t0 = time.time()
                print(f"== {task} / {arm} / {model}", flush=True)
                r = subprocess.run([sys.executable, str(HERE / "run.py"), "--iter", args.iter, "--task", task, "--arm", arm,
                                    "--model", model, "--timeout", str(args.timeout), "--via", args.via, "--force"])
                if r.returncode != 0:
                    sys.exit(f"run harness failed with exit {r.returncode}: {run}")
                meta = json.loads((run / "meta.json").read_text())
                if "environmentChanged" in meta:
                    sys.exit(f"environment changed in {run}; stop and inspect meta.json")
                if meta["exit"] != 0 and not meta["timedOut"]:
                    sys.exit(f"agent/controller failed with exit {meta['exit']}: {run}; inspect stderr.txt")
                subprocess.run(["uv", "run", "--no-project", "--with", "numpy", "--with", "pillow", "python",
                                str(HERE / "score.py"), str(run)], check=True)
                print(f"   done in {time.time() - t0:.0f} s", flush=True)


if __name__ == "__main__":
    main()
