"""Harness checks that do not need Cavalry or a model provider."""
import argparse
import contextlib
import io
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import batch
import blind


class BlindFolders(unittest.TestCase):
    def test_tag_keeps_other_comparison_and_swaps_order(self):
        with tempfile.TemporaryDirectory() as td:
            data = Path(td)
            tasks = data / 'tasks.json'
            tasks.write_text(json.dumps({'tasks': [{'id': 'sample', 'prompt': 'brief', 'rubric': ['claim']}]}))
            for arm in ('plugin', 'baseline', 'mcp'):
                run = data / 'runs' / 'iterfresh' / 'sample' / f'{arm}__model'
                run.mkdir(parents=True)
                (run / 'review.png').write_bytes(arm.encode())
            args = argparse.Namespace(iter='fresh', model='model', a='plugin', b='baseline', seed=7, tag='')
            with patch.object(blind, 'EVALS', data), patch.object(blind, 'TASKS', tasks), contextlib.redirect_stdout(io.StringIO()):
                blind.prepare(args)
                original = (data / 'judge' / 'fresh' / 'key.json').read_bytes()
                args.b, args.tag = 'mcp', 'mcp'
                blind.prepare(args)
                self.assertEqual((data / 'judge' / 'fresh' / 'key.json').read_bytes(), original)
                first = data / 'judge' / 'fresh-mcp' / 'sample'
                second = data / 'judge' / 'fresh-mcp-swap' / 'sample'
                self.assertEqual((first / 'A.png').read_bytes(), (second / 'B.png').read_bytes())
                self.assertEqual((first / 'B.png').read_bytes(), (second / 'A.png').read_bytes())

    def test_empty_iteration_still_has_keys(self):
        with tempfile.TemporaryDirectory() as td:
            data = Path(td)
            tasks = data / 'tasks.json'
            tasks.write_text(json.dumps({'tasks': []}))
            args = argparse.Namespace(iter='empty', model='model', a='plugin', b='baseline', seed=7, tag='mcp')
            with patch.object(blind, 'EVALS', data), patch.object(blind, 'TASKS', tasks), contextlib.redirect_stdout(io.StringIO()):
                blind.prepare(args)
                self.assertEqual(json.loads((data / 'judge' / 'empty-mcp' / 'key.json').read_text()), {})


class BatchStops(unittest.TestCase):
    def check_stop(self, meta, skip=False):
        with tempfile.TemporaryDirectory() as td:
            data = Path(td)
            tasks = data / 'tasks.json'
            tasks.write_text(json.dumps({'tasks': [{'id': 'one'}, {'id': 'two'}]}))
            run = data / 'runs' / 'iterfresh' / 'one' / 'plugin__model'
            run.mkdir(parents=True)
            (run / 'meta.json').write_text(json.dumps(meta))
            if skip:
                (run / 'score.json').write_text('{}')
            argv = ['batch.py', '--iter', 'fresh', '--arms', 'plugin', '--models', 'model', '--via', 'pioneer']
            if skip:
                argv.append('--skip-existing')
            with patch.object(batch, 'DATA', data), patch.object(batch, 'TASKS', tasks), patch.object(batch, 'freeze'), patch('sys.argv', argv), patch.object(batch.subprocess, 'run', return_value=argparse.Namespace(returncode=0)) as runner, contextlib.redirect_stdout(io.StringIO()):
                with self.assertRaises(SystemExit):
                    batch.main()
                self.assertEqual(runner.call_count, 0 if skip else 1)

    def test_environment_change_stops_before_scoring_or_next_run(self):
        self.check_stop({'exit': 0, 'timedOut': False, 'environmentChanged': {}})

    def test_resume_rejects_changed_environment(self):
        self.check_stop({'exit': 0, 'timedOut': False, 'environmentChanged': {}}, skip=True)

    def test_controller_failure_stops_before_scoring_or_next_run(self):
        self.check_stop({'exit': 1, 'timedOut': False})


if __name__ == '__main__':
    unittest.main()
