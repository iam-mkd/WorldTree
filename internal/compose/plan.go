package compose

import (
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// LabelWorld is put on every service, network and volume a world creates, so
// `gc` can find them from Docker alone.
const LabelWorld = "dev.worldtree.world"

// Severity of a finding. Error means `up` must refuse: the world would not be isolated.
type Severity int

// Severities, in increasing order.
const (
	Info Severity = iota
	Warn
	Error
)

func (s Severity) String() string {
	switch s {
	case Info:
		return "info"
	case Warn:
		return "warn"
	default:
		return "error"
	}
}

// Finding is something the analysis wants a human (or agent) to know.
type Finding struct {
	Severity Severity
	Service  string // empty for project-level findings
	Code     string // stable, machine-readable
	Message  string
}

func (f Finding) String() string {
	where := f.Service
	if where == "" {
		where = "(project)"
	}
	return fmt.Sprintf("%-5s %-22s %s: %s", f.Severity, f.Code, where, f.Message)
}

// PortRequest is one published host port that needs a leased replacement.
// Key is stable across runs as long as the service's port entry is unchanged,
// so a world keeps its ports when `up` is re-run.
type PortRequest struct {
	Key     string
	Service string
	Port    Port
}

// Options identify the world the override is rendered for.
type Options struct {
	WorldID    string // short id, value of the dev.worldtree.world label
	Project    string // compose project name of the world
	ProjectDir string // absolute directory holding the compose project (the worktree)
	// IsFile reports whether a bind source is an existing regular file.
	// Nil means "treat every source as a directory". Injected so analysis stays pure.
	IsFile func(path string) bool
}

// Plan is the analysed form of a compose project for one world.
type Plan struct {
	Ports    []PortRequest
	Findings []Finding

	opts     Options
	services map[string]*servicePlan
	volumes  map[string]resource // non-external top-level volumes
	networks map[string]resource // non-external top-level networks
}

type resource struct {
	rename string // new explicit name, empty to keep compose's <project>_<key>
}

type servicePlan struct {
	resetContainerName bool
	ports              []portEntry // complete replacement list; nil = leave ports alone
	binds              []Mount     // bind mounts re-pointed inside the world dir
}

type portEntry struct {
	key  string // lease key; empty = keep the entry unchanged (ephemeral host port)
	port Port
}

// systemBinds are host paths that are shared on purpose and left alone.
var systemBinds = []string{"/etc/localtime", "/etc/timezone", "/dev", "/sys", "/proc", "/lib/modules", "/run/udev"}

// NewPlan analyses cfg for the world described by opts.
func NewPlan(cfg *Config, opts Options) (*Plan, error) {
	if opts.WorldID == "" || opts.Project == "" || opts.ProjectDir == "" {
		return nil, fmt.Errorf("compose plan: WorldID, Project and ProjectDir are required")
	}
	p := &Plan{
		opts:     opts,
		services: map[string]*servicePlan{},
		volumes:  map[string]resource{},
		networks: map[string]resource{},
	}
	for _, name := range sortedKeys(cfg.Services) {
		p.services[name] = p.analyseService(name, cfg.Services[name])
	}
	for _, key := range sortedKeys(cfg.Volumes) {
		v := cfg.Volumes[key]
		if v.External {
			p.add(Warn, "", "external-volume", fmt.Sprintf("volume %q (%s) is external: shared by every world, not isolated", key, v.Name))
			continue
		}
		p.volumes[key] = p.namespace("volume", cfg.Name, key, v.Name)
	}
	for _, key := range sortedKeys(cfg.Networks) {
		n := cfg.Networks[key]
		if n.External {
			p.add(Warn, "", "external-network", fmt.Sprintf("network %q (%s) is external: shared by every world, not isolated", key, n.Name))
			continue
		}
		if len(n.Ipam.Config) > 0 {
			p.add(Error, "", "fixed-subnet", fmt.Sprintf("network %q pins an ipam subnet; a second world fails with 'Pool overlaps'. Remove the ipam config or use worldtree on one world only", key))
		}
		p.networks[key] = p.namespace("network", cfg.Name, key, n.Name)
	}
	return p, nil
}

// HasErrors reports whether any finding makes the world non-isolatable.
func (p *Plan) HasErrors() bool {
	for _, f := range p.Findings {
		if f.Severity == Error {
			return true
		}
	}
	return false
}

func (p *Plan) add(sev Severity, svc, code, msg string) {
	p.Findings = append(p.Findings, Finding{Severity: sev, Service: svc, Code: code, Message: msg})
}

// namespace decides whether a top-level volume/network carries an explicit name.
// Compose names them <project>_<key> unless `name:` is set; an explicit name is the
// same in every world, so it is rewritten to <world project>_<key>.
func (p *Plan) namespace(kind, project, key, name string) resource {
	if name == "" || name == project+"_"+key {
		return resource{}
	}
	newName := p.opts.Project + "_" + key
	p.add(Info, "", "explicit-name", fmt.Sprintf("%s %q has fixed name %q; renamed to %q", kind, key, name, newName))
	return resource{rename: newName}
}

func (p *Plan) analyseService(name string, s Service) *servicePlan {
	sp := &servicePlan{}
	if s.ContainerName != "" {
		sp.resetContainerName = true
		p.add(Info, name, "container-name", fmt.Sprintf("fixed container_name %q dropped", s.ContainerName))
	}

	switch {
	case s.NetworkMode == "host":
		p.add(Error, name, "network-mode-host", "network_mode: host shares the host's ports with every world; cannot isolate")
	case strings.HasPrefix(s.NetworkMode, "container:"):
		p.add(Error, name, "network-mode-container", fmt.Sprintf("network_mode %q joins a container outside the world; use service:<name> instead", s.NetworkMode))
	}
	if s.Pid == "host" {
		p.add(Warn, name, "pid-host", "pid: host sees processes of every world")
	}
	if s.Ipc == "host" {
		p.add(Warn, name, "ipc-host", "ipc: host shares IPC with every world")
	}
	for _, l := range s.ExternalLinks {
		p.add(Warn, name, "external-link", fmt.Sprintf("external_links %q points outside the world (shared)", l))
	}
	for _, v := range s.VolumesFrom {
		if strings.HasPrefix(v, "container:") {
			p.add(Warn, name, "volumes-from-container", fmt.Sprintf("volumes_from %q mounts a container outside the world (shared)", v))
		}
	}
	for _, net := range sortedKeys(s.Networks) {
		if sn := s.Networks[net]; sn != nil && (sn.IPv4Address != "" || sn.IPv6Address != "") {
			p.add(Error, name, "static-ip", fmt.Sprintf("static IP on network %q collides between worlds", net))
		}
	}

	// In host/container network mode docker ignores ports; nothing to lease.
	if len(s.Ports) > 0 && s.NetworkMode != "host" && !strings.HasPrefix(s.NetworkMode, "container:") {
		p.planPorts(name, s.Ports, sp)
	}
	for _, m := range s.Volumes {
		if m.Type == "bind" {
			p.planBind(name, m, sp)
		}
	}
	return sp
}

func (p *Plan) planPorts(svc string, ports []Port, sp *servicePlan) {
	seen := map[string]int{}
	changed := false
	for _, pt := range ports {
		if pt.Published == "" {
			sp.ports = append(sp.ports, portEntry{port: pt}) // ephemeral host port: no collision
			continue
		}
		proto := pt.Protocol
		if proto == "" {
			proto = "tcp"
		}
		key := fmt.Sprintf("%s/%s/%d/%s", svc, pt.HostIP, pt.Target, proto)
		seen[key]++
		if n := seen[key]; n > 1 {
			key = fmt.Sprintf("%s#%d", key, n)
		}
		if strings.Contains(pt.Published, "-") {
			p.add(Info, svc, "port-range", fmt.Sprintf("published range %s for target %d replaced by one leased port", pt.Published, pt.Target))
		}
		p.Ports = append(p.Ports, PortRequest{Key: key, Service: svc, Port: pt})
		sp.ports = append(sp.ports, portEntry{key: key, port: pt})
		changed = true
	}
	if !changed {
		sp.ports = nil
	}
}

func (p *Plan) planBind(svc string, m Mount, sp *servicePlan) {
	src := m.Source
	if src == p.opts.ProjectDir || within(src, p.opts.ProjectDir) {
		return // each worktree has its own copy: isolated for free
	}
	if src == "/var/run/docker.sock" || src == "/run/docker.sock" {
		p.add(Warn, svc, "docker-socket", "mounts the Docker socket: this container can control every world")
		return
	}
	for _, sys := range systemBinds {
		if within(src, sys) || src == sys {
			p.add(Info, svc, "system-bind", fmt.Sprintf("system path %s mounted as is (shared, expected)", src))
			return
		}
	}
	if m.ReadOnly {
		p.add(Info, svc, "shared-readonly-bind", fmt.Sprintf("read-only bind %s outside the project is shared (cannot collide)", src))
		return
	}
	if p.opts.IsFile != nil && p.opts.IsFile(src) {
		p.add(Warn, svc, "shared-file-bind", fmt.Sprintf("writable file bind %s outside the project is shared by every world (not remapped: would become an empty dir)", src))
		return
	}
	dst := path.Join(filepath.ToSlash(p.opts.ProjectDir), ".worldtree", "binds", svc, sanitizePath(m.Target))
	p.add(Info, svc, "bind-remapped", fmt.Sprintf("writable bind %s outside the project -> %s (starts empty)", src, dst))
	sp.binds = append(sp.binds, Mount{Type: "bind", Source: dst, Target: m.Target})
}

// within reports whether p is strictly inside dir. Both are compared in slash form;
// on Windows the comparison is case-insensitive.
func within(p, dir string) bool {
	a := path.Clean(filepath.ToSlash(p))
	b := path.Clean(filepath.ToSlash(dir))
	if runtime.GOOS == "windows" {
		a, b = strings.ToLower(a), strings.ToLower(b)
	}
	if b == "/" {
		return a != "/"
	}
	return strings.HasPrefix(a, b+"/")
}

func sanitizePath(target string) string {
	s := strings.Trim(target, "/")
	if s == "" {
		return "root"
	}
	return strings.NewReplacer("/", "_", ":", "_", "\\", "_").Replace(s)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
