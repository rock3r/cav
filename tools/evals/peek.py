"""Print the tool calls of a run so far: python peek.py <run dir> [max chars]"""
import json, sys
run, n = sys.argv[1], int(sys.argv[2]) if len(sys.argv) > 2 else 220
for l in open(run + '/events.jsonl'):
    try:
        e = json.loads(l)
    except ValueError:
        continue
    if e.get('type') == 'tool_execution_start':
        print('>>', e.get('toolName'), json.dumps(e.get('args') or {})[:n])
    elif e.get('type') == 'tool_execution_end':
        c = (e.get('result') or {}).get('content') or [{}]
        print('   <-', ('ERR ' if e.get('isError') else '') + json.dumps(c[0].get('text', ''))[:n])
    elif e.get('type') == 'turn_end':
        for p in (e.get('message') or {}).get('content') or []:
            if p.get('type') == 'text' and p.get('text', '').strip():
                print('  says:', p['text'].strip()[:n])
