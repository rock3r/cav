"""Where the eval harness finds its code, its inputs and its data.

Code and tasks live next to this file (tools/evals). Generated fixtures and run data (runs,
frozen snapshots, blind-judging folders, logs) go to CAV_EVAL_DATA, by default .plans/evals,
which is not committed. make_fixtures.py rebuilds the fixtures byte for byte.
"""
import os
from pathlib import Path

CODE = Path(__file__).resolve().parent
REPO = CODE.parent.parent
DATA = Path(os.environ.get("CAV_EVAL_DATA", REPO / ".plans" / "evals")).resolve()
TASKS = CODE / "tasks.json"
FIXTURES = DATA / "fixtures"
MCP_TOOLS = CODE / "mcp"
CAV = REPO / "bin" / "cav"
CAVALRY_MCP_DIR = Path(os.environ.get("CAVALRY_MCP_DIR", Path.home() / "src" / "cavalry-mcp"))
