"""Run one eval: a task, an arm and a model, with the pi agent.

  python run.py --iter 1 --task logo-sting --arm plugin --model zai/glm-5.3-flash [--via pi|pioneer]

Arms:
  plugin    the cavalry skill is loaded (pi --skill), cav on PATH
  baseline  no skill, cav on PATH (the prompt says cav exists)
  mcp       no skill, no cav; the upstream cavalry-mcp server through a pi MCP adapter

Each run gets its own folder: runs/iter<N>/<task>/<arm>__<model>/ with work/ (the agent's cwd),
events.jsonl (pi JSON events), cav.jsonl (every cav call), meta.json.
"""

import argparse
import hashlib
import json
import os
import shutil
import signal
import subprocess
import sys
import time
from pathlib import Path

from evalpaths import CAV, CAVALRY_MCP_DIR, DATA, FIXTURES, MCP_TOOLS, REPO, TASKS

SKILL = REPO / "plugins" / "cavalry" / "skills" / "cavalry"
ADAPTER = MCP_TOOLS / "node_modules" / "pi-mcp-adapter"


def slug(s):
    return s.replace("/", "_").replace(":", "_")


def cav(*args, timeout=120):
    return subprocess.run([str(CAV), *args], capture_output=True, text=True, timeout=timeout)


BRIDGE_PORTS = (8722, 8723)
TOKENS = (Path.home() / ".cav" / "token", Path.home() / ".cavalry-mcp-token")


def environment():
    """What owns the bridge ports, and a hash and mtime of each bridge token.

    An agent once found the cavalry-mcp bridge down, started a forwarder from 8722 to
    cav-bridge on 8723 and copied the cav token over the cavalry-mcp token. Every later MCP
    run then went through cav-bridge. Each run compares this before and after.
    """
    owners, listener_pids = {}, {}
    for port in BRIDGE_PORTS:
        r = subprocess.run(["lsof", "-nP", f"-iTCP:{port}", "-sTCP:LISTEN", "-Fpc"], capture_output=True, text=True)
        owners[str(port)] = sorted({l[1:] for l in r.stdout.splitlines() if l.startswith("c")})
        listener_pids[str(port)] = sorted({int(l[1:]) for l in r.stdout.splitlines() if l.startswith("p") and l[1:].isdigit()})
    tokens = {}
    for t in TOKENS:
        if t.exists():
            tokens[t.name] = {"sha": hashlib.sha256(t.read_bytes().strip()).hexdigest()[:12], "mtime": t.stat().st_mtime}
    return {"listeners": owners, "listenerPids": listener_pids, "tokens": tokens}


def check_environment(env, arm):
    """Problems that make a run invalid before it starts."""
    problems = []
    for port, owners in env["listeners"].items():
        if any(o != "Cavalry" for o in owners):
            problems.append(f"port {port} is held by {owners}, not by Cavalry")
    if arm == "mcp" and not env["listeners"].get("8722"):
        problems.append("the cavalry-mcp bridge is not running (nothing listens on 8722)")
    pids = env.get('listenerPids', {})
    if pids.get('8722') and pids.get('8723') and pids['8722'] != pids['8723']:
        problems.append('the two bridges are held by different Cavalry processes')
    shas = [t["sha"] for t in env["tokens"].values()]
    if len(shas) == 2 and shas[0] == shas[1]:
        problems.append("~/.cav/token and ~/.cavalry-mcp-token are the same, so each bridge accepts the other's jobs")
    return problems


def prepare_cavalry():
    """Check the bridge and start from an empty scene."""
    # Cavalry can answer late, and a timed-out run can leave its last script running inside
    # Cavalry (status then exits 3, "still running"). Wait up to 10 minutes for it to finish.
    deadline = time.time() + 600
    while True:
        r = cav("status")
        if r.returncode == 0:
            break
        if time.time() > deadline:
            sys.exit("bridge not ready:\n" + r.stdout + r.stderr)
        time.sleep(15)
    r = cav("scene", "new", "--force", "--width", "1920", "--height", "1080", "--fps", "60", "--seconds", "10")
    if r.returncode != 0:
        sys.exit("cannot reset the scene:\n" + r.stderr)


def build_prompt(task, contract, arm):
    tool = {
        "plugin": "You control Cavalry from the shell with the `cav` command-line tool.",
        "baseline": "You control Cavalry from the shell with the `cav` command-line tool (run `cav help` to see how).",
        "mcp": "You control Cavalry with the cavalry MCP tools (cavalry_* tools).",
    }[arm]
    size = task["expect"]
    contract = contract.replace("{width}", str(size.get("width", 1920))).replace("{height}", str(size.get("height", 1080)))
    return f"{task['prompt']}\n\n{tool}\n\n{contract}"


def agent_command(model, thinking, via):
    command = ["pi", "--model", model, "--thinking", thinking, "--no-session", "--mode", "json",
               "--no-context-files", "--no-prompt-templates", "--no-themes", "--no-skills"]
    # Pioneer snapshots enabled provider/auth extensions and removes their tools. Some
    # configured models exist only through those extensions. Direct pi keeps its previous
    # built-in-only setup; Pioneer still exposes MCP tools only through --pi-extension.
    if via == "pi":
        command.append("--no-extensions")
    return command


def runtime_bin(iteration):
    directory = DATA / "snapshots" / f"iter{iteration}" / "runtime" / "bin"
    return directory if (directory / "build-brief").is_file() else None


def provider_error_count(events_path):
    """Pi can report a provider failure while Pioneer itself exits successfully."""
    count = 0
    with Path(events_path).open() as stream:
        for line in stream:
            try:
                event = json.loads(line)
            except ValueError:
                continue
            if event.get("type") == "turn_end" and (event.get("message") or {}).get("stopReason") == "error":
                count += 1
    return count


def write_mcp_config(path, pioneer, cache=None):
    """The cavalry-mcp server config for the MCP arm.

    Directly, uv starts the server. Under Pioneer the agent has a private HOME, and
    cavalry-mcp reads its token from ~/.cavalry-mcp-token, so the server starts through
    /usr/bin/env with the real HOME and from its own virtualenv (uv's cache is read-only).
    """
    if pioneer:
        if cache is None or not cache.is_dir():
            sys.exit('frozen MCP embedding cache is missing; prepare the iteration through batch.py')
        previews = Path(path).parent / 'out' / '.mcp-tmp'
        previews.mkdir(parents=True, exist_ok=True)
        server = {"command": "/usr/bin/env",
                  "args": ["HOME=" + str(Path.home()), "TMPDIR=" + str(previews),
                           "FASTEMBED_CACHE_PATH=" + str(cache), "HF_HUB_OFFLINE=1",
                           str(CAVALRY_MCP_DIR / ".venv" / "bin" / "cavalry-mcp")]}
    else:
        server = {"command": "uv", "args": ["run", "--directory", str(CAVALRY_MCP_DIR), "--extra", "kb", "cavalry-mcp"]}
    server.update({"directTools": True, "lifecycle": "eager"})
    Path(path).write_text(json.dumps({"mcpServers": {"cavalry": server}}, indent=2))


def session_to_events(session_dir, events_path):
    """Append pi session messages as turn_end / tool_execution_end events (the JSON-mode shape)."""
    files = sorted(Path(session_dir).rglob("*.jsonl"))
    with open(events_path, "a") as out:
        for f in files:
            for line in open(f):
                try:
                    e = json.loads(line)
                except ValueError:
                    continue
                m = e.get("message") or {}
                if m.get("role") == "assistant":
                    out.write(json.dumps({"type": "turn_end", "message": m}) + "\n")
                elif m.get("role") == "toolResult":
                    out.write(json.dumps({"type": "tool_execution_end", "toolName": m.get("toolName"),
                                          "isError": m.get("isError"), "result": {"content": m.get("content")}}) + "\n")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--iter", required=True)
    ap.add_argument("--task", required=True)
    ap.add_argument("--arm", required=True, choices=["plugin", "baseline", "mcp"])
    ap.add_argument("--model", required=True)
    ap.add_argument("--via", default="pi", choices=["pi", "pioneer"])
    ap.add_argument("--thinking", default="medium")
    ap.add_argument("--timeout", type=int, default=1800)
    ap.add_argument("--force", action="store_true", help="replace an existing run")
    args = ap.parse_args()

    # Each iteration runs against a frozen snapshot of the CLI and the skill, so edits made
    # while an iteration runs do not leak into it.
    global CAV, SKILL
    snap = DATA / "snapshots" / f"iter{args.iter}"
    if (snap / "bin" / "cav").exists():
        CAV, SKILL = snap / "bin" / "cav", snap / "skill"  # bin/ holds only cav, so PATH shows nothing else
    spec = json.loads(TASKS.read_text())
    task = next(t for t in spec["tasks"] if t["id"] == args.task)
    run = DATA / "runs" / f"iter{args.iter}" / args.task / f"{args.arm}__{slug(args.model)}"
    if run.exists():
        if not args.force:
            sys.exit(f"{run} exists (use --force)")
        shutil.rmtree(run)
    # Pioneer restricts the actor to work/ and its explicit runtime-read grants. Keep
    # scratch scenes and renders with the other eval data, then move them into the run.
    scratch = DATA / "scratch" / f"iter{args.iter}_{args.task}_{args.arm}_{slug(args.model)}"
    if scratch.exists():
        shutil.rmtree(scratch)
    work = scratch / "work"
    (work / "out").mkdir(parents=True)
    run.mkdir(parents=True)
    if task["fixtures"]:
        (work / "fixtures").mkdir()
        for f in task["fixtures"]:
            if not (FIXTURES / f).exists():
                sys.exit(f"fixture {f} is missing: run `uv run --no-project --with numpy --with scipy --with pillow python tools/evals/make_fixtures.py`")
            shutil.copy(FIXTURES / f, work / "fixtures" / f)

    prompt = build_prompt(task, spec["contract"], args.arm)
    (run / "prompt.txt").write_text(prompt)

    env = dict(os.environ)
    env["CAV_LOG"] = str(run / "cav.jsonl")
    env.pop("CAV_SPOOL", None)
    path = env.get("PATH", "")
    if args.arm == "mcp":
        # Hide cav from the MCP arm.
        path = os.pathsep.join(p for p in path.split(os.pathsep) if Path(p).resolve() != CAV.parent.resolve())
    else:
        path = str(CAV.parent) + os.pathsep + path
    runtime = runtime_bin(args.iter)
    if runtime is not None:
        path = str(runtime) + os.pathsep + path
    env["PATH"] = path

    pi = agent_command(args.model, args.thinking, args.via)
    if args.arm == "plugin":
        shutil.copytree(SKILL, scratch / "skills" / "cavalry")
        pi += ["--skill", str(scratch / "skills" / "cavalry")]
    if args.arm == "mcp":
        if not ADAPTER.exists():
            sys.exit("pi-mcp-adapter is not installed: run `npm ci` in " + str(MCP_TOOLS))
        write_mcp_config(work / "mcp.json", pioneer=args.via == "pioneer", cache=snap / 'runtime' / 'mcp-kb')
        # An explicit --mcp-config is not subject to the adapter's project-server trust gate.
        pi += ["-e", str(ADAPTER), "--mcp-config", str(work / "mcp.json")]
    pi += ["-p", prompt]

    before = environment()
    problems = check_environment(before, args.arm)
    if problems:
        sys.exit("environment not clean:\n  " + "\n  ".join(problems))
    prepare_cavalry()
    meta = {"task": args.task, "arm": args.arm, "model": args.model, "via": args.via, "iter": args.iter,
            "thinking": args.thinking, "started": time.strftime("%Y-%m-%dT%H:%M:%S")}
    t0 = time.time()
    with open(run / "events.jsonl", "w") as out, open(run / "stderr.txt", "w") as err:
        relay = None
        if args.via == "pi":
            p = subprocess.Popen(pi, cwd=work, env=env, stdout=out, stderr=err, stdin=subprocess.DEVNULL, start_new_session=True)
        else:
            # Inside Pioneer's sandbox the actor can write only its run folder, so cav runs in spool
            # mode: it drops each command line into a spool folder and `cav relay`, running outside
            # the sandbox, executes it (in the actor's folder) and writes the result back.
            spool = work / ".cav" / "spool"
            spool.mkdir(parents=True)
            (work / ".cav-spool").write_text(str(spool) + "\n")
            relay_env = dict(env)
            relay_env.pop("CAV_SPOOL", None)
            relay = subprocess.Popen([str(CAV), "relay", "--spool", str(spool)], env=relay_env,
                                     stdout=open(run / "relay.log", "w"), stderr=subprocess.STDOUT, start_new_session=True)
            actor_env = {k: v for k, v in env.items() if k != "CAV_LOG"}
            cmd = ["pioneer", "eval", "run", "--run-dir", str(work), "--timeout-ms", str(args.timeout * 1000),
                   "--work-log", str(run / "pioneer.jsonl"), "--runtime-read", str(CAV.parent)]
            if runtime is not None:
                cmd += ["--runtime-read", str(runtime)]
            if args.arm == "plugin":
                cmd += ["--runtime-read", str(scratch / "skills")]
            home = Path.home()
            if args.arm == "mcp":
                # Pioneer gives the actor a private HOME; cavalry-mcp reads its token from
                # ~/.cavalry-mcp-token, so write_mcp_config starts the server with the real HOME.
                cmd += ["--allow-loopback", "127.0.0.1:8722", "--pi-extension", str(ADAPTER),
                        "--runtime-read", str(CAVALRY_MCP_DIR), "--runtime-read", str(home / ".local" / "share" / "uv"),
                        "--runtime-read", str(home / ".cavalry-mcp-token"), "--runtime-read", str(snap / 'runtime' / 'mcp-kb')]
                j = pi.index("-e")
                pi = pi[:j] + ["--mcp-config", "mcp.json"] + pi[j + 4:]
            # Pioneer streams the actor's stdout and stderr to files (0.4.2+), so pi's JSON event
            # stream with embedded images does not hit the in-memory output cap.
            actor_out, actor_err = run / "actor-events.jsonl", run / "actor-stderr.txt"
            for f in (actor_out, actor_err):
                if f.exists():
                    f.unlink()
            cmd += ["--stdout-file", str(actor_out), "--stderr-file", str(actor_err)]
            actor_pi = list(pi)
            cmd += ["--"] + actor_pi
            p = subprocess.Popen(cmd, cwd=work, env=actor_env, stdout=out, stderr=err, stdin=subprocess.DEVNULL, start_new_session=True)
        try:
            code = p.wait(timeout=args.timeout)
            meta["timedOut"] = False
        except subprocess.TimeoutExpired:
            for sig in (signal.SIGTERM, signal.SIGKILL):
                try:
                    os.killpg(p.pid, sig)
                except (ProcessLookupError, PermissionError):
                    pass
                time.sleep(3)
            try:
                code = p.wait(timeout=30)
            except subprocess.TimeoutExpired:
                code = -9
            meta["timedOut"] = True
        if relay is not None:
            relay.terminate()
            try:
                relay.wait(timeout=10)
            except subprocess.TimeoutExpired:
                relay.kill()
    if args.via == "pioneer":
        # Pioneer's own messages went to events.jsonl / stderr.txt; the actor's streams are in files.
        with open(run / "events.jsonl", "a") as out:
            if (run / "actor-events.jsonl").exists():
                out.write((run / "actor-events.jsonl").read_text())
        with open(run / "stderr.txt", "a") as err:
            if (run / "actor-stderr.txt").exists():
                err.write((run / "actor-stderr.txt").read_text())
    meta["exit"] = code
    after = environment()
    if after != before:
        # The agent changed the bridges or their tokens: the run did not test what it claims to.
        meta["environmentChanged"] = {"before": before, "after": after}
    meta["workdir"] = str(work)
    shutil.move(str(work), str(run / "work"))
    shutil.rmtree(scratch, ignore_errors=True)
    meta["wallSeconds"] = round(time.time() - t0, 1)

    # Token use from pi's turn_end events.
    tokens = {"input": 0, "output": 0, "cacheRead": 0, "total": 0}
    turns = tools = tool_errors = 0
    final_text = ""
    for line in open(run / "events.jsonl"):
        try:
            e = json.loads(line)
        except ValueError:
            continue
        if e.get("type") == "turn_end":
            turns += 1
            u = (e.get("message") or {}).get("usage") or {}
            for k in ("input", "output", "cacheRead"):
                tokens[k] += u.get(k, 0) or 0
            tokens["total"] += u.get("totalTokens", 0) or 0
            for part in (e.get("message") or {}).get("content") or []:
                if part.get("type") == "text":
                    final_text = part.get("text", "")
        if e.get("type") == "tool_execution_end":
            tools += 1
            if e.get("isError"):
                tool_errors += 1
    meta.update({"tokens": tokens, "turns": turns, "toolCalls": tools, "toolErrors": tool_errors, "finalText": final_text[-2000:]})
    meta["providerErrors"] = provider_error_count(run / "events.jsonl")
    (run / "meta.json").write_text(json.dumps(meta, indent=2))
    print(json.dumps({k: meta[k] for k in ("task", "arm", "model", "exit", "wallSeconds", "turns", "toolCalls", "toolErrors")}))
    if meta["providerErrors"]:
        sys.exit(f"provider failed in {meta['providerErrors']} completed turn(s); inspect {run / 'events.jsonl'}")


if __name__ == "__main__":
    main()
