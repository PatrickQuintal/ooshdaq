# Authentication Boundary

$OOSHDAQ has exactly two startup-selected identity modes: `mock` for local
fictional users and `proxy` behind a trusted identity-aware reverse proxy. A
request cannot select or combine modes, and invalid configuration fails
startup.

## Shared Principal

Downstream code receives only:

```text
id            stable non-empty subject
display_name  bounded display label
```

The ID is authoritative; the display name is escaped presentation text and is
never used for authorization. Raw headers, cookies, tokens, groups, and email
addresses do not cross the authentication boundary. All admitted principals
share one member role; introducing roles requires an authorization design and
threat-model update.

## Mock Mode

Mock mode accepts only a configured allowlist of fictional users.
`POST /auth/mock-login` creates a signed, expiring, host-only `HttpOnly`
session cookie, and `POST /auth/logout` clears it. Signatures are compared in
constant time. An optional signing secret must contain at least 32 bytes;
otherwise a process-random 256-bit key ends sessions on restart.

The default listener is loopback. Binding mock mode to another address
requires the explicit unsafe-development override and remains suitable only
when the published port is loopback-only. Mock mode must not accept arbitrary
subjects or proxy identity headers and must use synthetic data.

## Trusted Proxy Mode

Proxy mode reads a subject and optional display label from configured headers.
Header names alone provide no security. The required topology is:

1. clients can reach $OOSHDAQ only through the authenticating proxy;
2. the proxy removes inbound copies of both identity headers;
3. after authentication, it injects exactly one canonical subject and at most
   one display label;
4. untrusted writers cannot reach the proxy-to-application connection; and
5. both proxy and application fail closed when the subject is absent or
   malformed.

Direct access to the application listener or any route around the proxy would
let a client forge identity and is an account-takeover vulnerability.

The proxy adapter rejects missing, repeated, comma-joined, overlong, or
invalid-UTF-8 subject values and rejects repeated, overlong, control-bearing,
or invalid-UTF-8 display names. Principal validation additionally rejects
subject whitespace, slashes, backslashes, and control characters. An omitted
display name is accepted; a new minimal profile uses `$OOSHDAQ user`. Logs
never include identity header values.

Validating an upstream signed token directly would be another authentication
mode with issuer, audience, algorithm, time, key-rotation, and replay rules. It
must not be added inside the header adapter.

## Browser Session and CSRF

`GET /api/v1/session` issues or reuses a random CSRF token in both an
`HttpOnly` cookie and the JSON `csrf_token` field, including before mock
login. State-changing routes require exactly one `X-CSRF-Token` value, the
cookie, and a constant-time exact match. They also reject
`Sec-Fetch-Site: cross-site` when supplied.

Cookies are host-only, `HttpOnly`, and `SameSite=Strict`. TLS mode uses
`Secure` cookies and the `__Host-` prefix. JSON mutations require the declared
media type; bodyless mutations reject a body. Non-browser clients may omit
`Sec-Fetch-Site`, but still need the CSRF cookie and header.

After authenticated bootstrap, `POST /api/v1/activity` may advance the
private `last_seen_at` timestamp at most once per hour. It returns no timestamp
or presence data. Authentication alone is not CSRF protection.

## Failure and Logout Behavior

Missing or invalid identity yields `401` on protected routes. Mock login is
`404` in proxy mode. Proxy-mode logout returns `501` because sign-out belongs
to the proxy or identity provider. Identity resolution failures return a safe
generic error and redacted server log.

The exact session and error envelopes are in the [HTTP API](http-api.md). See
the [threat model](../security/threat-model.md) for abuse cases. Deployment
owners must enforce the proxy topology described above on their chosen
platform.
