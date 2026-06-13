#!/usr/bin/env python3
# [MISE] description="Smoke test the Compose tls profile (direct HTTPS + Caddy-proxied) with mkcert certs."
# [MISE] depends=["compose:tls:test:up"]
# [MISE] depends_post=["compose:tls:test:down"]
# [MISE] tools={python="3.14.5"}
"""Smoke test for the Compose `tls` profile.

Owns the tls stack lifecycle via its `depends`/`depends_post` hooks (which also
generate ephemeral certs through compose:tls:test:up -> certs:test) and
delegates probing to the shared `mise/lib/smoke.py`. It verifies two TLS
topologies: the binary serving HTTPS directly, and the same binary behind a
Caddy reverse proxy that terminates TLS and forwards client headers.

Server certificates are verified against the mkcert CA at tls/test/rootCA.pem
(full chain + hostname checks), mirroring production behaviour with a private
CA -- there is no insecure fallback.

Examples:
    # Probe the default tls stack
    mise run smoke:tls

    # Override the CA bundle or URLs
    mise run smoke:tls -- --ca-cert /path/to/rootCA.pem
"""

from __future__ import annotations

import os
import sys
from pathlib import Path

# The shared probe library lives in mise/lib, outside the task tree so mise
# does not treat it as a task.
sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "lib"))

from smoke import main_tls  # noqa: E402 (import after sys.path setup)

if __name__ == "__main__":
    argv = sys.argv[1:]
    # Default --ca-cert to the repo's mkcert CA unless the caller overrode it.
    if "--ca-cert" not in argv:
        project_root = os.environ.get("MISE_PROJECT_ROOT", ".")
        ca_cert = str(Path(project_root) / "tls" / "test" / "rootCA.pem")
        argv = ["--ca-cert", ca_cert, *argv]
    raise SystemExit(main_tls(argv))
