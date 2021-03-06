package api

import (
	"fmt"
	"mime"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/system"
	"github.com/docker/docker/pkg/version"
	"github.com/docker/libtrust"
)

// Common constants for daemon and client.
const (
	// Version of Current REST API
	DefaultVersion version.Version = "1.22"

	// MinVersion represents Minimum REST API version supported
	MinVersion version.Version = "1.12"

	// DefaultDockerfileName is the Default filename with Docker commands, read by docker build
	DefaultDockerfileName string = "Dockerfile"

	// NoBaseImageSpecifier is the symbol used by the FROM
	// command to specify that no base image is to be used.
	NoBaseImageSpecifier string = "scratch"
)

// byPortInfo is a temporary type used to sort types.Port by its fields
type byPortInfo []types.Port

func (r byPortInfo) Len() int      { return len(r) }
func (r byPortInfo) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r byPortInfo) Less(i, j int) bool {
	if r[i].PrivatePort != r[j].PrivatePort {
		return r[i].PrivatePort < r[j].PrivatePort
	}

	if r[i].IP != r[j].IP {
		return r[i].IP < r[j].IP
	}

	if r[i].PublicPort != r[j].PublicPort {
		return r[i].PublicPort < r[j].PublicPort
	}

	return r[i].Type < r[j].Type
}

// DisplayablePorts returns formatted string representing open ports of container
// e.g. "0.0.0.0:80->9090/tcp, 9988/tcp"
// it's used by command 'docker ps'
func DisplayablePorts(ports []types.Port) string {
	type portGroup struct {
		first int
		last  int
	}
	groupMap := make(map[string]*portGroup)
	var result []string
	var hostMappings []string
	var groupMapKeys []string
	sort.Sort(byPortInfo(ports))
	for _, port := range ports {
		current := port.PrivatePort
		portKey := port.Type
		if port.IP != "" {
			if port.PublicPort != current {
				hostMappings = append(hostMappings, fmt.Sprintf("%s:%d->%d/%s", port.IP, port.PublicPort, port.PrivatePort, port.Type))
				continue
			}
			portKey = fmt.Sprintf("%s/%s", port.IP, port.Type)
		}
		group := groupMap[portKey]

		if group == nil {
			groupMap[portKey] = &portGroup{first: current, last: current}
			// record order that groupMap keys are created
			groupMapKeys = append(groupMapKeys, portKey)
			continue
		}
		if current == (group.last + 1) {
			group.last = current
			continue
		}

		result = append(result, formGroup(portKey, group.first, group.last))
		groupMap[portKey] = &portGroup{first: current, last: current}
	}
	for _, portKey := range groupMapKeys {
		g := groupMap[portKey]
		result = append(result, formGroup(portKey, g.first, g.last))
	}
	result = append(result, hostMappings...)
	return strings.Join(result, ", ")
}

func formGroup(key string, start, last int) string {
	parts := strings.Split(key, "/")
	groupType := parts[0]
	var ip string
	if len(parts) > 1 {
		ip = parts[0]
		groupType = parts[1]
	}
	group := strconv.Itoa(start)
	if start != last {
		group = fmt.Sprintf("%s-%d", group, last)
	}
	if ip != "" {
		group = fmt.Sprintf("%s:%s->%s", ip, group, group)
	}
	return fmt.Sprintf("%s/%s", group, groupType)
}

// MatchesContentType validates the content type against the expected one
func MatchesContentType(contentType, expectedType string) bool {
	mimetype, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		logrus.Errorf("Error parsing media type: %s error: %v", contentType, err)
	}
	return err == nil && mimetype == expectedType
}

// LoadOrCreateTrustKey attempts to load the libtrust key at the given path,
// otherwise generates a new one
func LoadOrCreateTrustKey(trustKeyPath string) (libtrust.PrivateKey, error) {
	err := system.MkdirAll(filepath.Dir(trustKeyPath), 0700)
	if err != nil {
		return nil, err
	}
	trustKey, err := libtrust.LoadKeyFile(trustKeyPath)
	if err == libtrust.ErrKeyFileDoesNotExist {
		trustKey, err = libtrust.GenerateECP256PrivateKey()
		if err != nil {
			return nil, fmt.Errorf("Error generating key: %s", err)
		}
		if err := libtrust.SaveKey(trustKeyPath, trustKey); err != nil {
			return nil, fmt.Errorf("Error saving key file: %s", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("Error loading key file %s: %s", trustKeyPath, err)
	}
	return trustKey, nil
}
