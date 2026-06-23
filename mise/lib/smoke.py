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
import ipaddress
import json
import re
import ssl
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass

DEFAULT_BASE_URL = "http://127.0.0.1:8080"
API_PREFIX = "/api/v1"

# Optional TLS verification context, shared by every request. When set (via
# use_ca_cert), HTTPS requests verify the server certificate against the given
# CA bundle with hostname checking ON, mirroring production behaviour with a
# private CA. It stays None for plain-HTTP stacks.
_SSL_CONTEXT: ssl.SSLContext | None = None


def use_ca_cert(ca_cert: str | None) -> None:
    """Install a CA bundle used to verify HTTPS server certificates.

    Passing None (the default) leaves requests on the system default, which is
    only used for plain-HTTP stacks. The context keeps full chain and hostname
    verification enabled -- there is deliberately no insecure fallback.
    """
    global _SSL_CONTEXT
    if not ca_cert:
        _SSL_CONTEXT = None
        return
    ctx = ssl.create_default_context(cafile=ca_cert)
    ctx.check_hostname = True
    ctx.verify_mode = ssl.CERT_REQUIRED
    _SSL_CONTEXT = ctx


def use_client_cert(cert: str, key: str) -> None:
    """Add a client certificate to the shared TLS context for mutual TLS.

    Requires a CA context (call use_ca_cert first). Once installed, HTTPS
    requests present this certificate when the server requests one, satisfying a
    backend configured with RequireAndVerifyClientCert.
    """
    global _SSL_CONTEXT
    if _SSL_CONTEXT is None:
        raise SmokeError(
            "use_client_cert requires a CA context (call use_ca_cert first)"
        )
    _SSL_CONTEXT.load_cert_chain(certfile=cert, keyfile=key)


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
    set_cookies: tuple[str, ...] = ()

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
    handlers: list[urllib.request.BaseHandler] = []
    if not follow_redirects:
        handlers.append(_NoRedirect())
    if _SSL_CONTEXT is not None:
        handlers.append(urllib.request.HTTPSHandler(context=_SSL_CONTEXT))
    opener = urllib.request.build_opener(*handlers)
    try:
        with opener.open(req, timeout=timeout) as resp:  # noqa: S310 (loopback only)
            response = Response(
                status=resp.status,
                body=resp.read(),
                content_type=resp.headers.get("Content-Type", ""),
                location=resp.headers.get("Location", ""),
                set_cookies=tuple(resp.headers.get_all("Set-Cookie") or []),
            )
    except urllib.error.HTTPError as exc:
        # Non-2xx (including redirects when not followed) still carries a status,
        # headers, and body we want to inspect.
        response = Response(
            status=exc.code,
            body=exc.read(),
            content_type=exc.headers.get("Content-Type", "") if exc.headers else "",
            location=exc.headers.get("Location", "") if exc.headers else "",
            set_cookies=tuple(exc.headers.get_all("Set-Cookie") or [])
            if exc.headers
            else (),
        )
    if log:
        _log_exchange("GET", url, headers, response)
    return response


def http_send(
    url: str,
    method: str,
    timeout: float,
    *,
    data: bytes | None = None,
    headers: dict[str, str] | None = None,
    log: bool = True,
) -> Response:
    """Issue a request that may carry a body (POST/PUT/PATCH/DELETE)."""
    req = urllib.request.Request(url, data=data, method=method)
    for name, value in (headers or {}).items():
        req.add_header(name, value)
    handlers: list[urllib.request.BaseHandler] = []
    if _SSL_CONTEXT is not None:
        handlers.append(urllib.request.HTTPSHandler(context=_SSL_CONTEXT))
    opener = urllib.request.build_opener(*handlers)
    try:
        with opener.open(req, timeout=timeout) as resp:  # noqa: S310 (loopback only)
            response = Response(
                status=resp.status,
                body=resp.read(),
                content_type=resp.headers.get("Content-Type", ""),
                location=resp.headers.get("Location", ""),
                set_cookies=tuple(resp.headers.get_all("Set-Cookie") or []),
            )
    except urllib.error.HTTPError as exc:
        response = Response(
            status=exc.code,
            body=exc.read(),
            content_type=exc.headers.get("Content-Type", "") if exc.headers else "",
            location=exc.headers.get("Location", "") if exc.headers else "",
            set_cookies=tuple(exc.headers.get_all("Set-Cookie") or [])
            if exc.headers
            else (),
        )
    if log:
        _log_exchange(method, url, headers, response)
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
    scheme = body.get("scheme")
    expect(
        scheme in ("http", "https"),
        f"echo: scheme {scheme!r}, want 'http' or 'https'",
    )
    expect(
        body.get("body") is None,
        f"echo: body {body.get('body')!r}, want null for GET",
    )
    print("[ok] echo -> 200 (reflects method, path, query, headers, origin, scheme)")

    payload = "smoke-echo-body"
    resp = http_send(
        f"{base_url}{API_PREFIX}/echo",
        "POST",
        timeout=5.0,
        data=payload.encode("utf-8"),
        headers={"Content-Type": "text/plain"},
    )
    expect(resp.status == 200, f"echo POST: status {resp.status}, want 200")
    body = resp.json()
    expect(isinstance(body, dict), f"echo POST: body {body!r}, want object")
    expect(
        body.get("method") == "POST",
        f"echo POST: method {body.get('method')!r}, want POST",
    )
    expect(
        body.get("body") == payload,
        f"echo POST: body {body.get('body')!r}, want {payload!r}",
    )
    print("[ok] echo POST -> 200 (reflects method and request body)")


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


# The default cookie names the backend issues; mirror the AUTH_* defaults so the
# probe reads the CSRF token and tracks the session cookie.
SESSION_COOKIE = "devopsbin_session"
CSRF_COOKIE = "devopsbin_csrf"


def _update_cookies(jar: dict[str, str], resp: Response) -> None:
    """Merge a response's Set-Cookie headers into the cookie jar.

    A cookie cleared by the server (empty value, as logout does) is removed from
    the jar so subsequent requests stop sending it.
    """
    for raw in resp.set_cookies:
        name_value = raw.split(";", 1)[0].strip()
        if "=" not in name_value:
            continue
        name, value = name_value.split("=", 1)
        if value == "":
            jar.pop(name, None)
        else:
            jar[name] = value


def _cookie_header(jar: dict[str, str]) -> str:
    return "; ".join(f"{name}={value}" for name, value in jar.items())


def check_auth(base_url: str) -> None:
    """Exercise the cookie-session auth flow end to end.

    register -> me (200) -> logout without CSRF (403) -> logout with CSRF (204)
    -> me (401) -> password reset roundtrip (request token, reset, replay 410,
    login with the new password), plus a login with the seeded demo user. This
    proves bcrypt auth, the session cookie, the session-bound double-submit CSRF
    guard, server-side revocation on logout, and single-use password-reset
    tokens.
    """
    username = f"smoke-{int(time.time() * 1000)}"
    password = "smoke-password"
    jar: dict[str, str] = {}

    # register opens an authenticated session and sets both cookies.
    resp = http_send(
        f"{base_url}{API_PREFIX}/auth/register",
        "POST",
        timeout=5.0,
        data=json.dumps({"username": username, "password": password}).encode("utf-8"),
        headers={"Content-Type": "application/json"},
    )
    expect(resp.status == 201, f"auth/register: status {resp.status}, want 201")
    body = resp.json()
    expect(
        isinstance(body, dict) and body.get("username") == username,
        f"auth/register: body {body!r}, want username={username!r}",
    )
    _update_cookies(jar, resp)
    expect(
        SESSION_COOKIE in jar and CSRF_COOKIE in jar,
        f"auth/register: cookies {sorted(jar)!r}, want session and csrf cookies",
    )
    csrf_token = jar[CSRF_COOKIE]
    print("[ok] auth/register -> 201 (sets session + csrf cookies)")

    # me returns the freshly-registered identity for the session cookie.
    resp = http_get(
        f"{base_url}{API_PREFIX}/auth/me",
        timeout=5.0,
        headers={"Cookie": _cookie_header(jar)},
    )
    expect(resp.status == 200, f"auth/me: status {resp.status}, want 200")
    body = resp.json()
    expect(
        isinstance(body, dict) and body.get("username") == username,
        f"auth/me: body {body!r}, want username={username!r}",
    )
    print("[ok] auth/me -> 200 (returns the session user)")

    # logout without the CSRF header is rejected by the double-submit guard.
    resp = http_send(
        f"{base_url}{API_PREFIX}/auth/logout",
        "POST",
        timeout=5.0,
        headers={"Cookie": _cookie_header(jar)},
    )
    expect(
        resp.status == 403,
        f"auth/logout (no csrf): status {resp.status}, want 403",
    )
    print("[ok] auth/logout (no CSRF) -> 403 (double-submit guard)")

    # logout with the matching CSRF header succeeds and clears the cookies.
    resp = http_send(
        f"{base_url}{API_PREFIX}/auth/logout",
        "POST",
        timeout=5.0,
        headers={"Cookie": _cookie_header(jar), "X-CSRF-Token": csrf_token},
    )
    expect(resp.status == 204, f"auth/logout: status {resp.status}, want 204")
    _update_cookies(jar, resp)
    print("[ok] auth/logout -> 204 (revokes session, clears cookies)")

    # The revoked session no longer authenticates /auth/me.
    resp = http_get(
        f"{base_url}{API_PREFIX}/auth/me",
        timeout=5.0,
        headers={"Cookie": f"{SESSION_COOKIE}={csrf_token}"},
    )
    expect(
        resp.status == 401,
        f"auth/me (after logout): status {resp.status}, want 401",
    )
    print("[ok] auth/me (after logout) -> 401 (session revoked)")

    # A single-use reset token rotates the registered user's password.
    resp = http_send(
        f"{base_url}{API_PREFIX}/auth/password/reset-request",
        "POST",
        timeout=5.0,
        data=json.dumps({"username": username}).encode("utf-8"),
        headers={"Content-Type": "application/json"},
    )
    expect(
        resp.status == 200,
        f"auth/password/reset-request: status {resp.status}, want 200",
    )
    body = resp.json()
    reset_token = body.get("token") if isinstance(body, dict) else None
    expect(
        isinstance(reset_token, str) and reset_token != "",
        f"auth/password/reset-request: body {body!r}, want a token",
    )
    print("[ok] auth/password/reset-request -> 200 (issues a reset token)")

    new_password = "smoke-password-2"
    resp = http_send(
        f"{base_url}{API_PREFIX}/auth/password/reset",
        "POST",
        timeout=5.0,
        data=json.dumps({"token": reset_token, "newPassword": new_password}).encode(
            "utf-8"
        ),
        headers={"Content-Type": "application/json"},
    )
    expect(resp.status == 200, f"auth/password/reset: status {resp.status}, want 200")
    print("[ok] auth/password/reset -> 200 (sets a new password)")

    # The token is single-use: replaying it is rejected with 410 Gone.
    resp = http_send(
        f"{base_url}{API_PREFIX}/auth/password/reset",
        "POST",
        timeout=5.0,
        data=json.dumps({"token": reset_token, "newPassword": new_password}).encode(
            "utf-8"
        ),
        headers={"Content-Type": "application/json"},
    )
    expect(
        resp.status == 410,
        f"auth/password/reset (replay): status {resp.status}, want 410",
    )
    print("[ok] auth/password/reset (replay) -> 410 (token is single-use)")

    # The reset password now logs the registered user in.
    resp = http_send(
        f"{base_url}{API_PREFIX}/auth/login",
        "POST",
        timeout=5.0,
        data=json.dumps({"username": username, "password": new_password}).encode(
            "utf-8"
        ),
        headers={"Content-Type": "application/json"},
    )
    expect(
        resp.status == 200,
        f"auth/login (after reset): status {resp.status}, want 200",
    )
    print("[ok] auth/login (after reset) -> 200 (reset password works)")

    # The seeded demo user can log in with its documented credentials.
    resp = http_send(
        f"{base_url}{API_PREFIX}/auth/login",
        "POST",
        timeout=5.0,
        data=json.dumps({"username": "alice", "password": "alicepass"}).encode("utf-8"),
        headers={"Content-Type": "application/json"},
    )
    expect(resp.status == 200, f"auth/login (alice): status {resp.status}, want 200")
    body = resp.json()
    expect(
        isinstance(body, dict) and body.get("username") == "alice",
        f"auth/login (alice): body {body!r}, want username='alice'",
    )
    print("[ok] auth/login (alice) -> 200 (seeded demo user)")


def check_admin(base_url: str) -> None:
    """Exercise the role-restricted admin surface end to end.

    admin login -> list users/accounts/transfers (200) -> unlock and
    password-reset a user (with CSRF) -> a non-admin user is denied (403). This
    proves the session role gates the admin endpoints and that the admin
    mutations honor the double-submit CSRF guard.
    """
    # The seeded admin logs in; capture the session and CSRF cookies.
    admin_jar: dict[str, str] = {}
    resp = http_send(
        f"{base_url}{API_PREFIX}/auth/login",
        "POST",
        timeout=5.0,
        data=json.dumps({"username": "admin", "password": "adminpass"}).encode("utf-8"),
        headers={"Content-Type": "application/json"},
    )
    expect(resp.status == 200, f"admin login: status {resp.status}, want 200")
    _update_cookies(admin_jar, resp)
    expect(
        SESSION_COOKIE in admin_jar and CSRF_COOKIE in admin_jar,
        f"admin login: cookies {sorted(admin_jar)!r}, want session and csrf cookies",
    )
    admin_csrf = admin_jar[CSRF_COOKIE]
    print("[ok] admin login -> 200 (sets session + csrf cookies)")

    # GET /admin/users returns the user list, including the admin itself.
    resp = http_get(
        f"{base_url}{API_PREFIX}/admin/users",
        timeout=5.0,
        headers={"Cookie": _cookie_header(admin_jar)},
    )
    expect(resp.status == 200, f"admin/users: status {resp.status}, want 200")
    body = resp.json()
    users = body.get("users") if isinstance(body, dict) else None
    expect(
        isinstance(users, list) and len(users) > 0,
        f"admin/users: body {body!r}, want a non-empty users list",
    )
    admin_id = next(
        (u.get("id") for u in users if u.get("username") == "admin"),
        None,
    )
    expect(
        isinstance(admin_id, str) and admin_id != "",
        f"admin/users: no admin id in {users!r}",
    )
    print("[ok] admin/users -> 200 (lists users for an admin session)")

    # GET /admin/accounts and /admin/transfers are admin-only reads.
    for name in ("accounts", "transfers"):
        resp = http_get(
            f"{base_url}{API_PREFIX}/admin/{name}",
            timeout=5.0,
            headers={"Cookie": _cookie_header(admin_jar)},
        )
        expect(resp.status == 200, f"admin/{name}: status {resp.status}, want 200")
        key = name
        body = resp.json()
        expect(
            isinstance(body, dict) and isinstance(body.get(key), list),
            f"admin/{name}: body {body!r}, want a {key} list",
        )
        print(f"[ok] admin/{name} -> 200 (admin-only read)")

    # An admin mutation without the CSRF header is rejected (double-submit guard).
    resp = http_send(
        f"{base_url}{API_PREFIX}/admin/users/{admin_id}/unlock",
        "POST",
        timeout=5.0,
        headers={"Cookie": _cookie_header(admin_jar)},
    )
    expect(
        resp.status == 403,
        f"admin/unlock (no csrf): status {resp.status}, want 403",
    )
    print("[ok] admin/users/{id}/unlock (no CSRF) -> 403 (double-submit guard)")

    # With the CSRF header the unlock succeeds (a no-op when not locked).
    resp = http_send(
        f"{base_url}{API_PREFIX}/admin/users/{admin_id}/unlock",
        "POST",
        timeout=5.0,
        headers={"Cookie": _cookie_header(admin_jar), "X-CSRF-Token": admin_csrf},
    )
    expect(resp.status == 204, f"admin/unlock: status {resp.status}, want 204")
    print("[ok] admin/users/{id}/unlock -> 204 (clears login lockout)")

    # password-reset mints a single-use token for the target user.
    resp = http_send(
        f"{base_url}{API_PREFIX}/admin/users/{admin_id}/password-reset",
        "POST",
        timeout=5.0,
        headers={"Cookie": _cookie_header(admin_jar), "X-CSRF-Token": admin_csrf},
    )
    expect(
        resp.status == 200,
        f"admin/password-reset: status {resp.status}, want 200",
    )
    body = resp.json()
    expect(
        isinstance(body, dict)
        and isinstance(body.get("token"), str)
        and body["token"] != "",
        f"admin/password-reset: body {body!r}, want a token",
    )
    print("[ok] admin/users/{id}/password-reset -> 200 (mints a reset token)")

    # A non-admin session is forbidden from every admin endpoint.
    username = f"smoke-rbac-{int(time.time() * 1000)}"
    user_jar: dict[str, str] = {}
    resp = http_send(
        f"{base_url}{API_PREFIX}/auth/register",
        "POST",
        timeout=5.0,
        data=json.dumps({"username": username, "password": "smoke-password"}).encode(
            "utf-8"
        ),
        headers={"Content-Type": "application/json"},
    )
    expect(resp.status == 201, f"rbac register: status {resp.status}, want 201")
    _update_cookies(user_jar, resp)
    resp = http_get(
        f"{base_url}{API_PREFIX}/admin/users",
        timeout=5.0,
        headers={"Cookie": _cookie_header(user_jar)},
    )
    expect(
        resp.status == 403,
        f"admin/users (non-admin): status {resp.status}, want 403",
    )
    print("[ok] admin/users (non-admin) -> 403 (role-gated)")


def check_database(base_url: str) -> None:
    """Exercise the authenticated account-transfer flow end to end.

    alice login -> list accounts (200) -> transfer alice's funds to another
    account (200, balances reflect) -> transfer without CSRF (403) -> transfer
    from an account alice does not own (403). This proves the session gate, the
    double-submit CSRF guard, ownership enforcement, and that the transaction
    actually moves money.
    """
    jar: dict[str, str] = {}
    resp = http_send(
        f"{base_url}{API_PREFIX}/auth/login",
        "POST",
        timeout=5.0,
        data=json.dumps({"username": "alice", "password": "alicepass"}).encode("utf-8"),
        headers={"Content-Type": "application/json"},
    )
    expect(resp.status == 200, f"db login (alice): status {resp.status}, want 200")
    _update_cookies(jar, resp)
    csrf = jar[CSRF_COOKIE]
    print("[ok] db login (alice) -> 200 (sets session + csrf cookies)")

    # GET /accounts lists every account for any signed-in user.
    resp = http_get(
        f"{base_url}{API_PREFIX}/accounts",
        timeout=5.0,
        headers={"Cookie": _cookie_header(jar)},
    )
    expect(resp.status == 200, f"accounts: status {resp.status}, want 200")
    body = resp.json()
    accounts = body.get("accounts") if isinstance(body, dict) else None
    expect(
        isinstance(accounts, list) and len(accounts) >= 2,
        f"accounts: body {body!r}, want at least two accounts",
    )
    print("[ok] accounts -> 200 (lists accounts for a signed-in user)")

    # Pick alice's account as the source and any other account as the
    # destination, so the ownership check passes for the happy path.
    source = next((a for a in accounts if a.get("ownerUsername") == "alice"), None)
    dest = next((a for a in accounts if a.get("id") != (source or {}).get("id")), None)
    expect(
        isinstance(source, dict) and isinstance(dest, dict),
        f"accounts: could not find an alice-owned source and a distinct destination in {accounts!r}",
    )
    src_before = source["balanceCents"]
    dst_before = dest["balanceCents"]
    amount = 2500

    # A transfer without the CSRF header is rejected by the double-submit guard.
    resp = http_send(
        f"{base_url}{API_PREFIX}/transfer",
        "POST",
        timeout=5.0,
        data=json.dumps(
            {
                "fromAccountId": source["id"],
                "toAccountId": dest["id"],
                "amountCents": amount,
            }
        ).encode("utf-8"),
        headers={"Cookie": _cookie_header(jar), "Content-Type": "application/json"},
    )
    expect(resp.status == 403, f"transfer (no csrf): status {resp.status}, want 403")
    print("[ok] transfer (no CSRF) -> 403 (double-submit guard)")

    # With the CSRF header the transfer commits and returns the new balances.
    resp = http_send(
        f"{base_url}{API_PREFIX}/transfer",
        "POST",
        timeout=5.0,
        data=json.dumps(
            {
                "fromAccountId": source["id"],
                "toAccountId": dest["id"],
                "amountCents": amount,
            }
        ).encode("utf-8"),
        headers={
            "Cookie": _cookie_header(jar),
            "Content-Type": "application/json",
            "X-CSRF-Token": csrf,
        },
    )
    expect(resp.status == 200, f"transfer: status {resp.status}, want 200")
    body = resp.json()
    expect(
        isinstance(body, dict)
        and body.get("fromBalanceCents") == src_before - amount
        and body.get("toBalanceCents") == dst_before + amount,
        f"transfer: body {body!r}, want balances debited/credited by {amount}",
    )
    print("[ok] transfer -> 200 (atomically moves funds; balances reflect)")

    # The new balances are reflected on a fresh read of /accounts.
    resp = http_get(
        f"{base_url}{API_PREFIX}/accounts",
        timeout=5.0,
        headers={"Cookie": _cookie_header(jar)},
    )
    expect(
        resp.status == 200, f"accounts (after transfer): status {resp.status}, want 200"
    )
    after = {a["id"]: a["balanceCents"] for a in resp.json().get("accounts", [])}
    expect(
        after.get(source["id"]) == src_before - amount
        and after.get(dest["id"]) == dst_before + amount,
        f"accounts (after transfer): balances {after!r} do not reflect the transfer",
    )
    print("[ok] accounts (after transfer) -> 200 (persisted balances reflect)")

    # alice cannot move funds out of an account she does not own.
    not_owned = next(
        (a for a in accounts if a.get("ownerUsername") != "alice"),
        None,
    )
    if isinstance(not_owned, dict):
        resp = http_send(
            f"{base_url}{API_PREFIX}/transfer",
            "POST",
            timeout=5.0,
            data=json.dumps(
                {
                    "fromAccountId": not_owned["id"],
                    "toAccountId": source["id"],
                    "amountCents": amount,
                }
            ).encode("utf-8"),
            headers={
                "Cookie": _cookie_header(jar),
                "Content-Type": "application/json",
                "X-CSRF-Token": csrf,
            },
        )
        expect(
            resp.status == 403,
            f"transfer (not owner): status {resp.status}, want 403",
        )
        print("[ok] transfer (not owner) -> 403 (ownership enforced)")


def run_checks(base_url: str, timeout: float, expected_scheme: str = "http") -> None:
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
    check_scheme(base_url, expected_scheme)
    check_echo(base_url)
    check_status(base_url)
    check_delay(base_url)
    check_auth(base_url)
    check_admin(base_url)
    check_database(base_url)
    check_spa(base_url)
    check_openapi_spec(base_url)
    check_docs_ui(base_url, "/swagger", "swagger-ui")
    check_docs_ui(base_url, "/redoc", "redoc")


# An IP that is never a real peer in the stack; used to prove that a spoofed
# X-Forwarded-For is ignored when the caller is not a trusted proxy.
SPOOFED_FORWARDED_FOR = "203.0.113.99"


def _is_ip(value: object) -> bool:
    if not isinstance(value, str):
        return False
    try:
        ipaddress.ip_address(value)
    except ValueError:
        return False
    return True


def check_forwarded_ignored(base_url: str) -> None:
    """A direct caller's X-Forwarded-For must be ignored (no trusted proxy).

    The api-tls-direct service configures no trusted proxies, so a forged
    X-Forwarded-For header must not influence the reported origin -- it should
    reflect the real connecting peer instead.
    """
    resp = http_get(
        f"{base_url}{API_PREFIX}/ip",
        timeout=5.0,
        headers={"X-Forwarded-For": SPOOFED_FORWARDED_FOR},
    )
    expect(resp.status == 200, f"ip(spoof): status {resp.status}, want 200")
    body = resp.json()
    origin = body.get("origin") if isinstance(body, dict) else None
    expect(_is_ip(origin), f"ip(spoof): origin {origin!r}, want an IP address")
    expect(
        origin != SPOOFED_FORWARDED_FOR,
        f"ip(spoof): origin {origin!r} must not honor a spoofed "
        f"X-Forwarded-For from an untrusted caller",
    )
    print("[ok] /ip ignores spoofed X-Forwarded-For from an untrusted caller")


def check_forwarded_honored(base_url: str, proxy_ip: str) -> None:
    """Behind a trusted proxy, the real client and scheme must be recovered.

    Caddy terminates TLS and forwards X-Forwarded-For / X-Forwarded-Proto to
    api-tls-proxied, which trusts Caddy's address. The reported origin must be
    the forwarded client (not Caddy's own address), and the reflected
    X-Forwarded-Proto must show the original https scheme.
    """
    ip_resp = http_get(f"{base_url}{API_PREFIX}/ip", timeout=5.0)
    expect(ip_resp.status == 200, f"ip(proxied): status {ip_resp.status}, want 200")
    ip_body = ip_resp.json()
    origin = ip_body.get("origin") if isinstance(ip_body, dict) else None
    expect(_is_ip(origin), f"ip(proxied): origin {origin!r}, want an IP address")
    expect(
        origin != proxy_ip,
        f"ip(proxied): origin {origin!r} equals the proxy address {proxy_ip!r}; "
        f"the forwarded client IP was not honored",
    )

    hdr_resp = http_get(f"{base_url}{API_PREFIX}/headers", timeout=5.0)
    expect(
        hdr_resp.status == 200, f"headers(proxied): status {hdr_resp.status}, want 200"
    )
    hdr_body = hdr_resp.json()
    headers = hdr_body.get("headers") if isinstance(hdr_body, dict) else None
    expect(
        isinstance(headers, dict), f"headers(proxied): headers {headers!r}, want object"
    )
    proto = headers.get("X-Forwarded-Proto")
    expect(
        isinstance(proto, list) and "https" in proto,
        f"headers(proxied): X-Forwarded-Proto {proto!r}, want list containing 'https'",
    )
    print(
        f"[ok] /ip honors the forwarded client ({origin}) and X-Forwarded-Proto=https"
    )


def check_scheme(base_url: str, want: str) -> None:
    """The /scheme endpoint must report the expected request scheme.

    On the direct path the server terminates TLS, so it observes https itself;
    on the proxied path Caddy terminates TLS and forwards X-Forwarded-Proto,
    which the trusted-proxy handling recovers as https.
    """
    resp = http_get(f"{base_url}{API_PREFIX}/scheme", timeout=5.0)
    expect(resp.status == 200, f"scheme: status {resp.status}, want 200")
    body = resp.json()
    scheme = body.get("scheme") if isinstance(body, dict) else None
    expect(scheme == want, f"scheme: got {scheme!r}, want {want!r}")
    print(f"[ok] /scheme reports {want}")


def run_tls_checks(
    direct_url: str, proxied_url: str, proxy_ip: str, timeout: float
) -> None:
    """Probe both TLS topologies: direct HTTPS and the Caddy-proxied path."""
    print(f"\n== Direct HTTPS: {direct_url} ==")
    run_checks(direct_url, timeout, expected_scheme="https")
    check_forwarded_ignored(direct_url)

    print(f"\n== Proxied (Caddy TLS termination): {proxied_url} ==")
    wait_for_api(proxied_url, timeout)
    check_livez(proxied_url)
    check_forwarded_honored(proxied_url, proxy_ip)
    check_scheme(proxied_url, "https")


def check_mtls_rejected(base_url: str) -> None:
    """Without a client certificate the mTLS server must reject the connection.

    The current TLS context must NOT carry a client certificate (call this
    before use_client_cert). The backend serves HTTPS with
    RequireAndVerifyClientCert, so the handshake must fail rather than return a
    response.
    """
    try:
        http_get(f"{base_url}{API_PREFIX}/livez", timeout=5.0, log=False)
    except (urllib.error.URLError, ssl.SSLError, OSError) as exc:
        print(
            f"[ok] mTLS rejects a connection with no client certificate ({type(exc).__name__})"
        )
        return
    raise SmokeError("mTLS server accepted a connection without a client certificate")


def run_mtls_checks(
    direct_url: str,
    proxied_url: str,
    proxy_ip: str,
    client_cert: str,
    client_key: str,
    timeout: float,
) -> None:
    """Probe mutual TLS: rejection without a client cert, then both topologies.

    The shared TLS context starts with the CA only (set by the caller), so the
    rejection check runs first. After installing the client certificate, the
    direct path authenticates to the backend, and the proxied path goes through
    Caddy, which re-encrypts to the backend with its own client certificate.
    """
    print("\n== Direct mTLS without a client certificate (must be rejected) ==")
    check_mtls_rejected(direct_url)

    use_client_cert(client_cert, client_key)

    print(f"\n== Direct mTLS: {direct_url} ==")
    run_checks(direct_url, timeout, expected_scheme="https")
    check_forwarded_ignored(direct_url)

    print(f"\n== Proxied (Caddy re-encrypting to mTLS backend): {proxied_url} ==")
    wait_for_api(proxied_url, timeout)
    check_livez(proxied_url)
    check_forwarded_honored(proxied_url, proxy_ip)
    check_scheme(proxied_url, "https")


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
    parser.add_argument(
        "--ca-cert",
        default=None,
        help="PEM CA bundle to verify the server certificate for https URLs",
    )
    args = parser.parse_args(argv)

    try:
        use_ca_cert(args.ca_cert)
        run_checks(args.base_url, args.timeout)
        print("\nAll smoke checks passed.")
        return 0
    except SmokeError as exc:
        print(f"\nSMOKE FAILED: {exc}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("\nInterrupted.", file=sys.stderr)
        return 130


def main_tls(argv: list[str] | None = None) -> int:
    """Entry point for the TLS smoke task: direct HTTPS + Caddy-proxied paths."""
    parser = argparse.ArgumentParser(
        description=run_tls_checks.__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--direct-url",
        default="https://localhost:8443",
        help="direct-HTTPS api base URL (default: %(default)s)",
    )
    parser.add_argument(
        "--proxied-url",
        default="https://localhost:9443",
        help="Caddy-terminated proxied base URL (default: %(default)s)",
    )
    parser.add_argument(
        "--proxy-ip",
        default="172.16.7.10",
        help="Caddy's trusted address, which must NOT appear as the proxied "
        "origin (default: %(default)s)",
    )
    parser.add_argument(
        "--ca-cert",
        required=True,
        help="PEM CA bundle to verify the mkcert-issued server certificates",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=120.0,
        help="seconds to wait for each API (default: %(default)s)",
    )
    args = parser.parse_args(argv)

    try:
        use_ca_cert(args.ca_cert)
        run_tls_checks(args.direct_url, args.proxied_url, args.proxy_ip, args.timeout)
        print("\nAll TLS smoke checks passed.")
        return 0
    except SmokeError as exc:
        print(f"\nSMOKE FAILED: {exc}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("\nInterrupted.", file=sys.stderr)
        return 130


def main_mtls(argv: list[str] | None = None) -> int:
    """Entry point for the mTLS smoke task: direct mTLS + re-encrypting proxy."""
    parser = argparse.ArgumentParser(
        description=run_mtls_checks.__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--direct-url",
        default="https://localhost:8444",
        help="direct mTLS api base URL (default: %(default)s)",
    )
    parser.add_argument(
        "--proxied-url",
        default="https://localhost:9444",
        help="Caddy-terminated proxied base URL (default: %(default)s)",
    )
    parser.add_argument(
        "--proxy-ip",
        default="172.16.8.10",
        help="Caddy's trusted address, which must NOT appear as the proxied "
        "origin (default: %(default)s)",
    )
    parser.add_argument(
        "--ca-cert",
        required=True,
        help="PEM CA bundle to verify the mkcert-issued server certificates",
    )
    parser.add_argument(
        "--client-cert",
        required=True,
        help="PEM client certificate presented to the mTLS backend",
    )
    parser.add_argument(
        "--client-key",
        required=True,
        help="PEM private key for --client-cert",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=120.0,
        help="seconds to wait for each API (default: %(default)s)",
    )
    args = parser.parse_args(argv)

    try:
        use_ca_cert(args.ca_cert)
        run_mtls_checks(
            args.direct_url,
            args.proxied_url,
            args.proxy_ip,
            args.client_cert,
            args.client_key,
            args.timeout,
        )
        print("\nAll mTLS smoke checks passed.")
        return 0
    except SmokeError as exc:
        print(f"\nSMOKE FAILED: {exc}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("\nInterrupted.", file=sys.stderr)
        return 130


if __name__ == "__main__":
    raise SystemExit(main())
