# Authentication Boundary

$OOSHDAQ supports exactly two identity acquisition modes:

- `mock` for local development with fictional seeded users;
- `proxy` for deployment behind a trusted identity-aware reverse proxy.

The mode is chosen at process startup with `OOSHDAQ_AUTH_MODE`. It is not selectable per request, and invalid or missing
deployment configuration fails startup.

## Shared Principal

Downstream services receive a normalized principal:

```text
id            stable, non-empty identity key
display_name  bounded display label
```

Only `id` is authoritative. In proxy mode it is the validated proxy subject; local seed IDs use a distinct `mock-` prefix.
One installation must use a stable, collision-free proxy subject namespace. Display names can change and are never used
for authorization. Raw headers, cookies, tokens, group claims, and email addresses do not cross into the domain.

All authenticated principals currently have the same application role. Admission policy belongs at the identity-aware
proxy. Adding roles or administrator actions requires an explicit authorization model and threat-model update.

## Local Mock Mode

Mock mode provides the minimum mechanism needed to exercise multi-member matching:

- users come from a fixed fictional seed set;
- `POST /auth/mock-login` accepts only one of those IDs;
- a signed, expiring `HttpOnly` cookie identifies the selected local user;
- logout expires the cookie;
- cookie parsing uses constant-time signature comparison;
- state-changing requests require the CSRF cookie/token pair described below.

Mock mode is not an identity provider emulator and must never accept arbitrary subjects or identity headers.

Operational controls:

- the bare command binds to `127.0.0.1`; any non-loopback mock listener fails startup unless
  `OOSHDAQ_ALLOW_MOCK_AUTH=true`;
- use that override only for a container whose published host port remains loopback-only;
- use only synthetic data;
- never deploy the mock configuration to a shared cluster;
- ensure proxy-mode images/configuration do not expose the mock-login route.

`OOSHDAQ_SESSION_SECRET` optionally signs local session state and must contain at least 32 bytes when set. When omitted,
$OOSHDAQ generates a process-random 256-bit key, so existing mock sessions end on restart. Proxy mode does not use an
$OOSHDAQ session-signing secret.

## Trusted Proxy Mode

Proxy mode expects these configurable headers by default:

```text
OOSHDAQ_PROXY_USER_ID_HEADER=X-Forwarded-User-ID
OOSHDAQ_PROXY_USER_NAME_HEADER=X-Forwarded-User-Name
```

The names are configuration, not a security control. The security control is the network and proxy topology:

1. clients can reach $OOSHDAQ only through the trusted proxy;
2. the proxy removes both configured identity headers from every inbound request;
3. after successful IdP authentication, the proxy injects one canonical subject value and, when available, one display
   label;
4. the connection from proxy to $OOSHDAQ is protected from untrusted writers;
5. the proxy denies unauthenticated requests before forwarding, and $OOSHDAQ independently fails closed if identity is
   absent or malformed.

Never expose the application Service, pod port, or alternate ingress directly to clients in proxy mode. A browser can set
an `X-Forwarded-User-ID` header; trusting it without the topology above is an account-takeover vulnerability.

## Header Validation

The adapter should:

- require exactly one non-empty subject value and reject comma-joined or repeated ambiguity;
- trim surrounding whitespace, cap the subject at 200 bytes, and reject spaces, control characters, `/`, and `\`;
- accept at most one display-name value, trim it, cap it at 100 bytes, and reject control characters;
- use the safe local label `$OOSHDAQ user` when the proxy omits a display name;
- treat display name as untrusted text and escape it in HTML;
- avoid logging header values;
- preserve the stable proxy subject exactly after transport validation.

If the proxy can pass a signed token instead, validating that token directly would be a separate auth mode with key
rotation, audience, issuer, time, and algorithm requirements. It must not be improvised inside the header adapter.

## Browser Session and CSRF

In proxy mode the upstream proxy may own its own browser session; $OOSHDAQ does not receive or store its tokens. In mock
mode $OOSHDAQ owns only its signed local cookie.

`GET /api/v1/session` issues or reuses a random CSRF token in both an `HttpOnly` cookie and the JSON
`csrf_token` field, including before mock login. The browser sends that value in `X-CSRF-Token` for every `POST` and `PUT`.
The server requires exactly one valid header value, a valid cookie value, and a constant-time exact match.

For every state-changing browser route:

- require `POST`, `PUT`, or the specifically documented method;
- reject `Sec-Fetch-Site: cross-site` when the browser supplies it;
- require JSON on routes with a documented JSON body and reject a body where none is expected;
- use host-only, `HttpOnly`, `SameSite=Strict` cookies;
- use `Secure` and the `__Host-ooshdaq_csrf` cookie name in TLS-served deployment mode;
- do not use `GET` for state changes.

After an authenticated bootstrap, the bundled browser may send the documented
empty, CSRF-protected activity `POST`. The repository advances the private
`last_seen_at` value only when its previous value is at least one hour old.
This is not a presence endpoint: it returns no activity data, and no member can
read another member's exact or relative login time.

Non-browser clients may omit `Sec-Fetch-Site`, but they still need the CSRF cookie and exact header token. `Origin` is not
currently mandatory because non-browser/API clients may omit it. Authentication is not CSRF protection by itself.

## Logout

`POST /auth/logout` clears $OOSHDAQ's mock cookie. In proxy mode, a meaningful sign-out must use the auth proxy/IdP logout
route. The bundled UI hides its local logout control in proxy mode; a deployment shell may instead link to the proxy's
real logout route. $OOSHDAQ must not display “signed out” while the proxy would immediately reauthenticate the same
browser.

## Failure Behavior

- missing, malformed, or conflicting identity: `401 Unauthorized`;
- mock-login route in proxy mode: `404 Not Found`;
- invalid auth configuration: process startup fails;
- repository failure while resolving a trusted principal: generic `500`, with a correlated but redacted server log;
- identity values never appear in error response bodies.

See [threat model](../security/threat-model.md) and
[deployment direction](../operations/kubernetes-gcp.md).
