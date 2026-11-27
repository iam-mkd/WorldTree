# DECISIONS

## 2026-11-26 — Narrow scope after Spike A (user approved "Narrow, run Spike B")
- Don't build a worktree manager; integrate with worktrunk hooks (`pre-start`, `pre-remove`) instead.
- Defer per-world DB cloning / Postgres slicing: compose-per-world already gives each world its own Postgres.
- Why: worktrunk-style hook + 25-line override covered ~85-90% of the pain in Spike A.

## 2026-11-26 — Rewrite via generated override, not by rewriting the file (Spike B)
- Normalise with `docker compose config --format json`, emit override with `!reset` / `!override`.
- Why: original stays authoritative (relative build contexts, env_file keep working); compose itself resolves
  include/extends/anchors/profiles; resolved secrets are only read in memory, never written.
- Cost: needs Compose >= 2.24.

## 2026-11-26 — Shipping: local CLI, single Go binary (user approved)
- GitHub Releases + Homebrew tap (+ `go install`); no daemon, server, GUI or Docker image.
- Why: everything reconciles from registry + Docker labels; nothing long-running to drift. Tool needs host
  git worktrees and port probing, so a container image doesn't fit; worktrunk has hooks, not plugins.
- Name `worldtree`: no Homebrew formula/cask; 119 GitHub repos contain the word, none a dev CLI. Binary is
  never `wt` (worktrunk's).

## 2026-11-26 — Architecture choices that differ from the brief (see ARCHITECTURE.md)
- Docker CLI (`--format json`), not the Engine API: compose has no Engine API; CLI honours Docker contexts.
- JSON + flock registry, not SQLite: tiny data, no cgo, hand-repairable.
- Windows: best-effort (build + unit tests in CI). ASSUMED — user develops on Windows but did not confirm.
- Third-party compose files (AGPL/GPL) not committed; licence check before using them as testdata.

## 2026-11-26 — Day 2: compose rewrite rules (refines Spike B's rewrite.py)
- Synthetic `shape-*` fixtures instead of the real immich/mastodon/paperless files: licence still open.
  Real-file coverage is Spike B's; swap them in once the licence is decided.
- Ports rendered in long syntax under `!override`, and unpublished (ephemeral) ports are kept.
  rewrite.py dropped them: `!override` replaces the whole list.
- Binds outside the project: writable dirs are remapped under `.worldtree/binds/` (start empty);
  **read-only binds and writable file binds are left shared** with a finding. Remapping a config file or
  cert dir would break the app (a missing source becomes an empty dir). rewrite.py remapped everything.
- Volumes/networks with an explicit `name:` are renamed to `<world project>_<key>`: otherwise every world
  shares them. Detected as `name != <config project>_<key>`.
- `network_mode: container:<x>` is an error (joins a container outside the world); `pid/ipc: host`,
  external_links, `volumes_from: container:` and the Docker socket are warnings.
- Override sets top-level `name:` to the world project, so it is self-contained even without `-p`.
- Lease keys are `service/host_ip/target/proto[#n]`, not list positions, so reordering ports keeps leases.
- `render` exits 1 when any finding is an error, so scripts and agents notice.

## 2026-11-27 — Day 3: registry and port allocation
- Registry at `$WORLDTREE_HOME`, else `~/.local/state/worldtree` (XDG), `~/Library/Application Support/worldtree`,
  `%LocalAppData%\worldtree`. One `registry.json`, plus a separate `registry.lock`: locking the data file itself
  would break on rename.
- Lock is non-blocking flock/LockFileEx polled up to 30 s, so a stuck holder gives a clear error, not a hang.
  The OS drops it on kill -9 (tested).
- A corrupt or newer-schema registry is refused, never overwritten.
- Leases are per port number (TCP and UDP together) and keyed per world by the compose port key. Re-leasing keeps
  existing ports *without re-probing them*, because the world's own containers hold them. Keys no longer requested
  are released.
- Probe: TCP + UDP bind on 127.0.0.1, ::1, 0.0.0.0, ::. A missing address family (no IPv6) is skipped; any other
  bind error, including Windows' excluded port ranges (WSAEACCES), counts as busy.
- Search starts at an FNV hash of the world id within 20000–29999. A recreated world tends to get its old block
  back, and worlds start far apart. That is the useful part of worktrunk's hash_port, without its collisions.
- `render` now asks the real allocator as a dry run (unlocked snapshot, nothing saved); `--port-base` bypasses it.
- Known gap: a port can be taken by another program between leasing and `compose up` (TOCTOU). `up` will surface
  compose's bind error and re-lease on retry (day 8 reconcile), rather than try to hold sockets open.

## 2026-11-27 — Licence: Apache-2.0 (user choice)
- `LICENSE` is the verbatim text from apache.org; copyright holder "The WorldTree Authors".
- Versus MIT it adds an explicit patent grant. Still open: whether the AGPL/GPL
  compose files from Spike B may be committed as testdata (they are not, today).

## 2026-11-27 — World environment lives in `.worldtree/env`, not `.env.worldtree` (Day 6)
- The brief named `<dir>/.env.worldtree`; that file would show as untracked in every user repo. `.worldtree/`
  already ignores itself and is deleted by `down`, so the env file goes there (mode 0600: it holds the
  Postgres password). `worldtree env` prints it as dotenv, sh or PowerShell.
- Spring Boot binds `SPRING_*` names only from real environment variables, so the shell formats are the
  supported way to feed a host-run Spring app; dotenv suits direnv/compose-style loaders.
- Detection is by image name (postgres/postgis/pgvector, redis/valkey/keydb) plus a published standard port.
