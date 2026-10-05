# urlshortener-ui

[![CI](https://github.com/SpechtLabs/urlshortener-ui/actions/workflows/ci.yaml/badge.svg)](https://github.com/SpechtLabs/urlshortener-ui/actions/workflows/ci.yaml)
[![Release](https://github.com/SpechtLabs/urlshortener-ui/actions/workflows/release.yaml/badge.svg)](https://github.com/SpechtLabs/urlshortener-ui/actions/workflows/release.yaml)
[![codecov](https://codecov.io/gh/SpechtLabs/urlshortener-ui/graph/badge.svg)](https://codecov.io/gh/SpechtLabs/urlshortener-ui)

The web UI for [urlshortener](https://github.com/SpechtLabs/urlshortener):
users log in with GitHub and create, edit and delete their shortlinks through
urlshortener's API.

The image is published to `ghcr.io/spechtlabs/urlshortener-ui`: `main` for
every commit on main, and the version and `latest` for every release.

## Configuration

`urlshortener-ui serve` reads its configuration from the environment:

| Variable | Meaning |
| --- | --- |
| `CLIENT_ID`, `CLIENT_SECRET` | The GitHub OAuth app users log in with |
| `REDIRECT_URL` | The UI's `/oauth/redirect`, as registered with the OAuth app |
| `DASHBOARD_URL` | The UI's host name, which the login cookie is set for |
| `SHORTLINK_URL` | urlshortener's base URL, for its API and the shortlinks |
| `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_INSECURE` | Where traces and logs go (port 4317 for gRPC, 4318 for HTTP) |

## Development

Every tool is pinned in `.mise.toml`; `mise install` installs them.

```sh
mise run check      # what CI runs: lint, go.mod, tests
mise run serve      # the UI on localhost:8080, against the API at SHORTLINK_URL
mise run image      # build the container image for the local platform
mise tasks          # everything else
```
