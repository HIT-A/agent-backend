#!/usr/bin/env python3
import os
import re
import subprocess
import sys
import threading
from pathlib import Path


def find_arxiv_pkg() -> Path:
    npx_root = Path('/root/.npm/_npx')
    if not npx_root.exists():
        raise RuntimeError('npx cache directory not found')
    candidates = sorted(
        npx_root.glob('*/node_modules/arxiv-mcp-server'),
        key=lambda p: p.stat().st_mtime,
        reverse=True,
    )
    if not candidates:
        # warm up cache once
        subprocess.run(
            ['npx', '-y', 'arxiv-mcp-server', '--help'],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=False,
            env=os.environ.copy(),
        )
        candidates = sorted(
            npx_root.glob('*/node_modules/arxiv-mcp-server'),
            key=lambda p: p.stat().st_mtime,
            reverse=True,
        )
    if not candidates:
        raise RuntimeError('arxiv-mcp-server package not found in npx cache')
    return candidates[0]


def read_framed_message(stream) -> str | None:
    headers = {}
    while True:
        line = stream.readline()
        if not line:
            return None
        if line in (b'\r\n', b'\n'):
            break
        try:
            text = line.decode('utf-8', errors='replace').strip()
        except Exception:
            continue
        if ':' in text:
            k, v = text.split(':', 1)
            headers[k.strip().lower()] = v.strip()
    clen = headers.get('content-length')
    if not clen:
        return None
    length = int(clen)
    body = stream.read(length)
    if not body or len(body) < length:
        return None
    return body.decode('utf-8', errors='replace')


def write_framed_message(stream, payload: str) -> None:
    data = payload.encode('utf-8')
    header = f'Content-Length: {len(data)}\r\n\r\n'.encode('ascii')
    stream.write(header)
    stream.write(data)
    stream.flush()


def forward_stdin_to_child(child_stdin):
    for line in sys.stdin:
        msg = line.strip()
        if not msg:
            continue
        write_framed_message(child_stdin, msg)
    try:
        child_stdin.close()
    except Exception:
        pass


def forward_child_to_stdout(child_stdout):
    while True:
        msg = read_framed_message(child_stdout)
        if msg is None:
            break
        sys.stdout.write(msg + '\n')
        sys.stdout.flush()


def forward_stderr(child_stderr):
    for chunk in iter(lambda: child_stderr.read(4096), b''):
        if not chunk:
            break
        sys.stderr.buffer.write(chunk)
        sys.stderr.flush()


def main() -> int:
    try:
        pkg = find_arxiv_pkg()
    except Exception as e:
        print(f'arxiv wrapper setup failed: {e}', file=sys.stderr)
        return 1

    venv_python = pkg / '.venv' / 'bin' / 'python'
    python_exec = str(venv_python if venv_python.exists() else Path('/usr/bin/python3'))

    env = os.environ.copy()
    env['PYTHONPATH'] = str(pkg / 'src')

    child = subprocess.Popen(
        [python_exec, '-m', 'arxiv_mcp_server.server'],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env=env,
        bufsize=0,
    )

    t_in = threading.Thread(target=forward_stdin_to_child, args=(child.stdin,), daemon=True)
    t_out = threading.Thread(target=forward_child_to_stdout, args=(child.stdout,), daemon=True)
    t_err = threading.Thread(target=forward_stderr, args=(child.stderr,), daemon=True)

    t_in.start()
    t_out.start()
    t_err.start()

    rc = child.wait()
    return rc


if __name__ == '__main__':
    sys.exit(main())
