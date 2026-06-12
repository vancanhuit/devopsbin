#!/usr/bin/env python3
# [MISE] description="Smoke test the Compose cluster profile (build, up, probe endpoints, down)."
# [MISE] depends=["compose:cluster:up"]
# [MISE] depends_post=["compose:cluster:down"]
# [MISE] tools={python="3.14.5"}
"""Smoke test for the Compose `cluster` profile (Redis Cluster).

Thin wrapper that owns the cluster stack lifecycle via its `depends`/
`depends_post` hooks and delegates the actual probing to the shared
`mise/lib/smoke.py`.

Examples:
    # Probe the default stack
    mise run smoke:cluster

    # Point at a non-default host/port
    mise run smoke:cluster -- --base-url http://127.0.0.1:8080
"""

from __future__ import annotations

import sys
from pathlib import Path

# The shared probe library lives in mise/lib, outside the task tree so mise
# does not treat it as a task.
sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "lib"))

from smoke import main  # noqa: E402 (import after sys.path setup)

if __name__ == "__main__":
    raise SystemExit(main())
