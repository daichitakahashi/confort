package confort

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/cli/cli/command/image/build"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/jsonmessage"
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
			wait *Waiter, pullOptions *types.ImagePullOptions, pullOut io.Writer) (string, error)
		StartContainer(ctx context.Context, name string) (Ports, error)
		Release(ctx context.Context) error
	}
)

type Ports map[nat.Port][]string

func fromPortMap(ports nat.PortMap) Ports {
	p := make(Ports, len(ports))
	for port, bindings := range ports {
		endpoints := make([]string, len(bindings))
		for i, b := range bindings {
			endpoints[i] = b.HostIP + ":" + b.HostPort // TODO: specify host ip ???
		}
		p[port] = endpoints
	}
	return p
}

func (p Ports) Binding(port nat.Port) (string, bool) {
	bindings, ok := p[port]
	if !ok || len(bindings) == 0 {
		return "", false
	}
	return bindings[0], true
}

type ResourcePolicy string

const (
	ResourcePolicyError    ResourcePolicy = "error"
	ResourcePolicyReuse    ResourcePolicy = "reuse"
	ResourcePolicyTakeOver ResourcePolicy = "takeover"
)

func (p ResourcePolicy) Equals(s string) bool {
	return strings.EqualFold(string(p), s)
}

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

	var term []func(context.Context) error
	if nwCreated || d.policy == ResourcePolicyTakeOver {
		term = append(term, func(ctx context.Context) error {
			return d.cli.NetworkRemove(ctx, nw.ID)
		})
	}

	return &dockerNamespace{
		dockerBackend: d,
		namespace:     namespace,
		network:       nw,
		labels:        d.labels,
		terminate:     term,
		containers:    map[string]*containerInfo{},
	}, nil
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
	wait        *Waiter
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
	wait *Waiter, pullOptions *types.ImagePullOptions, pullOut io.Writer,
) (string, error) {
	var err error

	// inject labels
	container.Labels = d.labels

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

	if pullOptions != nil {
		err := d.pull(ctx, container.Image, *pullOptions, pullOut)
		if err != nil {
			return "", err
		}
	}

	containers, err := d.cli.ContainerList(ctx, types.ContainerListOptions{
		All: true, // contains exiting/paused images
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

	if existing == nil || d.policy == ResourcePolicyTakeOver {
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
		return nil, errors.New(containerNotFound(name))
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

	p := fromPortMap(portMap)
	if c.wait != nil {
		err = c.wait.Wait(ctx, &fetcher{
			cli:         d.cli,
			containerID: c.containerID,
			ports:       p,
		})
		if err != nil {
			return nil, err
		}
	}

	c.running = true
	c.ports = p
	return p, nil
}

func containerNotFound(name string) string {
	return fmt.Sprintf("dockerBackend: container %q not found", name)
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
