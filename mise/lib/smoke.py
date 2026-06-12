#!/usr/bin/env python3
"""Reusable smoke-probe library shared by the Compose smoke tasks.

Waits for the API to become healthy, then asserts the health, version, SPA, and
docs endpoints behave as expected. This module only probes a running stack; the
stack lifecycle (build, up, down) is owned by the calling mise task's
`depends`/`depends_post` hooks, so the same probes work against any topology
(dev/standalone, cluster, sentinel).

Uses only the Python standard library so it runs anywhere Python 3 is available.

Examples:
    # From a task wrapper
    from smoke import main
    raise SystemExit(main())

    # Point at a non-default host/port
    main(["--base-url", "http://127.0.0.1:8080"])
"""

from __future__ import annotations

import argparse
import json
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass

DEFAULT_BASE_URL = "http://127.0.0.1:8080"
API_PREFIX = "/api/v1"


class SmokeError(Exception):
    """Raised when a smoke check fails."""


@dataclass
class Response:
    status: int
    body: bytes
    content_type: str
    location: str = ""

    def json(self) -> object:
        return json.loads(self.body)


def http_get(url: str, timeout: float, *, follow_redirects: bool = True) -> Response:
    req = urllib.request.Request(url, method="GET")
    opener = (
        urllib.request.build_opener()
        if follow_redirects
        else urllib.request.build_opener(_NoRedirect)
    )
    try:
        with opener.open(req, timeout=timeout) as resp:  # noqa: S310 (loopback only)
            return Response(
                status=resp.status,
                body=resp.read(),
                content_type=resp.headers.get("Content-Type", ""),
                location=resp.headers.get("Location", ""),
            )
    except urllib.error.HTTPError as exc:
        # Non-2xx (including redirects when not followed) still carries a status,
        # headers, and body we want to inspect.
        return Response(
            status=exc.code,
            body=exc.read(),
            content_type=exc.headers.get("Content-Type", "") if exc.headers else "",
            location=exc.headers.get("Location", "") if exc.headers else "",
        )


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    """Surfaces 3xx responses as HTTPError instead of following them."""

    def redirect_request(self, *_args: object, **_kwargs: object) -> None:
        return None


def wait_for_api(base_url: str, timeout: float) -> None:
    """Poll /livez until it returns 200 or the deadline passes."""
    url = f"{base_url}{API_PREFIX}/livez"
    deadline = time.monotonic() + timeout
    last_err: str | None = None
    attempt = 0
    while time.monotonic() < deadline:
        attempt += 1
        try:
            resp = http_get(url, timeout=2.0)
            if resp.status == 200:
                print(f"API is up after {attempt} attempt(s)")
                return
            last_err = f"status {resp.status}"
        except (urllib.error.URLError, OSError) as exc:
            last_err = str(exc)
        time.sleep(1.0)
    raise SmokeError(f"API did not become healthy within {timeout:.0f}s: {last_err}")


def expect(condition: bool, message: str) -> None:
    if not condition:
        raise SmokeError(message)


def check_livez(base_url: str) -> None:
    resp = http_get(f"{base_url}{API_PREFIX}/livez", timeout=5.0)
    expect(resp.status == 200, f"livez: status {resp.status}, want 200")
    body = resp.json()
    expect(
        isinstance(body, dict) and body.get("status") == "ok",
        f"livez: body {body!r}, want status=ok",
    )
    print("[ok] livez -> 200 ok")


def check_readyz(base_url: str) -> None:
    resp = http_get(f"{base_url}{API_PREFIX}/readyz", timeout=5.0)
    expect(resp.status == 200, f"readyz: status {resp.status}, want 200 (deps healthy)")
    body = resp.json()
    expect(isinstance(body, dict), f"readyz: body {body!r}, want object")
    expect(
        body.get("status") == "ready",
        f"readyz: status {body.get('status')!r}, want ready",
    )
    checks = body.get("checks", {})
    for dep in ("postgres", "redis"):
        dep_status = checks.get(dep, {}).get("status")
        expect(dep_status == "ok", f"readyz: {dep} status {dep_status!r}, want ok")
    print("[ok] readyz -> 200 ready (postgres, redis ok)")


def check_startupz(base_url: str) -> None:
    resp = http_get(f"{base_url}{API_PREFIX}/startupz", timeout=5.0)
    expect(resp.status == 200, f"startupz: status {resp.status}, want 200")
    body = resp.json()
    expect(
        isinstance(body, dict) and body.get("status") == "started",
        f"startupz: body {body!r}, want status=started",
    )
    print("[ok] startupz -> 200 started")


def check_version(base_url: str) -> None:
    resp = http_get(f"{base_url}{API_PREFIX}/version", timeout=5.0)
    expect(resp.status == 200, f"version: status {resp.status}, want 200")
    body = resp.json()
    expect(isinstance(body, dict), f"version: body {body!r}, want object")
    required = ("service", "version", "git_sha", "build_time", "go_version")
    for field in required:
        value = body.get(field)
        expect(
            isinstance(value, str) and value != "",
            f"version: field {field!r} = {value!r}, want non-empty string",
        )
    print(f"[ok] version -> 200 ({body['service']} {body['version']})")


def check_spa(base_url: str) -> None:
    resp = http_get(f"{base_url}/", timeout=5.0)
    expect(resp.status == 200, f"spa root: status {resp.status}, want 200")
    expect(
        "text/html" in resp.content_type,
        f"spa root: content-type {resp.content_type!r}, want text/html",
    )
    print("[ok] / -> 200 text/html (SPA shell)")


def check_openapi_spec(base_url: str) -> None:
    resp = http_get(f"{base_url}{API_PREFIX}/openapi.yaml", timeout=5.0)
    expect(resp.status == 200, f"openapi spec: status {resp.status}, want 200")
    expect(
        "yaml" in resp.content_type,
        f"openapi spec: content-type {resp.content_type!r}, want yaml",
    )
    expect(
        b"openapi:" in resp.body,
        "openapi spec: body does not look like an OpenAPI document",
    )
    print("[ok] /api/v1/openapi.yaml -> 200 (OpenAPI document)")


def check_docs_ui(base_url: str, prefix: str, marker: str) -> None:
    # The bare prefix redirects to prefix/ so the UI's relative asset URLs
    # resolve.
    redirect = http_get(f"{base_url}{prefix}", timeout=5.0, follow_redirects=False)
    expect(
        redirect.status == 301,
        f"{prefix}: status {redirect.status}, want 301 redirect",
    )
    expect(
        redirect.location == f"{prefix}/",
        f"{prefix}: Location {redirect.location!r}, want {prefix + '/'!r}",
    )

    resp = http_get(f"{base_url}{prefix}/", timeout=5.0)
    expect(resp.status == 200, f"{prefix}/: status {resp.status}, want 200")
    expect(
        "text/html" in resp.content_type,
        f"{prefix}/: content-type {resp.content_type!r}, want text/html",
    )
    expect(
        marker.encode() in resp.body,
        f"{prefix}/: shell missing expected marker {marker!r}",
    )
    print(f"[ok] {prefix} -> 301 -> {prefix}/ 200 text/html")


def run_checks(base_url: str, timeout: float) -> None:
    """Run the full probe suite against a running stack."""
    print(f"Waiting for API at {base_url} ...")
    wait_for_api(base_url, timeout)

    check_livez(base_url)
    check_readyz(base_url)
    check_startupz(base_url)
    check_version(base_url)
    check_spa(base_url)
    check_openapi_spec(base_url)
    check_docs_ui(base_url, "/swagger", "swagger-ui")
    check_docs_ui(base_url, "/redoc", "redoc")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument(
        "--base-url",
        default=DEFAULT_BASE_URL,
        help="API base URL (default: %(default)s)",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=120.0,
        help="seconds to wait for the API (default: %(default)s)",
    )
    args = parser.parse_args(argv)

    try:
        run_checks(args.base_url, args.timeout)
        print("\nAll smoke checks passed.")
        return 0
    except SmokeError as exc:
        print(f"\nSMOKE FAILED: {exc}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("\nInterrupted.", file=sys.stderr)
        return 130


if __name__ == "__main__":
    raise SystemExit(main())
