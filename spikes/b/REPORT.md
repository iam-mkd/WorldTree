# Spike B — compose rewriting (2026-11-26)

Env: Windows 11 + Git Bash, Docker 29.9.2, Compose v5.6.1. Files fetched from `main` on 2026-11-26 into `src/`.
Approach (`rewrite.py`, ~110 lines): normalise with `docker compose --profile '*' config --format json`
(resolves include/extends/anchors/interpolation), then emit `docker-compose.worldtree.yml` using
`container_name: !reset null` and `ports: !override [...]`, plus `.worldtree.env` (COMPOSE_PROJECT_NAME, COMPOSE_FILE).
Original compose file is never modified (verified byte-identical). Copies live in `run/<app>/{w1,w2}` = two worktrees.
`run2.sh <app>` starts both copies, checks HTTP on the remapped ports, writes a sentinel row + cache key in w1, checks w2.

## Result: 3/3 real files survive (kill criterion "<2 of 3" not hit)
| App | Shape | Rewrites needed | 2 copies side by side | Sentinel DB / cache in w2 |
|---|---|---|---|---|
| immich | top-level `name:`, 4x `container_name`, bind data dirs via `.env`, env_file | drop 4 container_names, 1 port | HTTP 200 / 200 | absent / absent |
| mastodon | `127.0.0.1:` ports, `internal: true` network, relative binds, env_file, `db:setup` | 2 ports (host_ip kept) | HTTP 200 / 200 | absent / absent |
| paperless | named volumes, `8000:8000`, env_file | 1 port | HTTP 200 / 200 | absent / absent |

Notes:
- `COMPOSE_PROJECT_NAME` overrides immich's top-level `name: immich` (projects became immich-w1/immich-w2).
- Relative bind mounts (immich `./postgres`, mastodon `./postgres14`) are isolated for free because each worktree is a separate dir.
- App failures seen were app config, not rewriting: paperless needs `PAPERLESS_SECRET_KEY`; mastodon needs secrets + `db:setup`.

## Edge fixture (`edge/src`, synthetic): what the real files did not exercise
| Feature | Outcome |
|---|---|
| `include:` | works (included service's port remapped) |
| YAML anchors / `<<:` merge | works (resolved by `compose config`) |
| `extends:` (other file) with `container_name` | works (inherited container_name reset) |
| `profiles:` | works (all profiles rewritten; activation unchanged) |
| `build:` context | works (relative to project dir; image named per project) |
| port range `7100-7102` | works (expanded to 3 ports) |
| absolute bind outside project | remapped to `<world>/.worldtree/binds/...` (override merges by target) |
| external volume | shared; flagged WARN, cannot isolate without changing semantics |
| fixed `ipam` subnet + `ipv4_address` | **fails**: `Pool overlaps` on 2nd copy; flagged UNSUPPORTED (auto-remap possible, not done) |
| `network_mode: host` | flagged UNSUPPORTED (not run) |
| `env_file` | works unchanged |

## Not solved by rewriting (needs app-level handling)
- URLs/ports baked into app config (e.g. mastodon `LOCAL_DOMAIN`, OAuth redirect URLs, `localhost:8080` in env) are not rewritten.
- Host-run apps must read the world's ports (Spike A: `spring.config.import=optional:file:.env[.properties]`).
- Requires Compose >= 2.24 (`!reset` / `!override`).

## Cost / caveats
- Images on disk ~10 GB (estimate of ~3 GB was compressed size). Also pulled unapproved `alpine:latest` (13 MB) for cleanup.
- Not tested: macOS, Linux, real worktrunk, host networking at runtime.
- `rewrite.py` bug fixed during spike: Windows paths in YAML double quotes.
