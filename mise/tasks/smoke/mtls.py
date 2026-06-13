#!/usr/bin/env python3
# [MISE] description="Smoke test the Compose mtls profile (mutual TLS backend + re-encrypting Caddy) with mkcert certs."
# [MISE] depends=["compose:mtls:test:up"]
# [MISE] depends_post=["compose:mtls:test:down"]
# [MISE] tools={python="3.14.5"}
"""Smoke test for the Compose `mtls` profile.

Owns the mtls stack lifecycle via its `depends`/`depends_post` hooks (which also
generate ephemeral certs through compose:mtls:test:up -> certs:test) and
delegates probing to the shared `mise/lib/smoke.py`. It verifies mutual TLS end
to end: the backend serves HTTPS and requires a client certificate, while Caddy
terminates the browser-facing TLS and re-encrypts to the backend with its own
client certificate.

Server certificates are verified against the mkcert CA at tls/test/rootCA.pem
(full chain + hostname checks), and the client certificate at
tls/test/devopsbin-client.pem is presented for the direct mTLS path -- mirroring
production behaviour with a private CA, with no insecure fallback.

Examples:
    # Probe the default mtls stack
    mise run smoke:mtls

    # Override the CA bundle, client cert, or URLs
    mise run smoke:mtls -- --ca-cert /path/to/rootCA.pem
"""

from __future__ import annotations

import os
import sys
from pathlib import Path

# The shared probe library lives in mise/lib, outside the task tree so mise
# does not treat it as a task.
sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "lib"))

from smoke import main_mtls  # noqa: E402 (import after sys.path setup)

if __name__ == "__main__":
    argv = sys.argv[1:]
    # Default the cert paths to the repo's mkcert test certs unless overridden.
    project_root = os.environ.get("MISE_PROJECT_ROOT", ".")
    certs_dir = Path(project_root) / "tls" / "test"
    if "--ca-cert" not in argv:
        argv = ["--ca-cert", str(certs_dir / "rootCA.pem"), *argv]
    if "--client-cert" not in argv:
        argv = ["--client-cert", str(certs_dir / "devopsbin-client.pem"), *argv]
    if "--client-key" not in argv:
        argv = ["--client-key", str(certs_dir / "devopsbin-client-key.pem"), *argv]
    raise SystemExit(main_mtls(argv))
