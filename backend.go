package confort

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/go-cmp/cmp"
	"github.com/lestrrat-go/backoff/v2"
	"go.uber.org/multierr"
)

type (
	Backend interface {
		Namespace(ctx context.Context, namespace string) (Namespace, error)
		BuildImage(ctx context.Context, contextDir string, buildOptions types.ImageBuildOptions) (io.ReadCloser, error)
	}

	Namespace interface {
		Namespace() string
		Network() *types.NetworkResource

		CreateContainer(ctx context.Context, name string, container *container.Config, host *container.HostConfig,
			network *network.NetworkingConfig, pullOptions *types.ImagePullOptions) (string, error)
		StartContainer(ctx context.Context, name string, exclusive bool) (nat.PortMap, error)
		ReleaseContainer(ctx context.Context, name string, exclusive bool) error
		Release(ctx context.Context) error
	}
)

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
	networkMu sync.Mutex
	buildMu   *keyedLock
	cli       *client.Client // inject
	policy    ResourcePolicy
}

func (d *dockerBackend) Namespace(ctx context.Context, namespace string) (Namespace, error) {
	d.networkMu.Lock()
	defer d.networkMu.Unlock()

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
				return nil, errors.New("confort: network %q already exists")
			}
			nw = &n
			break
		}
	}

	var nwCreated bool
	if nw == nil {
		resp, err := d.cli.NetworkCreate(ctx, networkName, types.NetworkCreate{
			Driver: "bridge",
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
		acquireMu:     newKeyedLock(),
		namespace:     namespace,
		network:       nw,
		terminate:     term,
		containers:    map[string]*containerInfo2{},
	}, nil
}

func (d *dockerBackend) BuildImage(ctx context.Context, contextDir string, buildOptions types.ImageBuildOptions) (io.ReadCloser, error) {
	if len(buildOptions.Tags) == 0 {
		return nil, errors.New("tag not specified")
	}
	image := buildOptions.Tags[0]

	err := d.buildMu.Lock(ctx, image)
	if err != nil {
		return nil, err
	}
	defer d.buildMu.Unlock(image)

	// check if the same image already exists
	summaries, err := d.cli.ImageList(ctx, types.ImageListOptions{
		All: true,
	})
	if err != nil {
		return nil, err
	}
	for _, s := range summaries {
		for _, t := range s.RepoTags {
			if t == image {
				return nil, nil
			}
		}
	}

	tarball, relDockerfile, err := createArchive(contextDir, buildOptions.Dockerfile)
	if err != nil {
		return nil, err
	}
	buildOptions.Dockerfile = relDockerfile

	resp, err := d.cli.ImageBuild(ctx, tarball, buildOptions)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

var _ Backend = (*dockerBackend)(nil)

type dockerNamespace struct {
	*dockerBackend
	m         sync.Mutex
	acquireMu *keyedLock

	namespace  string
	network    *types.NetworkResource
	terminate  []func(ctx context.Context) error
	containers map[string]*containerInfo2
}

type containerInfo2 struct {
	containerID string
	container   *container.Config
	host        *container.HostConfig
	network     *network.NetworkingConfig
	portMap     nat.PortMap
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
	host *container.HostConfig, networking *network.NetworkingConfig, pullOptions *types.ImagePullOptions,
) (string, error) {
	d.m.Lock()
	defer d.m.Unlock()

	if pullOptions != nil {
		err := d.pull(ctx, container.Image, *pullOptions)
		if err != nil {
			return "", err
		}
	}

	name = d.namespace + name
	fullName := "/" + name
	containers, err := d.cli.ContainerList(ctx, types.ContainerListOptions{
		All: true, // contains exiting/paused images
	})
	if err != nil {
		return "", err
	}
	var existing *types.Container
	for _, c := range containers {
		for _, n := range c.Names {
			if fullName == n {
				if c.Image != container.Image {
					return "", errors.New(containerNameConflict(name, container.Image, c.Image))
				}
				existing = &c
				break
			}
		}
	}

	var containerID string
	var connected bool
	if existing != nil {
		if d.policy == ResourcePolicyError {
			return "", fmt.Errorf("confort: container %q(%s) already exists", name, container.Image)
		}
		info, err := d.cli.ContainerInspect(nil, existing.ID)
		if err != nil {
			return "", fmt.Errorf("confort: %w", err)
		}
		err = checkConfigConsistency(
			container, info.Config,
			host, info.HostConfig,
			networking.EndpointsConfig, info.NetworkSettings.Networks,
		)
		if err != nil {
			return "", fmt.Errorf("confort: %s: %w", info.Name, err)
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
						strings.TrimPrefix(name, d.namespace+"-"),
					},
				})
				if err != nil {
					return "", fmt.Errorf("confort: %w", err)
				}
				connected = true
			}

		case "paused":
			// MEMO: bound port is still existing
			return "", fmt.Errorf("confort: cannot start %q, unpause is not supported", name)

		default:
			return "", fmt.Errorf("confort: cannot start %q, unexpected container state %s", name, existing.State)
		}
	} else {
		created, err := d.cli.ContainerCreate(ctx, container, host, networking, nil, d.namespace+name)
		if err != nil {
			return "", fmt.Errorf("confort: %w", err)
		}
		containerID = created.ID
	}

	if existing == nil || d.policy == ResourcePolicyTakeOver {
		d.terminate = append(d.terminate, func(ctx context.Context) error {
			return d.cli.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{
				RemoveVolumes: true,
			})
		})
	} else if connected {
		d.terminate = append(d.terminate, func(ctx context.Context) error {
			return d.cli.NetworkDisconnect(ctx, d.network.ID, containerID, true)
		})
	}
	d.containers[name] = &containerInfo2{
		containerID: containerID,
		container:   container,
		host:        host,
		network:     networking,
		running:     false,
	}
	return containerID, nil
}

func (d *dockerNamespace) pull(ctx context.Context, image string, pullOptions types.ImagePullOptions) (err error) {
	rc, err := d.cli.ImagePull(ctx, image, pullOptions)
	if err != nil {
		return err
	}
	defer func() {
		err = multierr.Append(err, rc.Close())
	}()
	_, err = io.ReadAll(rc)
	return err
}

func checkConfigConsistency(
	container1, container2 *container.Config,
	host1, host2 *container.HostConfig,
	network1, network2 map[string]*network.EndpointSettings,
) error {
	if diff := cmp.Diff(container1, container2); diff != "" {
		return fmt.Errorf("inconsistent container config\n%s", diff)
	}
	if diff := cmp.Diff(host1, host2); diff != "" {
		return fmt.Errorf("inconsistent host config\n%s", diff)
	}
	if diff := cmp.Diff(network1, network2); diff != "" {
		return fmt.Errorf("inconsistent network config\n%s", diff)
	}
	return nil
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

func (d *dockerNamespace) StartContainer(ctx context.Context, name string, exclusive bool) (portMap nat.PortMap, err error) {
	d.m.Lock()
	defer func() {
		d.m.Unlock()
		if exclusive {
			err = d.acquireMu.Lock(ctx, name)
		} else {
			err = d.acquireMu.RLock(ctx, name)
		}
	}()

	name = d.namespace + name
	c, ok := d.containers[name]
	if !ok {
		return nil, fmt.Errorf("confort: unknown container %q", name)
	} else if c.running {
		return c.portMap, nil
	}

	err = d.cli.ContainerStart(ctx, c.containerID, types.ContainerStartOptions{})
	if err != nil {
		return nil, err
	}

	portMap, err = d.containerPortMap(ctx, c.containerID, c.container.ExposedPorts)
	if err != nil {
		return nil, err
	}
	c.running = true
	c.portMap = portMap
	return portMap, nil
}

func (d *dockerNamespace) ReleaseContainer(_ context.Context, name string, exclusive bool) error {
	name = d.namespace + name
	if exclusive {
		d.acquireMu.Unlock(name)
	} else {
		d.acquireMu.RUnlock(name)
	}
	return nil
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
