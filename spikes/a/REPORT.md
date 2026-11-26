# Spike A — gap check (2026-11-26)

Env: Windows 11 + Git Bash, Docker 29.9.2, Compose v5.6.1. Spring Boot 3.5 + Flyway + Postgres 17 + Redis 7.
App: `sample/` (sources; scratch git repo `app/` is not committed), worktrees `wt/agent-{1,2,3}`. agent-1/agent-2 add conflicting `V2__priority.sql`.
worktrunk binary NOT installed (not approved); its hook + `hash_port` behaviour emulated from docs (v0.83.0).

## Naive run (typical compose: fixed container_name, fixed host ports, named volume)
1. `container_name` → agent-2/3 `up` fails immediately ("name already in use").
2. Failed `up` leaves orphans: `agent-2_default`, `agent-3_default` networks + `agent-{2,3}_pgdata` volumes.
3. Remove container_name → next failure: `Bind for 0.0.0.0:6379 failed: port is already allocated`; 3 containers left in `created`.
4. Agent "reuses the running stack" (most common agent move):
   - agent-3 (no V2 locally) starts **silently** on a schema with agent-1's V2 column (Flyway ignores future migrations).
   - agent-2 crashes: `Migration checksum mismatch for migration version 2`.
   - agent-3's write is visible in agent-1's app; Redis counter shared.
5. Unplanned: a native Windows Postgres on `0.0.0.0:5432`; Docker still published `[::]:5432` without error,
   and `localhost` hit the native server ("password authentication failed"). Port-free checks must probe all address families.
   Volumes were already namespaced (project name = worktree dir basename), but only by luck of distinct dir names.

## worktrunk-style fix (`sample/docker-compose.worldtree.yml` + `sample/.config/wt-hook.sh`, ~25 lines)
- Override with Compose `!reset null` (container_name) and `!override` (ports); hook writes `.env` with
  COMPOSE_PROJECT_NAME, COMPOSE_FILE, hash_port-style ports. Plain `docker compose up` then works per worktree.
- Result: 3 stacks up concurrently; data, Redis, volumes, networks isolated; agent-1 `priority integer`, agent-2 `priority text`.
- Host-run app works with `spring.config.import=optional:file:.env[.properties]`.
- Worktree `rm -rf`'d outside wt → stack keeps running (pre-remove never fires), but
  `docker compose -p notes-agent-3 down -v` removes everything from labels alone, no compose file needed.
- Cost per world idle: app 332 MiB, postgres 53 MiB, redis 3 MiB. DB isolation came free (one Postgres per world).

## Remaining gaps
- hash_port: no availability check, no registry. P(any collision) with 3 ports/world: 3 worlds 0.4%, 10 worlds 4.3%,
  20 worlds 16.3%. Deterministic, so a colliding branch pair collides every time. Native services are not detected.
- Override is hand-written per repo (needs service names + which ports). Automating it = Spike B.
- worktrunk `post-start` runs in background → agent can race the stack; must use blocking `pre-start`.
- No proof of isolation; no gc for worlds whose worktree vanished (but a label-based gc is ~10 lines).
- Shared-Postgres/template-clone only matters for large seed data; compose-per-world already isolates DBs.
