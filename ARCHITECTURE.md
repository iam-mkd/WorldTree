# WorldTree — Architecture (MVP)

Status: proposed 2026-11-26, after Spike A (`spikes/a/REPORT.md`) and Spike B (`spikes/b/REPORT.md`).

## What it is
A local CLI, `worldtree`, that turns any directory holding a docker-compose project (typically a git worktree)
into an isolated **world**: its own compose project, collision-free host ports, its own containers, networks,
volumes, Postgres and Redis, and a generated `.env.worldtree` the agent or app reads.
It does not create worktrees. worktrunk (or any script or agent) does that and calls `worldtree` from hooks.

Shipped as one static Go binary (macOS arm64/amd64, Linux amd64/arm64; Windows best-effort) via
GitHub Releases + a Homebrew tap. No daemon, no server, no account, no network calls.
Requires on the host: `git`, Docker with Compose >= 2.24 (for `!reset` / `!override`).

## Commands
| Command | Does |
|---|---|
| `worldtree up [dir]` | create or reconcile the world for `dir` (default cwd); blocks until services are healthy |
| `worldtree down [dir]` | remove the world's containers, networks, volumes, port lease and generated files |
| `worldtree ps` | list worlds: dir, branch, state, ports, health, orphan flags |
| `worldtree env [dir]` | print the world's env (for `eval`, direnv, or the agent) |
| `worldtree gc [--yes]` | find orphaned worlds, containers, volumes, networks, leases; dry-run unless `--yes` |
| `worldtree doctor --prove` | prove isolation between all live worlds (see below); exit 1 on any failure |
| `worldtree init` | add worktrunk hooks to `.config/wt.toml` (`pre-start` = `worldtree up`, `pre-remove` = `worldtree down`) |
| `worldtree render [dir]` | print the generated override and findings without touching Docker (debug, used in tests) |

## How `up` works (reconcile, every step idempotent)
1. **Identify**: world id = short hash of the absolute dir path; name = `<repo>-<branch>` (sanitised).
   Compose project name = `wt-<name>-<id>`.
2. **Register intent**: write `{id, dir, state: creating}` to the registry *before* touching Docker.
3. **Analyse**: `docker compose --profile '*' config --format json` → services, ports, binds, networks.
   Unsupported features (`network_mode: host`, fixed ipam subnets / static IPs) → error with an explanation;
   external volumes/networks → warning ("shared, not isolated").
4. **Lease ports**: one port per published host port, from a per-user range (default 20000–29999), under the
   registry lock. Each candidate is probe-bound on `127.0.0.1`, `::1`, `0.0.0.0` and `::` before being leased
   (Spike A: a native Postgres on IPv4 was invisible to Docker's IPv6 bind). Leases are stable per world.
5. **Generate** `<dir>/.worldtree/compose.override.yml`: `container_name: !reset null`, `ports: !override`
   (host_ip preserved), binds outside `dir` remapped under `<dir>/.worldtree/binds/`, and a label
   `dev.worldtree.world=<id>` on every service, network and volume.
6. **Start**: `docker compose -p <project> -f <original> -f <override> up -d --wait`.
7. **Env**: write `<dir>/.env.worldtree` with per-service host ports, plus, for detected Postgres/Redis services
   (by image name), `DATABASE_URL`, `SPRING_DATASOURCE_URL/USERNAME/PASSWORD`, `REDIS_URL`,
   `SPRING_DATA_REDIS_HOST/PORT`. Credentials come from the service's `POSTGRES_*` environment.
8. **Mark ready** in the registry.

A crash at any step leaves either a registry entry in `creating` or labelled Docker resources (or both);
re-running `up` continues from what exists, and `gc` can always find and remove it.
The original compose file is never modified.

## How `gc` decides something is an orphan
Join three sources: the registry, Docker resources with label `dev.worldtree.world`, and the filesystem.
A world is orphaned if its dir no longer exists, or its registry entry is missing, or it has sat in
`creating` longer than a timeout with no running containers. Orphans are removed with
`docker compose -p <project> down -v` (works from labels alone, verified in Spike A),
plus label-filtered `docker volume/network rm` for leftovers, then the lease is released.

## How `doctor --prove` proves isolation
For every pair of live worlds A, B:
- **Ports**: every leased port is bound by A's container only; no port appears in two worlds; probe that each is
  actually reachable on the expected world.
- **Postgres**: connect (pgx) to A on its host port, write a sentinel row with a random token; connect to B and
  assert the token is absent; clean up.
- **Redis**: same with a key (minimal RESP client, no extra dependency).
- **Volumes/binds**: no named volume or bind source is mounted by containers of two different worlds,
  except those explicitly reported as shared (external, system binds like `/etc/localtime`).
Output: a table plus exit code; `--json` for agents and CI.

## Packages
```
cmd/worldtree        CLI wiring (cobra)
internal/registry    JSON state file + flock, atomic write (temp + rename); worlds and port leases
internal/ports       range allocator + multi-address probe
internal/compose     run `docker compose config`, analyse, render override (pure, golden-tested)
internal/world       up/down/reconcile state machine
internal/envfile     .env.worldtree generation (Postgres/Redis/Spring conventions)
internal/docker      thin wrapper over the `docker` CLI with --format json (labels, ps, rm)
internal/gc          orphan detection + removal
internal/doctor      isolation proofs (pgx, RESP)
testdata/compose     immich, mastodon, paperless, edge fixture, Spring sample app
```

## Choices that differ from the original brief
- **Docker CLI, not the Engine API.** Compose has no Engine API, so `up/down` must exec `docker compose`
  anyway. Using the CLI for inspection too means one integration point, and Docker contexts
  (Docker Desktop, OrbStack, colima, rootless) work without extra code. Cost: parse JSON output.
- **JSON + flock registry, not SQLite.** Tens of records; no cgo; easy to inspect and repair by hand.
- **No per-world DB cloning in the MVP.** Each world already runs its own Postgres (Spike A). Template
  cloning only pays off with large seed data; revisit after MVP alongside snapshot/rollback.
- **No worktree creation.** worktrunk already does it well; WorldTree hooks in via `pre-start` (blocking).
  `post-start` runs in the background and lets the agent race the stack (Spike A).

## Testing
- Unit: `internal/compose` golden tests on the 3 real files + edge fixture; registry concurrency
  (N goroutines and N processes allocating simultaneously, no duplicates); port probe against a held socket.
- Integration (build tag `integration`, real Docker, no mocks): two worlds of the Spring sample app,
  `doctor --prove` passes; kill -9 mid-`up` then `gc` leaves zero labelled resources; `rm -rf` of a world dir
  then `gc` cleans it.
- CI: GitHub Actions ubuntu (Docker available) runs unit + integration; macOS and Windows run build + unit
  only (hosted macOS runners have no Docker). macOS integration is run by hand before each release.

## Security: what is and is not isolated
Isolated per world: container names, host ports, compose networks, named volumes, binds inside the world dir,
Postgres data (separate server), Redis data (separate server).
**Not** isolated: the Docker daemon itself (any agent with Docker access can see, stop or exec into every
world), the host filesystem outside the world dir, external volumes/networks, `network_mode: host` (refused),
images and build cache, outbound network access, the host kernel, and secrets in env files (copied into
containers as-is). WorldTree prevents accidental collisions between cooperating agents; it is not a sandbox
against a malicious one.

## Out of scope for the MVP
Cloud, Kubernetes, GUI, LLM calls, agent orchestration, snapshot/rollback, template-DB cloning,
automatic remapping of fixed subnets, rewriting URLs baked into app config.
