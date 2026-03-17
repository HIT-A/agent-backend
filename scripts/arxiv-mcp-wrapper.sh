#!/usr/bin/env bash
set -euo pipefail

pkg_dir="$(ls -dt /root/.npm/_npx/*/node_modules/arxiv-mcp-server 2>/dev/null | head -n1 || true)"
if [[ -z "${pkg_dir}" ]]; then
  echo "arxiv-mcp-server package not found in npx cache" >&2
  exit 1
fi

venv_python="${pkg_dir}/.venv/bin/python"
if [[ -x "${venv_python}" ]]; then
  exec env PYTHONPATH="${pkg_dir}/src" "${venv_python}" -m arxiv_mcp_server.server
fi

exec env PYTHONPATH="${pkg_dir}/src" python3 -m arxiv_mcp_server.server
