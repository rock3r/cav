"""Judge prepared A/B sheets without exposing the answer key.

  python judge.py --iter name --via codex --judge-model gpt-6.1-sol [--tag mcp]

Run blind.py prepare first. Codex receives A.png, B.png and task.md directly, with tools
disabled; the harness rejects any tool use. Pioneer grants access only to the case folder
and explicit runtime files. Logs and model provenance stay outside the case folder.
This command never contacts Cavalry.
"""
import argparse
import hashlib
import json
import os
import subprocess
import sys
import time
from pathlib import Path

from blind import judge_name, validate_verdict
from evalpaths import DATA
from run import agent_command, provider_error_count, runtime_bin

VERDICT_SCHEMA = {
    'type': 'object', 'additionalProperties': False,
    'properties': {
        'claims': {'type': 'object', 'additionalProperties': False,
                   'properties': {side: {'type': 'array', 'items': {'type': ['boolean', 'null']},
                                         'minItems': 5, 'maxItems': 5} for side in ('A', 'B')},
                   'required': ['A', 'B']},
        'better': {'type': 'string', 'enum': ['A', 'B', 'tie']},
        'reason': {'type': 'string'},
    },
    'required': ['claims', 'better', 'reason'],
}


def codex_command(binary, model, case, logdir):
    # Attach all judging evidence directly. No tools, inherited skills, project notes,
    # memories, integrations or answer-key access are needed for this comparison.
    command = [binary, 'exec', '--ignore-user-config', '--ephemeral', '--sandbox', 'read-only',
               '--skip-git-repo-check', '--model', model, '--json', '--cd', str(case),
               '--enable', 'skip_host_skill_discovery', '-c', 'project_doc_max_bytes=0',
               '-c', 'web_search="disabled"', '-c', 'model_reasoning_effort="medium"']
    for feature in ('shell_tool', 'unified_exec', 'view_image', 'multi_agent', 'apps', 'plugins',
                    'memories', 'browser_use', 'computer_use', 'hooks', 'code_mode_host', 'skill_search'):
        command += ['--disable', feature]
    command += ['--image', str(case / 'A.png'), '--image', str(case / 'B.png'),
                '--output-schema', str(logdir / 'schema.json'),
                '--output-last-message', str(logdir / 'returned-verdict.json'), '-']
    return command


def codex_event_summary(path):
    errors = 0
    tools = set()
    completed = False
    for line in path.read_text().splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            errors += 1
            continue
        if event.get('type') in ('error', 'turn.failed'):
            errors += 1
        if event.get('type') == 'turn.completed':
            completed = True
        item = event.get('item') or {}
        if event.get('type') in ('item.started', 'item.completed') and item.get('type') not in ('agent_message', 'reasoning', 'error'):
            tools.add(item.get('type', 'unknown'))
    return {'providerErrors': errors, 'toolsUsed': sorted(tools), 'turnCompleted': completed}

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
    ap.add_argument('--via', choices=('codex', 'pioneer'), default='codex')
    ap.add_argument('--codex-bin', default='codex', help='Codex binary supporting the requested model')
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
    codex_version = None
    if args.via == 'codex':
        codex_version = subprocess.run([args.codex_bin, '--version'], capture_output=True, text=True, check=True).stdout.strip()
    runtime = runtime_bin(args.iter)
    environment = dict(os.environ)
    if runtime is not None:
        environment['PATH'] = str(runtime) + os.pathsep + environment.get('PATH', '')
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
                if args.skip_existing and previous.get('judgeModel') == args.judge_model and previous.get('via') == args.via and previous.get('exit') == 0 and previous.get('providerErrors') == 0:
                    count += 1
                    continue
                sys.exit(f'verdict already exists in {case}; preserve it or use --skip-existing with the same judge')
            logdir.mkdir(parents=True, exist_ok=True)
            # Pioneer output paths are create-only. Refuse to overwrite a failed attempt.
            names = {'work': 'pioneer.jsonl', 'stdout': 'actor-events.jsonl', 'stderr': 'actor-stderr.txt'} if args.via == 'pioneer' else {'stdout': 'codex-events.jsonl', 'stderr': 'codex-stderr.txt', 'verdict': 'returned-verdict.json'}
            outputs = {k: logdir / v for k, v in names.items()}
            if any(p.exists() for p in outputs.values()):
                sys.exit(f'judge logs already exist: {logdir}; preserve the failed attempt before retrying')
            if args.via == 'pioneer':
                command = ['pioneer', 'eval', 'run', '--run-dir', str(case), '--timeout-ms', str(args.timeout * 1000),
                           '--work-log', str(outputs['work']), '--stdout-file', str(outputs['stdout']), '--stderr-file', str(outputs['stderr'])]
                if runtime is not None:
                    command += ['--runtime-read', str(runtime)]
                command += ['--', *agent_command(args.judge_model, 'medium', 'pioneer'), '-p', PROMPT]
                prompt = None
                stdout_path, stderr_path = logdir / 'controller-stdout.txt', logdir / 'controller-stderr.txt'
            else:
                (logdir / 'schema.json').write_text(json.dumps(VERDICT_SCHEMA, indent=2) + '\n')
                command = codex_command(args.codex_bin, args.judge_model, case, logdir)
                prompt = PROMPT.replace('Read only task.md, A.png and B.png in\nthis folder. View both images.', 'Use the attached images and task.md text supplied below.')
                prompt = prompt.replace('Write verdict.json', 'Return the JSON verdict as your final response').replace('Finish with a short confirmation.', 'Return only the JSON verdict.')
                prompt += '\nThe first attached image is A; the second is B. Do not use tools or read files.\n\n' + (case / 'task.md').read_text()
                stdout_path, stderr_path = outputs['stdout'], outputs['stderr']
            print(f'Judging {case.name}{suffix} with {args.judge_model}', flush=True)
            started = time.time()
            with stdout_path.open('w') as out, stderr_path.open('w') as err:
                try:
                    result = subprocess.run(command, cwd=case, env=environment, stdout=out, stderr=err,
                                            input=prompt, text=True, timeout=args.timeout + 30)
                    exit_code = result.returncode
                except subprocess.TimeoutExpired:
                    exit_code = 124
            info = {'judgeModel': args.judge_model, 'testedModel': args.tested_model, 'via': args.via,
                    'case': str(case), 'exit': exit_code, 'wallSeconds': round(time.time() - started, 1),
                    'inputHashes': {p: hashlib.sha256((case / p).read_bytes()).hexdigest() for p in ('A.png', 'B.png', 'task.md')}}
            if args.via == 'codex':
                info.update(codex_event_summary(outputs['stdout']))
                info['judgeBinary'] = args.codex_bin
                info['codexVersion'] = codex_version
            else:
                info['providerErrors'] = provider_error_count(outputs['stdout']) if outputs['stdout'].is_file() else None
            provenance.write_text(json.dumps(info, indent=2) + '\n')
            if exit_code:
                sys.exit(f'judge controller failed with exit {exit_code}; inspect {logdir}')
            if info['providerErrors'] != 0:
                sys.exit(f'judge provider failed or its event stream is missing; inspect {logdir}')
            if args.via == 'codex':
                if info['toolsUsed'] or not info['turnCompleted']:
                    sys.exit(f'judge used tools or did not complete its turn; inspect {logdir}')
                value = validate_verdict(json.loads(outputs['verdict'].read_text()))
                verdict.write_text(json.dumps(value, indent=2) + '\n')
            if not verdict.is_file():
                sys.exit(f'judge did not write verdict.json; inspect {logdir}')
            validate_verdict(json.loads(verdict.read_text()))
            count += 1
            print(f'Validated verdict {count}', flush=True)
    print(f'{count} verdicts validated; judge {args.judge_model}')


if __name__ == '__main__':
    main()
