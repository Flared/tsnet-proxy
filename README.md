# tsnet-proxy

Expose a service on your [Tailscale](https://tailscale.com) tailnet under its
own hostname, without running `tailscaled` next to it.

This is a fork of Tailscale's
[`cmd/tsnet-proxy`](https://github.com/tailscale/tailscale/blob/a00fd3273b3865ec587d0c4b36ab5debf358545e/cmd/tsnet-proxy/tsnet-proxy.go)
with two differences:

- **Identity headers for tagged nodes.** Upstream drops all identity headers
  when the caller is a tagged node (servers, CI runners, …). This version
  always forwards `Tailscale-Node-Name`, and forwards `Tailscale-Node-Tags`
  for tagged nodes, so the backend can authorize machine-to-machine traffic.
- **Configurable target.** Upstream always proxies to `localhost:<port>`.
  Here the target can be any `host:port` or `http(s)://` URL, which makes it
  usable as a sidecar or as a standalone container in front of another
  service.

## Modes

| Mode    | Tailnet listener                 | Default listen port | Headers |
|---------|----------------------------------|---------------------|---------|
| `tcp`   | raw TCP                          | target port         | no      |
| `http`  | HTTP reverse proxy               | 80                  | yes     |
| `https` | HTTPS with an auto-issued cert   | 443                 | yes     |

`https` requires [HTTPS certificates](https://tailscale.com/kb/1153/enabling-https)
to be enabled on the tailnet.

## Identity headers

In `http` and `https` modes, any incoming header starting with `Tailscale-`
is stripped, then the following are set from a Tailscale WhoIs lookup of the
caller:

| Header                       | User nodes | Tagged nodes | Value                                     |
|------------------------------|:----------:|:------------:|-------------------------------------------|
| `Tailscale-Node-Name`        | ✓          | ✓            | MagicDNS FQDN, e.g. `laptop.tail1234.ts.net` |
| `Tailscale-Node-Tags`        |            | ✓            | Comma-separated, e.g. `tag:ci,tag:prod`   |
| `Tailscale-User-Login`       | ✓          |              | Login name, e.g. `alice@example.com`      |
| `Tailscale-User-Name`        | ✓          |              | Display name                              |
| `Tailscale-User-Profile-Pic` | ✓          |              | Profile picture URL                       |

Non-ASCII values are RFC 2047 Q-encoded, as done by `tailscale serve`.
`X-Forwarded-For`, `X-Forwarded-Host` and `X-Forwarded-Proto` are also set.

The backend must only be reachable through the proxy, otherwise clients can
forge these headers.

## Configuration

Every flag can also be set with an environment variable. Flags take
precedence.

| Flag               | Environment variable          | Default             | Description |
|--------------------|-------------------------------|---------------------|-------------|
| `--hostname`       | `TSNET_PROXY_HOSTNAME`        | *required*          | Tailnet hostname to register. |
| `--target`         | `TSNET_PROXY_TARGET`          | *required*          | A port (`8080` → `localhost:8080`), `host:port`, or, in HTTP modes, an `http(s)://` URL (a path is prefixed to requests). |
| `--mode`           | `TSNET_PROXY_MODE`            | `tcp`               | `tcp`, `http` or `https`. |
| `--listen-port`    | `TSNET_PROXY_LISTEN_PORT`     | see [Modes](#modes) | Tailnet port to listen on. |
| `--state-dir`      | `TSNET_PROXY_STATE_DIR`       | per-user config dir (`/var/lib/tsnet-proxy` in Docker) | Where tsnet persists its node state. |
| `--ephemeral`      | `TSNET_PROXY_EPHEMERAL`       | `false`             | Register as an [ephemeral node](https://tailscale.com/kb/1111/ephemeral-nodes). |
| `--advertise-tags` | `TSNET_PROXY_ADVERTISE_TAGS`  |                     | Comma-separated tags, e.g. `tag:proxy`. Required with OAuth clients. |
| `-v`               | `TSNET_PROXY_VERBOSE`         | `false`             | Verbose tsnet logs. |

Authentication is handled by tsnet, using the first of:

- `TS_AUTHKEY`: an [auth key](https://tailscale.com/kb/1085/auth-keys).
- `TS_CLIENT_SECRET`: an [OAuth client secret](https://tailscale.com/kb/1215/oauth-clients),
  used with `--advertise-tags`.
- Otherwise, a login URL is printed to the logs.

The auth key is only used on first start. Once the node state exists in the
state directory, it is reused.

## Usage

```sh
# Raw TCP: expose a remote Postgres as db:5432 on the tailnet.
tsnet-proxy --hostname=db --target=postgres.internal:5432

# HTTPS with identity headers: https://app.<tailnet>.ts.net -> http://localhost:8080
tsnet-proxy --hostname=app --mode=https --target=8080

# HTTPS in front of an HTTPS backend with a path prefix.
tsnet-proxy --hostname=wiki --mode=https --target=https://wiki.internal/app
```

### Docker

```sh
docker run -d \
  -e TS_AUTHKEY=tskey-auth-... \
  -e TSNET_PROXY_HOSTNAME=app \
  -e TSNET_PROXY_MODE=https \
  -e TSNET_PROXY_TARGET=http://backend:8080 \
  -v tsnet-proxy-state:/var/lib/tsnet-proxy \
  ghcr.io/flared/tsnet-proxy:latest
```

The image runs as UID `65532`. Persist `/var/lib/tsnet-proxy` so the node
keeps its identity across restarts, or use `TSNET_PROXY_EPHEMERAL=true` with a
reusable auth key.

## Development

```sh
make ci            # build, vet, test, format-check
make format
make docker-build
make update-deps   # update direct Go dependencies to their latest versions
```

## Releasing

Push a `v*` tag. The [Docker Release](.github/workflows/docker-release.yml)
workflow builds `linux/amd64` and `linux/arm64` images and pushes them to
`ghcr.io/flared/tsnet-proxy`, tagged with the
version (`1.2.3`, `1.2`), the commit SHA and `latest`.

```sh
git tag v0.1.0
git push origin v0.1.0
```

## License

BSD 3-Clause, see [LICENSE](LICENSE). Derived from Tailscale's `tsnet-proxy`.
