# restgo

A small REST API in Go with JWT authentication, built on the standard library.
No web framework: Go 1.22+ gave `net/http.ServeMux` method-aware routing, which
is enough for an API this size.

## Quick start

```bash
cp .env.example .env
make run          # http://localhost:8080
make test         # go test -race ./...
```

`make help` lists every target.

## Endpoints

| Method | Path                    | Auth   | Purpose                                 |
| ------ | ----------------------- | ------ | --------------------------------------- |
| GET    | `/healthz`              | public | Liveness probe                          |
| POST   | `/api/v1/auth/register` | public | Create an account, returns a token pair |
| POST   | `/api/v1/auth/login`    | public | Exchange credentials for a token pair   |
| POST   | `/api/v1/auth/refresh`  | public | Trade a refresh token for a new pair    |
| GET    | `/api/v1/me`            | Bearer | The caller's own profile                |
| GET    | `/api/v1/users`         | Bearer | List all users                          |

### Walkthrough

```bash
# 1. Register. The response contains the user and both tokens.
curl -s -X POST localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"ada@example.com","name":"Ada Lovelace","password":"sup3rsecret"}'

# 2. Log in and keep the access token.
TOKEN=$(curl -s -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"ada@example.com","password":"sup3rsecret"}' \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["tokens"]["access_token"])')

# 3. Call a protected route.
curl -s localhost:8080/api/v1/me -H "Authorization: Bearer $TOKEN"

# 4. When the access token expires, mint a new pair.
curl -s -X POST localhost:8080/api/v1/auth/refresh \
  -H 'Content-Type: application/json' \
  -d '{"refresh_token":"<refresh token>"}'
```

Every error uses one envelope, so a client only parses one shape:

```json
{ "error": "validation failed", "details": { "password": "must be at least 8 characters" } }
```

`details` only appears on `422` responses.

## Configuration

All settings come from the environment; see `.env.example`.

| Variable            | Default       | Notes                                        |
| ------------------- | ------------- | -------------------------------------------- |
| `JWT_SECRET`        | dev fallback  | **Required** when `APP_ENV=production`       |
| `APP_ENV`           | `development` | `production` enables strict secret checks    |
| `PORT`              | `8080`        |                                              |
| `ACCESS_TOKEN_TTL`  | `15m`         | Any `time.ParseDuration` value               |
| `REFRESH_TOKEN_TTL` | `168h`        | 7 days                                       |
| `JWT_ISSUER`        | `restgo`      | Verified on every token                      |

`config.Load` fails at startup rather than at the first request, and in
production it refuses to boot on a missing, too-short, or default secret.

## Layout

```
cmd/api/            main: config, wiring, graceful shutdown
internal/api/       routes and handlers
internal/config/    environment parsing and validation
internal/httpx/     JSON request decoding and response helpers
internal/middleware/ recovery, request logging, JWT auth, JSON error rewriting
internal/model/     domain types
internal/store/     UserStore interface + in-memory implementation
internal/token/     JWT issuing and verification
```

Handlers depend on the `store.UserStore` interface, not a concrete database. To
move to Postgres, implement that interface and change one line in
`cmd/api/main.go`.

## How the auth works

Login returns two HS256 JWTs. The **access token** is short-lived and sent as
`Authorization: Bearer <jwt>` on every protected call. The **refresh token**
lives much longer and is only used against `/auth/refresh`.

Each token carries a `kind` claim (`access` or `refresh`), and `Verify` requires
the kind the caller expects. This is what stops a long-lived refresh token from
being replayed as an access token on a protected route — a mix-up that otherwise
quietly turns your 15-minute window into a 7-day one.

Other deliberate choices:

- **Passwords** are hashed with bcrypt at default cost and never serialised:
  `User.PasswordHash` is tagged `json:"-"`.
- **Login is uniform.** An unknown email and a wrong password return the same
  `401` and the same message, and the unknown-email path still runs a bcrypt
  comparison against a dummy hash so the timing matches. Without this, response
  timing tells an attacker which addresses are registered.
- **`alg` is pinned** to HS256 via `jwt.WithValidMethods`, and expiry is
  mandatory. An attacker cannot present an `alg: none` token and be believed.
  There's a test for exactly that.
- **Verification failures collapse** into one `ErrInvalidToken`, so the specific
  reason can't leak to a client.
- **Refresh re-reads the user** from the store, so a deleted account stops being
  able to mint new tokens.
- **Request bodies** are capped at 1 MiB and reject unknown fields, so a typo
  like `"passwrd"` fails loudly instead of being silently dropped.
- **Protected routes are wrapped individually** rather than by prefix, so adding
  a route and forgetting a group can't silently expose it.

## Before production

The parts intentionally left out, in rough priority order:

1. **A real database.** The in-memory store loses everything on restart.
2. **Refresh-token revocation.** Tokens are stateless, so logout can't actually
   invalidate one and a stolen refresh token stays valid until it expires.
   Persist a token ID (the `jti` claim is already there) and check it on
   refresh.
3. **Rate limiting** on `/auth/login` and `/auth/register` to slow credential
   stuffing.
4. **TLS**, normally terminated at a load balancer or reverse proxy.
5. **Roles and permissions.** Every authenticated user can currently list all
   users.
6. **CORS**, if a browser on another origin will call this.
# go-lang
