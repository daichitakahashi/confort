package confort

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/daichitakahashi/confort/internal/beacon"
	"github.com/daichitakahashi/confort/wait"
	"github.com/docker/cli/cli/command/image/build"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/lestrrat-go/backoff/v2"
	"go.uber.org/multierr"
)

type (
	Backend interface {
		Namespace(ctx context.Context, namespace string) (Namespace, error)
		BuildImage(ctx context.Context, buildContext io.Reader, buildOptions types.ImageBuildOptions, force bool, buildOut io.Writer) error
	}

	Namespace interface {
		Namespace() string
		Network() *types.NetworkResource

		CreateContainer(ctx context.Context, name string, container *container.Config, host *container.HostConfig,
			network *network.NetworkingConfig, configConsistency bool,
			wait *wait.Waiter, pullOptions *types.ImagePullOptions, pullOut io.Writer) (string, error)
		StartContainer(ctx context.Context, name string) (Ports, error)
		Release(ctx context.Context) error
	}
)

type Ports nat.PortMap

// Binding returns the first value associated with the given container port.
// If there are no values associated with the port, Binding returns zero value.
// To access multiple values, use the nat.PortMap directly.
func (p Ports) Binding(port nat.Port) (b nat.PortBinding) {
	bindings := p[port]
	if len(bindings) == 0 {
		return b
	}
	return bindings[0]
}

// HostPort returns "host:port" style string of the first value associated with the given container port.
// If there are no values associated with the port, HostPort returns empty string.
func (p Ports) HostPort(port nat.Port) string {
	bindings := p[port]
	if len(bindings) == 0 {
		return ""
	}
	return bindings[0].HostIP + ":" + bindings[0].HostPort
}

// URL returns "scheme://host:port" style string of the first value associated with the given container port.
// If there are no values associated with the port, URL returns empty string.
// And if scheme is empty, use "http" as a default scheme.
func (p Ports) URL(port nat.Port, scheme string) string {
	if scheme == "" {
		scheme = "http"
	}
	if s := p.HostPort(port); s != "" {
		return fmt.Sprintf("%s://%s", scheme, s)
	}
	return ""
}

type ResourcePolicy string

const (
	ResourcePolicyError    ResourcePolicy = beacon.ResourcePolicyError
	ResourcePolicyReuse    ResourcePolicy = beacon.ResourcePolicyReuse
	ResourcePolicyReusable ResourcePolicy = beacon.ResourcePolicyReusable
	ResourcePolicyTakeOver ResourcePolicy = beacon.ResourcePolicyTakeOver
)

type dockerBackend struct {
	cli    *client.Client // inject
	policy ResourcePolicy
	labels map[string]string
}

func (d *dockerBackend) Namespace(ctx context.Context, namespace string) (Namespace, error) {
	networkName := namespace
	namespace += "-"

	var nw *types.NetworkResource
	// create network if not exists
	list, err := d.cli.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, err
	}
	for _, n := range list {
		if n.Name == networkName {
			if d.policy == ResourcePolicyError {
				return nil, fmt.Errorf("dockerBackend: network %q already exists", networkName)
			}
			nw = &n
			break
		}
	}

	var nwCreated bool
	if nw == nil {
		resp, err := d.cli.NetworkCreate(ctx, networkName, types.NetworkCreate{
			Driver:         "bridge",
			CheckDuplicate: true,
			Labels:         d.labels,
		})
		if err != nil {
			return nil, err
		}
		n, err := d.cli.NetworkInspect(ctx, resp.ID, types.NetworkInspectOptions{
			Verbose: true,
		})
		if err != nil {
			return nil, err
		}
		nwCreated = true
		nw = &n
	}

	// resolve host ip
	hostIP, err := resolveHostIP(d.cli.DaemonHost(), nw.IPAM)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve docker host ip: %w", err)
	}

	var term []func(context.Context) error
	if (nwCreated && d.policy != ResourcePolicyReusable) || d.policy == ResourcePolicyTakeOver {
		term = append(term, func(ctx context.Context) error {
			return d.cli.NetworkRemove(ctx, nw.ID)
		})
	}

	return &dockerNamespace{
		dockerBackend: d,
		namespace:     namespace,
		network:       nw,
		hostIP:        hostIP,
		labels:        d.labels,
		terminate:     term,
		containers:    map[string]*containerInfo{},
	}, nil
}

// see: https://github.com/testcontainers/testcontainers-go/blob/34481cf9027b79aaad4f6aa2dbdb7091dd9c49fb/docker.go#L1245
func resolveHostIP(daemonHost string, ipamConfig network.IPAM) (string, error) {
	hostURL, err := url.Parse(daemonHost)
	if err != nil {
		return "", err
	}

	switch hostURL.Scheme {
	case "http", "https", "tcp":
		return hostURL.Hostname(), nil
	case "unix", "npipe":
		if _, err := os.Stat("/.dockerenv"); err == nil { // inside a container
			// Use "host.docker.internal" if enabled.
			addr, err := net.ResolveIPAddr("", "host.docker.internal")
			if err == nil {
				return addr.String(), nil
			}

			// Use Gateway IP of bridge network.
			// This doesn't work in Docker Desktop for Mac.
			for _, cfg := range ipamConfig.Config {
				if cfg.Gateway != "" {
					return cfg.Gateway, nil
				}
			}
		}
		return "localhost", nil
	default:
		return "", fmt.Errorf("unknown scheme found in daemon host: %s", hostURL.String())
	}
}

func (d *dockerBackend) BuildImage(ctx context.Context, buildContext io.Reader, buildOptions types.ImageBuildOptions, force bool, buildOut io.Writer) (err error) {
	image := buildOptions.Tags[0]

	if !force {
		// check if the same image already exists
		summaries, err := d.cli.ImageList(ctx, types.ImageListOptions{
			All: true,
		})
		if err != nil {
			return err
		}
		for _, s := range summaries {
			for _, t := range s.RepoTags {
				if t == image {
					return nil
				}
			}
		}
	}

	resp, err := d.cli.ImageBuild(ctx, buildContext, buildOptions)
	if err != nil {
		return err
	}
	defer func() {
		err = multierr.Append(err, resp.Body.Close())
	}()

	return handleJSONMessageStream(buildOut, resp.Body)
}

func createArchive(ctxDir, dockerfilePath string) (io.ReadCloser, string, error) {
	absContextDir, relDockerfile, err := build.GetContextFromLocalDir(ctxDir, dockerfilePath)
	if err != nil {
		return nil, "", err
	}

	excludes, err := build.ReadDockerignore(absContextDir)
	if err != nil {
		return nil, "", err
	}

	// We have to include docker-ignored Dockerfile and .dockerignore for build.
	// When `ADD` or `COPY` executes, daemon excludes these docker-ignored files.
	excludes = build.TrimBuildFilesFromExcludes(excludes, relDockerfile, false)

	err = build.ValidateContextDirectory(absContextDir, excludes)
	if err != nil {
		return nil, "", err
	}

	tarball, err := archive.TarWithOptions(absContextDir, &archive.TarOptions{
		ExcludePatterns: excludes,
		Compression:     archive.Uncompressed,
		NoLchown:        true,
	})
	if err != nil {
		return nil, "", err
	}
	return tarball, relDockerfile, nil
}

var _ Backend = (*dockerBackend)(nil)

type dockerNamespace struct {
	*dockerBackend
	namespace string
	network   *types.NetworkResource
	hostIP    string
	labels    map[string]string

	m          sync.RWMutex
	terminate  []func(ctx context.Context) error
	containers map[string]*containerInfo
}

type containerInfo struct {
	containerID string
	container   *container.Config
	host        *container.HostConfig
	network     *network.NetworkingConfig
	ports       Ports
	wait        *wait.Waiter
	running     bool
}

func (d *dockerNamespace) Namespace() string {
	return d.namespace
}

func (d *dockerNamespace) Network() *types.NetworkResource {
	return d.network
}

func (d *dockerNamespace) CreateContainer(
	ctx context.Context, name string, container *container.Config,
	host *container.HostConfig, networking *network.NetworkingConfig, configConsistency bool,
	wait *wait.Waiter, pullOptions *types.ImagePullOptions, pullOut io.Writer,
) (string, error) {
	var err error

	// merge labels
	if container.Labels == nil {
		container.Labels = d.labels
	} else {
		for k, v := range d.labels {
			container.Labels[k] = v
		}
	}

	d.m.Lock()
	defer d.m.Unlock()

	fullName := "/" + name
	c, ok := d.containers[name]
	if ok {
		if configConsistency {
			err = checkConfigConsistency(
				container, c.container,
				host, c.host,
				networking.EndpointsConfig, c.network.EndpointsConfig,
			)
		}
		return c.containerID, err
	}

	containers, err := d.cli.ContainerList(ctx, types.ContainerListOptions{
		All: true, // contains exiting/paused images
		Filters: filters.NewArgs(
			filters.Arg("name", name),
		),
	})
	if err != nil {
		return "", err
	}
	var existing *types.Container
LOOP:
	for _, c := range containers {
		for _, n := range c.Names {
			if fullName == n {
				if c.Image != container.Image {
					return "", errors.New(containerNameConflict(name, container.Image, c.Image))
				}
				existing = &c
				break LOOP
			}
		}
	}

	// try pull image when container not exists
	if existing == nil && pullOptions != nil {
		err := d.pull(ctx, container.Image, *pullOptions, pullOut)
		if err != nil {
			return "", err
		}
	}

	var containerID string
	var connected bool
	if existing != nil {
		if d.policy == ResourcePolicyError {
			return "", fmt.Errorf("dockerNamespace: container %q(%s) already exists", name, container.Image)
		}
		info, err := d.cli.ContainerInspect(ctx, existing.ID)
		if err != nil {
			return "", err
		}
		if configConsistency {
			err = checkConfigConsistency(
				container, info.Config,
				host, info.HostConfig,
				networking.EndpointsConfig, info.NetworkSettings.Networks,
			)
			if err != nil {
				return "", err
			}
		}

		switch existing.State {
		case "running", "created":
			containerID = existing.ID

			var found bool
			for _, setting := range existing.NetworkSettings.Networks {
				if setting.NetworkID == d.network.ID {
					found = true
					break
				}
			}
			if !found {
				err := d.cli.NetworkConnect(ctx, d.network.ID, containerID, &network.EndpointSettings{
					NetworkID: d.network.ID,
					Aliases: []string{
						strings.TrimPrefix(name, d.namespace),
					},
				})
				if err != nil {
					return "", err
				}
				connected = true
			}

		case "paused":
			// MEMO: bound port is still existing
			return "", fmt.Errorf("dockerNamespace: cannot start %q, unpause is not supported", name)

		default:
			return "", fmt.Errorf("dockerNamespace: cannot start %q, unexpected container state %q", name, existing.State)
		}
	} else {
		created, err := d.cli.ContainerCreate(ctx, container, host, networking, nil, name)
		if err != nil {
			return "", err
		}
		containerID = created.ID
	}

	if (existing == nil && d.policy != ResourcePolicyReusable) || d.policy == ResourcePolicyTakeOver {
		d.terminate = append(d.terminate, func(ctx context.Context) error {
			return d.cli.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{
				Force:         true,
				RemoveVolumes: true,
			})
		})
	} else if connected {
		d.terminate = append(d.terminate, func(ctx context.Context) error {
			return d.cli.NetworkDisconnect(ctx, d.network.ID, containerID, true)
		})
	}
	d.containers[name] = &containerInfo{
		containerID: containerID,
		container:   container,
		host:        host,
		network:     networking,
		wait:        wait,
		running:     false,
	}
	return containerID, nil
}

func (d *dockerNamespace) pull(ctx context.Context, image string, pullOptions types.ImagePullOptions, out io.Writer) (err error) {
	rc, err := d.cli.ImagePull(ctx, image, pullOptions)
	if err != nil {
		return err
	}
	defer func() {
		err = multierr.Append(err, rc.Close())
	}()
	return handleJSONMessageStream(out, rc)
}

func containerNameConflict(name, wantImage, gotImage string) string {
	return fmt.Sprintf("container name %q already exists but image is not %q(%q)", name, wantImage, gotImage)
}

func (d *dockerNamespace) containerPortMap(ctx context.Context, containerID string, requiredPorts nat.PortSet) (nat.PortMap, error) {
	if len(requiredPorts) == 0 {
		return nat.PortMap{}, nil
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	b := backoff.Constant(
		backoff.WithInterval(200*time.Millisecond),
		backoff.WithMaxRetries(0),
	).Start(ctx)
retry:
	for backoff.Continue(b) {
		i, err := d.cli.ContainerInspect(ctx, containerID)
		if err != nil {
			return nil, err
		}

		for p, bindings := range i.NetworkSettings.Ports {
			if _, ok := requiredPorts[p]; !ok {
				continue
			} else if len(bindings) == 0 {
				// endpoint not bound yet
				continue retry
			}
			// Use this port. Replace host ip.
			for i := range bindings {
				bindings[i].HostIP = d.hostIP
			}
		}
		return i.NetworkSettings.Ports, nil
	}
	return nil, errors.New("cannot get endpoints")
}

func (d *dockerNamespace) StartContainer(ctx context.Context, name string) (Ports, error) {
	d.m.RLock()
	c, ok := d.containers[name]
	d.m.RUnlock()
	if !ok {
		return nil, fmt.Errorf("dockerNamespace: container %q not found", name)
	} else if c.running {
		return c.ports, nil
	}

	err := d.cli.ContainerStart(ctx, c.containerID, types.ContainerStartOptions{})
	if err != nil {
		return nil, err
	}

	portMap, err := d.containerPortMap(ctx, c.containerID, c.container.ExposedPorts)
	if err != nil {
		return nil, err
	}

	if c.wait != nil {
		err = c.wait.Wait(ctx, &fetcher{
			cli:         d.cli,
			containerID: c.containerID,
			ports:       portMap,
		})
		if err != nil {
			return nil, err
		}
	}

	c.running = true
	c.ports = Ports(portMap)
	return c.ports, nil
}

func (d *dockerNamespace) Release(ctx context.Context) error {
	d.m.Lock()
	defer d.m.Unlock()

	var err error
	last := len(d.terminate) - 1
	for i := range d.terminate {
		err = multierr.Append(err, d.terminate[last-i](ctx))
	}
	return err
}

var _ Namespace = (*dockerNamespace)(nil)

// write stream message line by line
func handleJSONMessageStream(dst io.Writer, src io.Reader) error {
	dec := json.NewDecoder(src)
	buf := &bytes.Buffer{}

	for {
		var msg jsonmessage.JSONMessage
		err := dec.Decode(&msg)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		err = msg.Display(buf, false)
		if err != nil {
			return err
		}
		_, err = dst.Write(buf.Bytes())
		if err != nil {
			return err
		}
		buf.Reset()
	}
	return nil
}

// wait.Fetcher implementation
type fetcher struct {
	cli         *client.Client
	containerID string
	ports       nat.PortMap
}

func (f *fetcher) ContainerID() string {
	return f.containerID
}

func (f *fetcher) Status(ctx context.Context) (*types.ContainerState, error) {
	i, err := f.cli.ContainerInspect(ctx, f.containerID)
	if err != nil {
		return nil, err
	}
	return i.State, nil
}

func (f *fetcher) Ports() nat.PortMap {
	return f.ports
}

func (f *fetcher) Log(ctx context.Context) (io.ReadCloser, error) {
	return f.cli.ContainerLogs(ctx, f.containerID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
}

func (f *fetcher) Exec(ctx context.Context, cmd ...string) ([]byte, error) {
	r, err := f.cli.ContainerExecCreate(ctx, f.containerID, types.ExecConfig{
		AttachStderr: true,
		AttachStdout: true,
		Cmd:          cmd,
	})
	if err != nil {
		return nil, err
	}

	hijackedResp, err := f.cli.ContainerExecAttach(ctx, r.ID, types.ExecStartCheck{})
	if err != nil {
		return nil, err
	}
	defer hijackedResp.Close()

	buf := bytes.NewBuffer(nil)
	_, err = stdcopy.StdCopy(buf, buf, hijackedResp.Reader)
	if err != nil {
		return nil, err
	}

	info, err := f.cli.ContainerExecInspect(ctx, r.ID)
	if err != nil {
		return nil, err
	}
	if info.ExitCode != 0 {
		return nil, &ExitError{
			ExitCode: info.ExitCode,
		}
	}
	return buf.Bytes(), nil
}

var _ wait.Fetcher = (*fetcher)(nil)
