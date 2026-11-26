# Compose fixtures

- `spring-notes/`: Spring Boot + Flyway + Postgres + Redis sample from Spike A, in its original
  colliding form (fixed `container_name`, fixed host ports, named volume).
- `edge/`: synthetic file covering include, anchors, extends, profiles, build, port ranges,
  host networking, fixed subnet + static IP, external volume, absolute bind (Spike B).

The real immich / mastodon / paperless files from Spike B are not here yet (AGPL/GPL; licence check pending,
see `spikes/b/SOURCES.md`).
