#!/usr/bin/env python3
"""Smoke test for the Compose `dev` profile.

Waits for the API to become healthy, then asserts the health, version, and SPA
endpoints behave as expected. The stack lifecycle (build, up, down) is owned by
the `smoke:dev` mise task, so this script only probes a running stack.

Uses only the Python standard library so it runs anywhere Python 3 is available.

Examples:
    # Probe the default stack
    scripts/smoke_dev.py

    # Point at a non-default host/port
    scripts/smoke_dev.py --base-url http://127.0.0.1:8080
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

    def json(self) -> object:
        return json.loads(self.body)


def http_get(url: str, timeout: float) -> Response:
    req = urllib.request.Request(url, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:  # noqa: S310 (loopback only)
            return Response(
                status=resp.status,
                body=resp.read(),
                content_type=resp.headers.get("Content-Type", ""),
            )
    except urllib.error.HTTPError as exc:
        # Non-2xx still carries a status and body we want to inspect.
        return Response(
            status=exc.code,
            body=exc.read(),
            content_type=exc.headers.get("Content-Type", "") if exc.headers else "",
        )


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


def main() -> int:
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
    args = parser.parse_args()

    try:
        print(f"Waiting for API at {args.base_url} ...")
        wait_for_api(args.base_url, args.timeout)

        check_livez(args.base_url)
        check_readyz(args.base_url)
        check_startupz(args.base_url)
        check_version(args.base_url)
        check_spa(args.base_url)

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
