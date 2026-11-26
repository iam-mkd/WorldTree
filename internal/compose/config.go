// Package compose turns a normalised compose model (the JSON printed by
// `docker compose config --format json`) into a per-world override file.
//
// The package is pure except for load.go, which shells out to the docker CLI.
// Analysis and rendering never touch Docker or the filesystem, so they are
// golden-tested on captured JSON.
package compose

import (
	"encoding/json"
	"fmt"
)

// Config is the subset of the normalised compose model that WorldTree reads.
// Anything not listed here is left to compose and never rewritten.
type Config struct {
	Name     string             `json:"name"`
	Services map[string]Service `json:"services"`
	Networks map[string]Network `json:"networks"`
	Volumes  map[string]Volume  `json:"volumes"`
}

// Service is one service of the normalised model.
type Service struct {
	ContainerName string                     `json:"container_name"`
	Image         string                     `json:"image"`
	NetworkMode   string                     `json:"network_mode"`
	Pid           string                     `json:"pid"`
	Ipc           string                     `json:"ipc"`
	Ports         []Port                     `json:"ports"`
	Volumes       []Mount                    `json:"volumes"`
	Networks      map[string]*ServiceNetwork `json:"networks"`
	ExternalLinks []string                   `json:"external_links"`
	VolumesFrom   []string                   `json:"volumes_from"`
	Profiles      []string                   `json:"profiles"`
	Environment   map[string]*string         `json:"environment"`
}

// Port is a long-syntax port entry. Compose prints published as a string
// ("5432" or a range "9000-9005"); it is empty for an ephemeral host port.
type Port struct {
	Name      string `json:"name,omitempty"`
	Mode      string `json:"mode,omitempty"`
	HostIP    string `json:"host_ip,omitempty"`
	Target    uint32 `json:"target"`
	Published string `json:"published,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
	AppProto  string `json:"app_protocol,omitempty"`
}

// Mount is a long-syntax volume entry of a service.
type Mount struct {
	Type     string `json:"type"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

// ServiceNetwork is a service's attachment to a network (null in JSON when bare).
type ServiceNetwork struct {
	IPv4Address string `json:"ipv4_address"`
	IPv6Address string `json:"ipv6_address"`
}

// Network is a top-level network.
type Network struct {
	Name     string `json:"name"`
	External bool   `json:"external"`
	Ipam     struct {
		Config []json.RawMessage `json:"config"`
	} `json:"ipam"`
}

// Volume is a top-level named volume.
type Volume struct {
	Name     string `json:"name"`
	External bool   `json:"external"`
}

// ParseConfig decodes the output of `docker compose config --format json`.
func ParseConfig(data []byte) (*Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse compose config: %w", err)
	}
	if c.Name == "" {
		return nil, fmt.Errorf("parse compose config: no project name (is this compose config JSON?)")
	}
	return &c, nil
}
