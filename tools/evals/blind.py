#!/usr/bin/env python3
"""Blind pairwise judging of two arms.

prepare: for every task that has both arms in the iteration, copy the two review sheets to
         judge/<iter>/<task>/A.png and B.png in a random order, write the task text and
         rubric to task.md, and keep the answer key in judge/<iter>/key.json (never shown to
         the judge). A second pass with the order swapped goes to judge/<iter>-swap/.
unblind: read verdict.json files written by the judge and report wins per arm, the rubric
         claims each arm met, and how often the two passes agreed.
"""
import argparse
import json
import random
import shutil
from pathlib import Path

from evalpaths import DATA as EVALS, TASKS


def slug(s):
    return s.replace("/", "_").replace(":", "_")


def judge_name(args):
    return args.iter + ("-" + args.tag if args.tag else "")


def validate_verdict(value):
    if not isinstance(value, dict) or set(value) != {'claims', 'better', 'reason'}:
        raise ValueError('verdict must contain exactly claims, better and reason')
    if value['better'] not in ('A', 'B', 'tie'):
        raise ValueError('better must be A, B or tie')
    if not isinstance(value['reason'], str) or not value['reason'].strip():
        raise ValueError('reason must be nonempty text')
    claims = value['claims']
    if not isinstance(claims, dict) or set(claims) != {'A', 'B'}:
        raise ValueError('claims must contain A and B')
    for label in 'AB':
        entries = claims[label]
        if not isinstance(entries, list) or len(entries) != 5:
            raise ValueError(f'{label} needs five claims')
        if any(type(c) is not bool and c is not None for c in entries):
            raise ValueError(f'{label} claims must be true, false or null')
    return value



def prepare(args):
    spec = json.loads(TASKS.read_text())
    rng = random.Random(args.seed)
    keep = getattr(args, 'keep_existing', False)
    previous = {}
    for suffix in ("", "-swap"):
        out = EVALS / "judge" / f"{judge_name(args)}{suffix}"
        previous[suffix] = json.loads((out / 'key.json').read_text()) if keep and (out / 'key.json').is_file() else {}
        if out.exists() and not keep:
            shutil.rmtree(out)
    key, key_swap = {}, {}
    prepared = []
    for task in spec["tasks"]:
        runs = {arm: EVALS / "runs" / f"iter{args.iter}" / task["id"] / f"{arm}__{slug(args.model)}" for arm in (args.a, args.b)}
        if not all((r / "review.png").exists() for r in runs.values()):
            continue
        first = [args.a, args.b]
        rng.shuffle(first)
        if keep and task['id'] in previous['']:
            retained = previous[''][task['id']]
            if (not isinstance(retained, dict) or set(retained) != {'A', 'B'} or
                    not all(isinstance(value, str) for value in retained.values()) or
                    set(retained.values()) != {args.a, args.b}):
                raise ValueError('invalid retained blind mapping; preserve the prior comparison')
            first = [retained['A'], retained['B']]
            if previous['-swap'].get(task['id']) != {'A': first[1], 'B': first[0]}:
                raise ValueError('retained passes are not swapped; preserve the prior comparison')
        for suffix, order, k in (("", first, key), ("-swap", first[::-1], key_swap)):
            d = EVALS / "judge" / f"{judge_name(args)}{suffix}" / task["id"]
            claims = "\n".join(f"{i + 1}. {c}" for i, c in enumerate(task["rubric"]))
            inputs = {label + '.png': (runs[arm] / 'review.png').read_bytes() for label, arm in zip('AB', order)}
            inputs['task.md'] = f"# Task\n\n{task['prompt']}\n\n# Rubric claims\n\n{claims}\n".encode()
            mapping = {"A": order[0], "B": order[1]}
            if keep and d.exists():
                if previous[suffix].get(task['id']) != mapping:
                    raise ValueError(f'existing blind mapping differs: {d}; preserve the prior comparison')
                for name, content in inputs.items():
                    path = d / name
                    if not path.is_file() or path.read_bytes() != content:
                        raise ValueError(f'existing blind input differs: {path}; preserve the prior comparison')
            else:
                prepared.append((d, inputs))
            k[task["id"]] = mapping
    for suffix, k in (("", key), ("-swap", key_swap)):
        if keep and set(previous[suffix]) - set(k):
            raise ValueError('a previously prepared pair is missing; preserve the prior comparison')
    # Validate all retained inputs before writing any new cases or answer keys.
    for directory, inputs in prepared:
        directory.mkdir(parents=True)
        for name, content in inputs.items():
            (directory / name).write_bytes(content)
    for suffix, k in (("", key), ("-swap", key_swap)):
        base = EVALS / "judge" / f"{judge_name(args)}{suffix}"
        base.mkdir(parents=True, exist_ok=True)
        (base / "key.json").write_text(json.dumps(k, indent=1))
    print(f"{len(key)} pairs in judge/{judge_name(args)} and judge/{judge_name(args)}-swap")


def unblind(args):
    tally = {args.a: 0, args.b: 0, "tie": 0}
    claims = {args.a: 0, args.b: 0}
    unknown = {args.a: 0, args.b: 0}
    total_claims = 0
    agree = pairs = 0
    rows = []
    verdicts = {}
    for suffix in ("", "-swap"):
        base = EVALS / "judge" / f"{judge_name(args)}{suffix}"
        key = json.loads((base / "key.json").read_text())
        for task, mapping in key.items():
            v = validate_verdict(json.loads((base / task / "verdict.json").read_text()))
            winner = mapping.get(v["better"], "tie")
            verdicts.setdefault(task, []).append(winner)
            tally[winner] += 1
            for label in "AB":
                met = sum(1 for c in v["claims"][label] if c is True)
                unknown[mapping[label]] += sum(1 for c in v["claims"][label] if c is None)
                claims[mapping[label]] += met
            total_claims += len(v["claims"]["A"])
    for task, ws in verdicts.items():
        pairs += 1
        agree += 1 if len(set(ws)) == 1 else 0
        rows.append(f"| {task} | {' / '.join(ws)} |")
    print(f"# Blind comparison, iteration {args.iter}: {args.a} vs {args.b}\n")
    print("| Task | Better (pass 1 / swapped pass 2) |\n|---|---|")
    print("\n".join(rows))
    print(f"\nWins over both passes: {args.a} {tally[args.a]}, {args.b} {tally[args.b]}, tie {tally['tie']}.")
    print(f"Rubric claims met: {args.a} {claims[args.a]}/{total_claims}, {args.b} {claims[args.b]}/{total_claims}.")
    print(f"Unknown claims: {args.a} {unknown[args.a]}, {args.b} {unknown[args.b]} (included in the claim totals above).")
    print(f"The two passes agreed on {agree} of {pairs} tasks.")


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("cmd", choices=["prepare", "unblind"])
    ap.add_argument("--iter", required=True)
    ap.add_argument("--model", default="zai/glm-5.3-flash")
    ap.add_argument("--a", default="plugin")
    ap.add_argument("--b", default="baseline")
    ap.add_argument("--seed", type=int, default=7)
    ap.add_argument("--tag", default="", help="separate judging folder for another arm comparison")
    ap.add_argument("--keep-existing", action="store_true", help="add ready pairs without changing prior inputs, mappings or verdicts")
    args = ap.parse_args()
    if args.tag and (Path(args.tag).name != args.tag or args.tag in (".", "..")):
        ap.error("--tag must be a folder name, without path separators")
    prepare(args) if args.cmd == "prepare" else unblind(args)
