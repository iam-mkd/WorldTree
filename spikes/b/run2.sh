#!/usr/bin/env bash
# usage: run2.sh <app>   — run 2 rewritten copies of spikes/b/run/<app>/{w1,w2} side by side and prove isolation.
set -uo pipefail
cd "$(dirname "$0")"
app=$1
case $app in
  immich)    svcs="immich-server database redis"; db=database; dbu=postgres; dbn=immich; cache=redis; http=2283; path=/api/server/ping ;;
  mastodon)  svcs="db redis web streaming sidekiq"; db=db; dbu=postgres; dbn=postgres; cache=redis; http=3000; path=/health ;;
  paperless) svcs="broker db webserver"; db=db; dbu=paperless; dbn=paperless; cache=broker; http=8000; path=/accounts/login/ ;;
esac
dc() { (cd run/$app/$1 && set -a && . ./.worldtree.env && set +a && shift && docker compose "$@"); }
for w in w1 w2; do python rewrite.py run/$app/$w $w $([ $w = w1 ] && echo 21000 || echo 22000) >/dev/null; done
for w in w1 w2; do
  if [ $app = mastodon ]; then dc $w up -d --wait db redis >/dev/null 2>&1; dc $w run --rm web bundle exec rails db:setup >/dev/null 2>&1 && echo "$w db:setup ok" || echo "$w db:setup FAILED"; fi
  dc $w up -d $svcs 2>&1 | grep -iE "error|conflict|allocated|overlap" | head -3
done
echo "-- waiting for HTTP on remapped ports"
for w in w1 w2; do
  port=$(dc $w port $(dc $w config --services | grep -E "^(immich-server|web|webserver)$") $http | sed 's/.*://')
  code=000; for i in $(seq 1 60); do code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port$path"); [[ $code =~ ^(200|302)$ ]] && break; sleep 5; done
  echo "$w: http 127.0.0.1:$port$path -> $code"
done
echo "-- sentinel: write in w1, look in w2"
dc w1 exec -T $db psql -U $dbu -d $dbn -qAtc "create table if not exists wt_sentinel(v text); insert into wt_sentinel values ('w1-only')" >/dev/null
for w in w1 w2; do echo "$w db: $(dc $w exec -T $db psql -U $dbu -d $dbn -qAtc "select coalesce((select string_agg(v,',') from wt_sentinel),'') " 2>&1 | grep -oE 'w1-only|does not exist' | head -1 || echo none)"; done
dc w1 exec -T $cache sh -c 'redis-cli set wt_sentinel w1-only 2>/dev/null || valkey-cli set wt_sentinel w1-only' >/dev/null
for w in w1 w2; do echo "$w cache: $(dc $w exec -T $cache sh -c 'redis-cli get wt_sentinel 2>/dev/null || valkey-cli get wt_sentinel')"; done
echo "-- containers / volumes / networks per world"
for w in w1 w2; do p=$app-$w
  echo "$p: $(docker ps --filter label=com.docker.compose.project=$p --format '{{.Names}}' | wc -l) containers, ports: $(docker ps --filter label=com.docker.compose.project=$p --format '{{.Ports}}' | grep -oE '[0-9.]+:[0-9]+->' | sort -u | tr '\n' ' ')"
  echo "   volumes: $(docker volume ls -q --filter label=com.docker.compose.project=$p | tr '\n' ' ') networks: $(docker network ls --filter label=com.docker.compose.project=$p --format '{{.Name}}' | tr '\n' ' ')"
done
