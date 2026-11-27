# WorldTree

**Give every parallel AI coding agent its own local runtime, not just its own git worktree.**

`worldtree` is a single-binary CLI that turns a docker-compose project in a worktree into an isolated
**world**: its own compose project, collision-free host ports, no fixed container names, and its own
networks, volumes, Postgres and Redis. Run two or three agents (Claude Code, Codex, Cursor) on the same
app at once without them fighting over `5432`, sharing a database, or leaving orphaned containers behind.

> **Status: pre-alpha, under active development.** `render`, `up`, `down`, `ps` and `env` work today;
> `gc` and `doctor --prove` are being built (see [Roadmap](#roadmap)). There are no releases yet; build
> from source. Interfaces may change.

---

## The problem

Git worktrees isolate **files**, not **runtime**. Start the same compose stack from three worktrees and:

| What happens | Why |
|---|---|
| 2nd and 3rd `docker compose up` fail immediately | fixed `container_name: notes-postgres` is already taken |
| Remove the names, next failure is `port is already allocated` | every copy publishes `5432:5432`, `6379:6379`, `8080:8080` |
| Agents "just reuse the running stack" | now they **share one Postgres**: agent B's Flyway migration crashes with a checksum mismatch, agent C silently runs against a schema its branch doesn't have, and every agent sees the others' data and Redis keys |
| A failed `up` | leaves networks, volumes and `created` containers behind |

All of this was reproduced with a Spring Boot + Flyway + Postgres + Redis app in
[`spikes/a/REPORT.md`](spikes/a/REPORT.md).

## How it works

```
worktree/                          worldtree up
├── docker-compose.yml   ──────►   docker compose config --format json   (compose resolves include/extends/anchors/.env)
├── src/ ...                                │
└── .worldtree/                             ▼
    ├── compose.override.yml   ◄──  analyse + render override   ◄── ports leased from a locked per-user registry
    ├── env  (0600)            ◄──  ports, DATABASE_URL, SPRING_DATASOURCE_*, REDIS_URL
    └── .gitignore  (*)                     │
                                            ▼
                                   docker compose -p wt-<name>-<id> -f <your files> -f .worldtree/compose.override.yml up -d --wait
```

- **Your compose files are never modified.** WorldTree generates an override using Compose's merge tags:
  `container_name: !reset null` drops fixed names, `ports: !override [...]` swaps every published host port
  for a leased one (host IP such as `127.0.0.1` is kept).
- **Unique project name per world** (`wt-<dir>-<id>`), so containers, networks and named volumes are
  namespaced by Compose itself. Every resource gets the label `dev.worldtree.world=<id>`.
- **Collision-free ports.** Ports come from a per-user range (default `20000-29999`) recorded in a
  file-locked registry. Each candidate is probe-bound on `127.0.0.1`, `::1`, `0.0.0.0` and `::` first:
  Docker will happily publish `[::]:5432` while a native Postgres holds `0.0.0.0:5432`, and `localhost`
  then reaches the wrong server.
- **One database per world.** Each world runs its own Postgres and Redis containers, so migrations,
  data and cache keys cannot leak between agents.
- **Crash-safe by design.** The world is recorded in the registry *before* anything is created in Docker,
  and every step of `up` is idempotent: re-running it continues from what exists and keeps the same ports.
- **Refuses what it cannot isolate** instead of pretending: host networking, pinned subnets and static IPs
  are errors; external volumes/networks, `pid: host` and the Docker socket are warnings.

Tested on three real-world compose files (immich, mastodon, paperless-ngx: two copies each ran side by side
with isolated data) plus a synthetic edge-case file. See [`spikes/b/REPORT.md`](spikes/b/REPORT.md).

## Requirements

- Docker with **Compose >= 2.24** (for `!reset` / `!override`; checked at startup)
- git
- To build from source: Go 1.27+

Platforms: macOS and Linux are the targets; Windows is best-effort (built and unit-tested in CI;
the author develops on it with Docker Desktop).

## Install

No releases yet. Build from source:

```sh
git clone https://github.com/iam-mkd/WorldTree.git
cd WorldTree
go install ./cmd/worldtree    # -> $(go env GOPATH)/bin/worldtree
```

`go install` puts the binary in `$(go env GOPATH)/bin` (usually `~/go/bin`); make sure that directory is on
your `PATH` so `worldtree` runs from any worktree. Alternatively, `make build` writes `bin/worldtree` in the
clone; then add the clone's absolute `bin/` path to `PATH` or call the binary by its full path.

Homebrew tap and release binaries are planned for v0.1.0.

## Quickstart

From a worktree that contains a compose project:

```sh
worldtree render      # dry run: print the override, the ports it would lease, and any findings
worldtree up          # create (or reconcile) this worktree's world and wait for it to start
```

`up` waits with `docker compose up --wait`: services that define a `healthcheck` must report healthy;
services without one count as started once their container is running, which can be before the app
inside is ready. Add healthchecks to the services your agent depends on.

`render` on the bundled Spring Boot sample ([`testdata/compose/spring-notes`](testdata/compose/spring-notes)):

```text
ports:
  app                  *:8080 -> 21000  (container 8080/tcp)
  postgres             *:5432 -> 21001  (container 5432/tcp)
  redis                *:6379 -> 21002  (container 6379/tcp)

findings:
  info  container-name         app: fixed container_name "notes-app" dropped
  info  container-name         postgres: fixed container_name "notes-postgres" dropped
  info  container-name         redis: fixed container_name "notes-redis" dropped
```

`up` in two copies of that sample, as two agents would:

```text
$ worldtree up ../agent-a
world 094eee10 ready: project wt-agent-a-094eee10
  app                  *:29726 -> container 8080/tcp
  postgres             *:29727 -> container 5432/tcp
  redis                *:29728 -> container 6379/tcp

$ worldtree up ../agent-b
world 685b5880 ready: project wt-agent-b-685b5880
  app                  *:27749 -> container 8080/tcp
  postgres             *:27750 -> container 5432/tcp
  redis                *:27751 -> container 6379/tcp
```

### Connect an app running on the host

`up` also writes the world's environment to `.worldtree/env`; `worldtree env` prints it. Postgres and
Redis/Valkey services are detected by image name, credentials come from the service's `POSTGRES_*` settings:

```text
$ worldtree env ../agent-b
WORLDTREE_ID='5aca4467'
WORLDTREE_PROJECT='wt-agent-b-5aca4467'
WORLDTREE_PORT_APP_8080='21976'
WORLDTREE_PORT_POSTGRES_5432='21977'
WORLDTREE_PORT_REDIS_6379='21978'
DATABASE_URL='postgres://notes:notes@127.0.0.1:21977/notes'
SPRING_DATASOURCE_URL='jdbc:postgresql://127.0.0.1:21977/notes'
SPRING_DATASOURCE_USERNAME='notes'
SPRING_DATASOURCE_PASSWORD='notes'
REDIS_URL='redis://127.0.0.1:21978'
SPRING_DATA_REDIS_HOST='127.0.0.1'
SPRING_DATA_REDIS_PORT='21978'
```

Spring Boot applies `SPRING_*` names only from real environment variables, so load them into the shell
that starts the app:

```sh
eval "$(worldtree env --format sh)" && ./mvnw spring-boot:run              # bash / zsh
```

```powershell
worldtree env --format powershell | Out-String | Invoke-Expression; ./mvnw spring-boot:run
```

With that, the bundled Spring sample started on the host wrote into world B's Postgres and Redis, and
world A stayed empty.

See every world, then remove one when its agent is done:

```text
$ worldtree ps
ID        PHASE  CONTAINERS   PORTS                                 DIR
153adea6  ready  3/3 running  app:28743 postgres:28744 redis:28745  /work/agent-a
10066047  ready  3/3 running  app:25305 postgres:25306 redis:25307  /work/agent-b  (missing: run `worldtree down /work/agent-b`)

$ worldtree down /work/agent-b
world 10066047 removed: project wt-agent-b-10066047
```

`down` deletes the world's containers, networks and **named volumes, including its database**, then
releases its ports, its registry record and `<dir>/.worldtree`. Your compose files are never touched.
It works even after the worktree directory was deleted (as for `agent-b` above), and re-running it is
harmless.

## Commands

| Command | Status | What it does |
|---|---|---|
| `worldtree render [dir]` | available | Print the override, leased ports (dry run) and findings; touches nothing |
| `worldtree up [dir]` | available | Create or reconcile the world; `docker compose up -d --wait` (healthchecks decide readiness) |
| `worldtree version` | available | Print version, commit and platform |
| `worldtree down [dir]` | available | Remove the world's containers, networks, volumes (incl. its database), ports, registry record and `.worldtree/`; works after the dir is deleted |
| `worldtree ps` | available | List worlds with phase, running containers, ports and dir; flags worlds whose dir is gone |
| `worldtree env [dir]` | available | Print the world's ports, `DATABASE_URL`, `SPRING_DATASOURCE_*`, `REDIS_URL`; `--format dotenv\|sh\|powershell` |
| `worldtree gc [--yes]` | planned | Find and remove orphaned worlds, containers, volumes, networks and leases |
| `worldtree doctor --prove` | planned | Prove isolation: sentinel row/key written in world A is absent in B, ports and volumes not shared |
| `worldtree init` | planned | Add worktrunk `pre-start` / `pre-remove` hooks |

## Using it with worktrunk

WorldTree does not create worktrees; [worktrunk](https://github.com/max-sixty/worktrunk) (or any script) does.
The integration is two blocking hooks in `.config/wt.toml` (add them by hand until `worldtree init` lands):

```toml
pre-start  = "worldtree up"      # pre-* hooks block, so the agent never races a half-started stack
pre-remove = "worldtree down"
```

Use `pre-start`, not `post-start`: worktrunk runs `post-*` hooks in the background.

## Why not worktrunk or binky?

| | worktrunk + hooks | binky | WorldTree |
|---|---|---|---|
| Creates and manages worktrees | yes | — | no (use worktrunk) |
| Per-worktree ports | `hash_port`: deterministic, no availability check, no registry | — | leased from a locked registry, probe-bound on IPv4 + IPv6 |
| Isolates a whole compose stack (names, ports, networks, volumes) | only with a hand-written override per repo | no | generated from any compose file |
| Per-agent Postgres / Redis | DIY | slices Postgres/Redis/Kafka | own Postgres/Redis containers per world |
| Cleans up after crashes or deleted worktrees | `pre-remove` only runs on `wt remove` | — | `gc` from registry + Docker labels (planned) |
| Proves isolation | no | — | `doctor --prove` (planned) |
| Maturity | stable, widely used | alpha | pre-alpha |

**Honest take:** if you only run one or two worlds on one repo, worktrunk's `hash_port` plus a 25-line
override and hook script gets you most of the way; [`spikes/a/REPORT.md`](spikes/a/REPORT.md) shows
exactly that setup. WorldTree is for when you want it generated for any compose file, collision-free,
recoverable after crashes, and provable.

## Security: what is and isn't isolated

WorldTree prevents **accidental** collisions between cooperating agents. It is **not a sandbox** against a
malicious one.

**Isolated per world:** container names, host ports, compose networks, named volumes, bind mounts inside
the world's directory, Postgres data (separate server), Redis data (separate server).

**Not isolated:**
- the Docker daemon itself: any agent with Docker access can see, stop or exec into every world;
- the host filesystem outside the world directory, the host kernel, and outbound network access;
- external volumes and networks (reported as warnings), `pid: host`, `ipc: host`, the Docker socket;
- images and the build cache (shared by design);
- secrets in env files: copied into containers as-is. `docker compose config` output can contain resolved
  secrets; WorldTree keeps it in memory. The one secret it writes to disk is the detected Postgres password,
  in `<dir>/.worldtree/env` (mode 0600 on macOS/Linux, git-ignored, deleted by `down`), and `worldtree env`
  prints it.

`network_mode: host`, pinned network subnets and static container IPs are refused.

## Limitations

- URLs and ports baked into an app's own config (OAuth redirect URLs, `LOCAL_DOMAIN`, a hard-coded
  `localhost:8080`) are not rewritten.
- If another program grabs a leased port after it was leased, re-running `up` keeps failing on it
  (known issue; fix planned).
- Each world runs its own database server: memory cost scales with the number of worlds
  (idle Postgres about 50 MB). Template-DB cloning and snapshots are out of scope for the MVP.

## Roadmap

The 14-day MVP plan is in [`PLAN.md`](PLAN.md), the design in [`ARCHITECTURE.md`](ARCHITECTURE.md), and
the reasoning behind each choice in [`DECISIONS.md`](DECISIONS.md). Next: a real-Docker
integration test in CI, crash reconcile, `gc`, `doctor --prove`, worktrunk `init`, then v0.1.0 with
Homebrew and release binaries.

## Development

```sh
make test     # unit tests (compose acceptance tests are skipped without the docker CLI; no daemon needed)
make lint     # gofmt, go vet, staticcheck
make build
```

Every change goes through a pull request. CI runs lint on Linux and build + tests on Linux, macOS and
Windows.

## License

Copyright 2026 The WorldTree Authors. Licensed under the [Apache License, Version 2.0](LICENSE).
