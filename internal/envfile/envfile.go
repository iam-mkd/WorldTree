// Package envfile builds the environment a world's app or agent reads: leased host ports
// per service, and connection settings for the world's Postgres and Redis.
package envfile

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/iam-mkd/WorldTree/internal/compose"
)

// Var is one variable of a world's environment.
type Var struct {
	Key, Value string
}

// World is what Build needs to know about one world.
type World struct {
	ID      string
	Project string
	Config  *compose.Config
	Ports   []compose.PortRequest
	Leases  map[string]int // PortRequest.Key -> host port
}

// Build returns the world's variables in a stable order:
//
//	WORLDTREE_ID, WORLDTREE_PROJECT
//	WORLDTREE_PORT_<SERVICE>_<TARGET>[_UDP]  for every leased port
//	DATABASE_URL, SPRING_DATASOURCE_URL/USERNAME/PASSWORD   for the first Postgres service
//	REDIS_URL, SPRING_DATA_REDIS_HOST/PORT                  for the first Redis/Valkey service
//
// A database or cache is "detected" by image name and needs a published port for its
// standard target (5432, 6379), since apps on the host connect through it.
func Build(w World) []Var {
	vars := []Var{{"WORLDTREE_ID", w.ID}, {"WORLDTREE_PROJECT", w.Project}}
	ports := append([]compose.PortRequest(nil), w.Ports...)
	sort.SliceStable(ports, func(i, j int) bool {
		if ports[i].Service != ports[j].Service {
			return ports[i].Service < ports[j].Service
		}
		if ports[i].Port.Target != ports[j].Port.Target {
			return ports[i].Port.Target < ports[j].Port.Target
		}
		return ports[i].Key < ports[j].Key // stable order for same target on several host IPs
	})
	used := map[string]bool{}
	for _, r := range ports {
		base := "WORLDTREE_PORT_" + envName(r.Service) + "_" + strconv.Itoa(int(r.Port.Target))
		if r.Port.Protocol == "udp" {
			base += "_UDP"
		}
		// Distinct leases can map to one name (same target on two host IPs; services
		// "api-web" and "api_web"), and a suffixed name can equal another service's plain
		// one ("api" 8080 #2 vs service "api_8080" target 2). Take the first unused of
		// base, base_2, base_3...; the order above keeps the result stable.
		key := base
		for n := 2; used[key]; n++ {
			key = base + "_" + strconv.Itoa(n)
		}
		used[key] = true
		vars = append(vars, Var{key, strconv.Itoa(w.Leases[r.Key])})
	}
	if pg, ok := find(w, isPostgres, 5432); ok {
		env := w.Config.Services[pg.Service].Environment
		user := envOr(env, "POSTGRES_USER", "postgres")
		pass := envOr(env, "POSTGRES_PASSWORD", "")
		db := envOr(env, "POSTGRES_DB", user)
		host, port := hostOf(pg), strconv.Itoa(w.Leases[pg.Key])
		u := url.URL{Scheme: "postgres", User: url.UserPassword(user, pass), Host: net.JoinHostPort(host, port), Path: "/" + db}
		if pass == "" {
			u.User = url.User(user)
		}
		vars = append(vars,
			Var{"DATABASE_URL", u.String()},
			Var{"SPRING_DATASOURCE_URL", fmt.Sprintf("jdbc:postgresql://%s/%s", net.JoinHostPort(host, port), url.PathEscape(db))},
			Var{"SPRING_DATASOURCE_USERNAME", user},
			Var{"SPRING_DATASOURCE_PASSWORD", pass},
		)
	}
	if rd, ok := find(w, isRedis, 6379); ok {
		host, port := hostOf(rd), strconv.Itoa(w.Leases[rd.Key])
		vars = append(vars,
			Var{"REDIS_URL", "redis://" + net.JoinHostPort(host, port)},
			Var{"SPRING_DATA_REDIS_HOST", host},
			Var{"SPRING_DATA_REDIS_PORT", port},
		)
	}
	return vars
}

// find returns the leased port of the first service (by name) whose image matches and
// that publishes target.
func find(w World, match func(image string) bool, target uint32) (compose.PortRequest, bool) {
	var best *compose.PortRequest
	for i, r := range w.Ports {
		if r.Port.Target != target || (r.Port.Protocol != "" && r.Port.Protocol != "tcp") {
			continue
		}
		if !match(w.Config.Services[r.Service].Image) {
			continue
		}
		if best == nil || r.Service < best.Service {
			best = &w.Ports[i]
		}
	}
	if best == nil {
		return compose.PortRequest{}, false
	}
	return *best, true
}

// repoName is the last path segment of an image reference, without tag or digest:
// "ghcr.io/immich-app/postgres:14@sha256:..." -> "postgres".
func repoName(image string) string {
	if i := strings.Index(image, "@"); i >= 0 {
		image = image[:i]
	}
	if i := strings.LastIndex(image, "/"); i >= 0 {
		image = image[i+1:]
	}
	if i := strings.Index(image, ":"); i >= 0 {
		image = image[:i]
	}
	return strings.ToLower(image)
}

func isPostgres(image string) bool {
	n := repoName(image)
	return strings.Contains(n, "postgres") || strings.Contains(n, "postgis") || n == "pgvector"
}

func isRedis(image string) bool {
	n := repoName(image)
	return strings.Contains(n, "redis") || strings.Contains(n, "valkey") || n == "keydb"
}

// hostOf is the address a host process uses for a published port.
func hostOf(r compose.PortRequest) string {
	switch ip := r.Port.HostIP; ip {
	case "", "0.0.0.0", "::":
		return "127.0.0.1"
	default:
		return ip
	}
}

func envOr(env map[string]*string, key, def string) string {
	if v, ok := env[key]; ok && v != nil && *v != "" {
		return *v
	}
	return def
}

var nonAlnum = regexp.MustCompile(`[^A-Z0-9]+`)

func envName(s string) string {
	return strings.Trim(nonAlnum.ReplaceAllString(strings.ToUpper(s), "_"), "_")
}
