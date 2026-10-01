"""Judge prepared A/B sheets through Pioneer, without exposing the answer key.

  python judge.py --iter name --judge-model opencode/claude-sonnet-4-6 [--tag mcp] [--skip-existing]

Run blind.py prepare first. Each task folder is the actor's only writable/readable input
folder. Controller logs and model provenance stay outside it. The judge reads only A.png,
B.png and task.md and writes verdict.json. This command never contacts Cavalry.
"""
import argparse
import json
import subprocess
import sys
import time
from pathlib import Path

from blind import judge_name, validate_verdict
from evalpaths import DATA
from run import agent_command

PROMPT = """Compare two motion-design contact sheets. Read only task.md, A.png and B.png in
this folder. View both images. Do not read any other source, scene, video, log or answer key.
Judge only the visible evidence in these two sheets against the five rubric claims in task.md.
For each claim use true if met, false if visibly not met, and null if the sheets cannot establish
it. Audio and exact beat timing cannot be established from still sheets. Do not infer them.
Pick the better sheet overall for fulfilling the brief with clear design, readable content,
coherent motion visible across frames and a polished composition, or choose tie.
Write verdict.json with exactly this shape:
{"claims":{"A":[true,false,null,true,false],"B":[true,false,null,true,false]},"better":"A","reason":"your short evidence-based reason"}
Use five entries per sheet, in rubric order. better must be A, B or tie. Do not identify or guess
the tools or models that made the sheets. Finish with a short confirmation.
"""


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--iter', required=True)
    ap.add_argument('--tag', default='')
    ap.add_argument('--judge-model', required=True)
    ap.add_argument('--tested-model', default='zai/glm-5.3-flash')
    ap.add_argument('--timeout', type=int, default=600)
    ap.add_argument('--skip-existing', action='store_true')
    args = ap.parse_args()
    for name in (args.iter, args.tag):
        if name and (Path(name).name != name or name in ('.', '..')):
            ap.error('iteration and tag must be folder names')
    if args.judge_model.rsplit('/', 1)[-1] == args.tested_model.rsplit('/', 1)[-1]:
        ap.error('judge must use a different model from the tested model')
    if args.timeout < 1:
        ap.error('timeout must be positive')
    name = judge_name(args)
    count = 0
    for suffix in ('', '-swap'):
        base = DATA / 'judge' / f'{name}{suffix}'
        if not base.is_dir():
            ap.error(f'prepared folder missing: {base}')
        # Do not read key.json. Folder names suffice to find prepared cases.
        cases = sorted(p for p in base.iterdir() if p.is_dir())
        for case in cases:
            if not all((case / p).is_file() for p in ('A.png', 'B.png', 'task.md')):
                sys.exit(f'judging input incomplete: {case}')
            logdir = DATA / 'judge-logs' / f'{name}{suffix}' / case.name
            verdict = case / 'verdict.json'
            provenance = logdir / 'judge.json'
            if verdict.exists():
                validate_verdict(json.loads(verdict.read_text()))
                previous = json.loads(provenance.read_text()) if provenance.exists() else {}
                if args.skip_existing and previous.get('judgeModel') == args.judge_model and previous.get('exit') == 0:
                    count += 1
                    continue
                sys.exit(f'verdict already exists in {case}; preserve it or use --skip-existing with the same judge')
            logdir.mkdir(parents=True, exist_ok=True)
            # Pioneer output paths are create-only. Refuse to overwrite a failed attempt.
            outputs = {k: logdir / v for k, v in {'work': 'pioneer.jsonl', 'stdout': 'actor-events.jsonl', 'stderr': 'actor-stderr.txt'}.items()}
            if any(p.exists() for p in outputs.values()):
                sys.exit(f'judge logs already exist: {logdir}; preserve the failed attempt before retrying')
            command = ['pioneer', 'eval', 'run', '--run-dir', str(case), '--timeout-ms', str(args.timeout * 1000),
                       '--work-log', str(outputs['work']), '--stdout-file', str(outputs['stdout']), '--stderr-file', str(outputs['stderr']),
                       '--', *agent_command(args.judge_model, 'medium', 'pioneer'), '-p', PROMPT]
            print(f'Judging {case.name}{suffix} with {args.judge_model}', flush=True)
            started = time.time()
            with (logdir / 'controller-stdout.txt').open('w') as out, (logdir / 'controller-stderr.txt').open('w') as err:
                result = subprocess.run(command, cwd=case, stdout=out, stderr=err)
            info = {'judgeModel': args.judge_model, 'testedModel': args.tested_model, 'via': 'pioneer',
                    'case': str(case), 'exit': result.returncode, 'wallSeconds': round(time.time() - started, 1)}
            provenance.write_text(json.dumps(info, indent=2) + '\n')
            if result.returncode:
                sys.exit(f'judge controller failed with exit {result.returncode}; inspect {logdir}')
            if not verdict.is_file():
                sys.exit(f'judge did not write verdict.json; inspect {logdir}')
            validate_verdict(json.loads(verdict.read_text()))
            count += 1
            print(f'Validated verdict {count}', flush=True)
    print(f'{count} verdicts validated; judge {args.judge_model}')


if __name__ == '__main__':
    main()
