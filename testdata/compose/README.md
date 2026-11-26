# Compose fixtures

- `spring-notes/`: Spring Boot + Flyway + Postgres + Redis sample from Spike A, in its original
  colliding form (fixed `container_name`, fixed host ports, named volume).
- `edge/`: synthetic file covering include, anchors, extends, profiles, build, port ranges,
  host networking, fixed subnet + static IP, external volume, absolute bind (Spike B).
- `edge-names/`: synthetic; explicit volume/network `name:`, IPv6 and UDP ports, an ephemeral port next
  to a fixed one, a published range onto one target, read-only / writable / file binds outside the
  project, Docker socket, `network_mode: service:` and `container:`, external_links, volumes_from, pid: host.
- `shape-immich/`, `shape-mastodon/`, `shape-paperless/`: **synthetic** files written to have the same
  shape as the real immich / mastodon / paperless-ngx compose files tested in Spike B (top-level `name:`,
  fixed container names, `.env`-driven binds, loopback ports, internal network, env_file, named volumes).
  They are not copies; the real files (AGPL/GPL) stay out of the repo until the licence question is settled
  (see `spikes/b/SOURCES.md`). Spike B ran the real files.

Captured `docker compose config` JSON and golden outputs live in `internal/compose/testdata/<fixture>/`;
regenerate with `go test ./internal/compose -update` (needs the docker CLI, not a daemon; run on
Linux/macOS, since the path placeholder substitution assumes slash paths).
