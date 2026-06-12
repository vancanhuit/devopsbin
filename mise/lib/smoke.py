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
import re
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass

DEFAULT_BASE_URL = "http://127.0.0.1:8080"
API_PREFIX = "/api/v1"

# RFC 4122 version 4 UUID (any variant nibble 8-b in the documented range).
UUID_V4_RE = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$",
    re.IGNORECASE,
)


class SmokeError(Exception):
    """Raised when a smoke check fails."""


# Bodies longer than this are truncated in the console log to keep output
# readable; the full body is still used by the assertions.
MAX_LOG_BODY = 2000


@dataclass
class Response:
    status: int
    body: bytes
    content_type: str
    location: str = ""

    def json(self) -> object:
        return json.loads(self.body)


def _format_payload(body: bytes, content_type: str) -> str:
    """Render a response/request body for the console log.

    JSON is pretty-printed; other text is shown verbatim; anything that is not
    valid UTF-8 is summarized by its byte length. Long payloads are truncated.
    """
    if not body:
        return "<empty>"
    try:
        text = body.decode("utf-8")
    except UnicodeDecodeError:
        return f"<{len(body)} bytes of binary {content_type or 'data'}>"
    if "json" in content_type:
        try:
            text = json.dumps(json.loads(text), indent=2, sort_keys=True)
        except json.JSONDecodeError:
            pass
    if len(text) > MAX_LOG_BODY:
        text = f"{text[:MAX_LOG_BODY]}\n... (truncated, {len(text)} chars total)"
    return text


def _log_exchange(
    method: str,
    url: str,
    req_headers: dict[str, str] | None,
    resp: Response,
) -> None:
    """Print the request (with any custom headers/body) and response payload."""
    print(f"  --> {method} {url}")
    for name, value in (req_headers or {}).items():
        print(f"      {name}: {value}")
    print(f"  <-- {resp.status} {resp.content_type or '(no content-type)'}")
    if resp.location:
        print(f"      Location: {resp.location}")
    payload = _format_payload(resp.body, resp.content_type)
    print(
        "\n".join(f"      {line}" for line in payload.splitlines()) or "      <empty>"
    )


def http_get(
    url: str,
    timeout: float,
    *,
    follow_redirects: bool = True,
    headers: dict[str, str] | None = None,
    log: bool = True,
) -> Response:
    req = urllib.request.Request(url, method="GET")
    for name, value in (headers or {}).items():
        req.add_header(name, value)
    opener = (
        urllib.request.build_opener()
        if follow_redirects
        else urllib.request.build_opener(_NoRedirect)
    )
    try:
        with opener.open(req, timeout=timeout) as resp:  # noqa: S310 (loopback only)
            response = Response(
                status=resp.status,
                body=resp.read(),
                content_type=resp.headers.get("Content-Type", ""),
                location=resp.headers.get("Location", ""),
            )
    except urllib.error.HTTPError as exc:
        # Non-2xx (including redirects when not followed) still carries a status,
        # headers, and body we want to inspect.
        response = Response(
            status=exc.code,
            body=exc.read(),
            content_type=exc.headers.get("Content-Type", "") if exc.headers else "",
            location=exc.headers.get("Location", "") if exc.headers else "",
        )
    if log:
        _log_exchange("GET", url, headers, response)
    return response


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
            resp = http_get(url, timeout=2.0, log=False)
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


def check_uuid(base_url: str) -> None:
    resp = http_get(f"{base_url}{API_PREFIX}/uuid", timeout=5.0)
    expect(resp.status == 200, f"uuid: status {resp.status}, want 200")
    body = resp.json()
    expect(isinstance(body, dict), f"uuid: body {body!r}, want object")
    value = body.get("uuid")
    expect(
        isinstance(value, str) and UUID_V4_RE.match(value) is not None,
        f"uuid: uuid {value!r}, want a version 4 UUID",
    )
    print("[ok] uuid -> 200 (version 4 UUID)")


def check_ip(base_url: str) -> None:
    resp = http_get(f"{base_url}{API_PREFIX}/ip", timeout=5.0)
    expect(resp.status == 200, f"ip: status {resp.status}, want 200")
    body = resp.json()
    expect(isinstance(body, dict), f"ip: body {body!r}, want object")
    origin = body.get("origin")
    expect(
        isinstance(origin, str) and origin != "",
        f"ip: origin {origin!r}, want non-empty string",
    )
    print(f"[ok] ip -> 200 (origin {origin})")


def check_headers(base_url: str) -> None:
    marker = "smoke-headers-probe"
    resp = http_get(
        f"{base_url}{API_PREFIX}/headers",
        timeout=5.0,
        headers={"X-Smoke-Test": marker},
    )
    expect(resp.status == 200, f"headers: status {resp.status}, want 200")
    body = resp.json()
    expect(isinstance(body, dict), f"headers: body {body!r}, want object")
    headers = body.get("headers")
    expect(isinstance(headers, dict), f"headers: headers {headers!r}, want object")
    values = headers.get("X-Smoke-Test")
    expect(
        isinstance(values, list) and marker in values,
        f"headers: X-Smoke-Test {values!r}, want list containing {marker!r}",
    )
    print("[ok] headers -> 200 (reflects request headers)")


def check_user_agent(base_url: str) -> None:
    ua = "devopsbin-smoke/1.0"
    resp = http_get(
        f"{base_url}{API_PREFIX}/user-agent",
        timeout=5.0,
        headers={"User-Agent": ua},
    )
    expect(resp.status == 200, f"user-agent: status {resp.status}, want 200")
    body = resp.json()
    expect(isinstance(body, dict), f"user-agent: body {body!r}, want object")
    expect(
        body.get("user-agent") == ua,
        f"user-agent: user-agent {body.get('user-agent')!r}, want {ua!r}",
    )
    print("[ok] user-agent -> 200 (reflects User-Agent)")


def check_echo(base_url: str) -> None:
    marker = "smoke-echo-probe"
    resp = http_get(
        f"{base_url}{API_PREFIX}/echo?foo=bar&foo=baz",
        timeout=5.0,
        headers={"X-Smoke-Test": marker},
    )
    expect(resp.status == 200, f"echo: status {resp.status}, want 200")
    body = resp.json()
    expect(isinstance(body, dict), f"echo: body {body!r}, want object")
    expect(
        body.get("method") == "GET", f"echo: method {body.get('method')!r}, want GET"
    )
    expect(
        body.get("path") == f"{API_PREFIX}/echo",
        f"echo: path {body.get('path')!r}, want {API_PREFIX + '/echo'!r}",
    )
    query = body.get("query")
    expect(
        isinstance(query, dict) and query.get("foo") == ["bar", "baz"],
        f"echo: query {query!r}, want foo=[bar, baz]",
    )
    headers = body.get("headers")
    expect(
        isinstance(headers, dict) and marker in headers.get("X-Smoke-Test", []),
        f"echo: headers {headers!r}, want X-Smoke-Test containing {marker!r}",
    )
    origin = body.get("origin")
    expect(
        isinstance(origin, str) and origin != "",
        f"echo: origin {origin!r}, want non-empty string",
    )
    print("[ok] echo -> 200 (reflects method, path, query, headers, origin)")


def check_status(base_url: str) -> None:
    # A teapot is a body-carrying code; assert the echoed code and description.
    resp = http_get(f"{base_url}{API_PREFIX}/status/418", timeout=5.0)
    expect(resp.status == 418, f"status/418: status {resp.status}, want 418")
    body = resp.json()
    expect(
        isinstance(body, dict) and body.get("code") == 418,
        f"status/418: body {body!r}, want code=418",
    )

    # 204 No Content must carry no body.
    no_body = http_get(f"{base_url}{API_PREFIX}/status/204", timeout=5.0)
    expect(no_body.status == 204, f"status/204: status {no_body.status}, want 204")
    expect(no_body.body == b"", f"status/204: body {no_body.body!r}, want empty")

    # Out-of-range codes are rejected with a 400 and an error body.
    bad = http_get(f"{base_url}{API_PREFIX}/status/600", timeout=5.0)
    expect(bad.status == 400, f"status/600: status {bad.status}, want 400")
    bad_body = bad.json()
    expect(
        isinstance(bad_body, dict) and bool(bad_body.get("error")),
        f"status/600: body {bad_body!r}, want non-empty error",
    )
    print("[ok] status/{code} -> echoes code (418), 204 no body, 600 -> 400")


def check_delay(base_url: str) -> None:
    # A short delay returns 200 and echoes the delayed seconds, and the request
    # must actually take at least that long.
    start = time.monotonic()
    resp = http_get(f"{base_url}{API_PREFIX}/delay/1", timeout=10.0)
    elapsed = time.monotonic() - start
    expect(resp.status == 200, f"delay/1: status {resp.status}, want 200")
    body = resp.json()
    expect(
        isinstance(body, dict) and body.get("delay") == 1,
        f"delay/1: body {body!r}, want delay=1",
    )
    expect(elapsed >= 1.0, f"delay/1: elapsed {elapsed:.2f}s, want >= 1s")

    # Negative delays are rejected with a 400 and an error body.
    bad = http_get(f"{base_url}{API_PREFIX}/delay/-1", timeout=5.0)
    expect(bad.status == 400, f"delay/-1: status {bad.status}, want 400")
    bad_body = bad.json()
    expect(
        isinstance(bad_body, dict) and bool(bad_body.get("error")),
        f"delay/-1: body {bad_body!r}, want non-empty error",
    )
    print("[ok] delay/{seconds} -> waits 1s and echoes delay, -1 -> 400")


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
    check_uuid(base_url)
    check_ip(base_url)
    check_headers(base_url)
    check_user_agent(base_url)
    check_echo(base_url)
    check_status(base_url)
    check_delay(base_url)
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
