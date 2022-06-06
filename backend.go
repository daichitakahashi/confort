package confort

import (
	"context"
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
	"github.com/docker/go-connections/nat"
	"github.com/goccy/go-reflect"
	"github.com/google/go-cmp/cmp"
	"github.com/lestrrat-go/backoff/v2"
	"go.uber.org/multierr"
)

type (
	Backend interface {
		Namespace(ctx context.Context, namespace string) (Namespace, error)
		BuildImage(ctx context.Context, contextDir string, buildOptions types.ImageBuildOptions, force bool) (io.ReadCloser, error)
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
				return nil, fmt.Errorf("dockerBackend: network %q already exists", networkName)
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
		containers:    map[string]*containerInfo{},
	}, nil
}

func (d *dockerBackend) BuildImage(ctx context.Context, contextDir string, buildOptions types.ImageBuildOptions, force bool) (io.ReadCloser, error) {
	if len(buildOptions.Tags) == 0 {
		return nil, errors.New("tag not specified")
	}
	image := buildOptions.Tags[0]

	err := d.buildMu.Lock(ctx, image)
	if err != nil {
		return nil, err
	}
	defer d.buildMu.Unlock(image)

	if !force {
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

	m          sync.RWMutex
	acquireMu  *keyedLock
	terminate  []func(ctx context.Context) error
	containers map[string]*containerInfo
}

type containerInfo struct {
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

	name = d.namespace + name
	fullName := "/" + name
	c, ok := d.containers[name]
	if ok {
		err := checkConfigConsistency(
			container, c.container,
			host, c.host,
			networking.EndpointsConfig, c.network.EndpointsConfig,
		)
		return c.containerID, err
	}

	if pullOptions != nil {
		err := d.pull(ctx, container.Image, *pullOptions)
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
		err = checkConfigConsistency(
			container, info.Config,
			host, info.HostConfig,
			networking.EndpointsConfig, info.NetworkSettings.Networks,
		)
		if err != nil {
			return "", err
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

func containerNameConflict(name, wantImage, gotImage string) string {
	return fmt.Sprintf("container name %q already exists but image is not %q(%q)", name, wantImage, gotImage)
}

// TODO: make it optional
func checkConfigConsistency(
	container1, container2 *container.Config,
	host1, host2 *container.HostConfig,
	network1, network2 map[string]*network.EndpointSettings,
) (err error) {
	if err = checkContainerConfigConsistency(container2, container1); err != nil {
		return fmt.Errorf("inconsistent container config\n %w", err)
	}
	if err = checkHostConfigConsistency(host2, host1); err != nil {
		return fmt.Errorf("inconsistent host config\n%w", err)
	}
	if err = checkEndpointSettingsConsistency(network2, network1); err != nil {
		return fmt.Errorf("inconsistent network config\n%w", err)
	}
	return nil
}

func checkContainerConfigConsistency(expected, target *container.Config) (err error) {
	if err = stringSubset("Hostname", expected.Hostname, target.Hostname); err != nil {
		return err
	}
	if err = stringSubset("Domainname", expected.Domainname, target.Domainname); err != nil {
		return err
	}
	if err = stringSubset("User", expected.User, target.User); err != nil {
		return err
	}
	// AttachStdin
	// AttachStdout
	// AttachStderr
	if err = mapSubset("ExposedPorts", target.ExposedPorts, target.ExposedPorts); err != nil {
		return err
	}
	// Tty
	// OpenStdin
	// StdinOnce
	if err = sliceSubset("Env", expected.Env, target.Env); err != nil {
		return err
	}
	if err = sequentialSubset("Cmd", expected.Cmd, target.Cmd); err != nil {
		return err
	}
	if err = pointerSubset("Healthcheck", expected.Healthcheck, target.Healthcheck); err != nil {
		return err
	}
	if expected.ArgsEscaped != target.ArgsEscaped {
		return diffError("ArgsEscaped", expected.ArgsEscaped, target.ArgsEscaped)
	}
	if expected.Image != target.Image {
		return diffError("Image", expected.Image, target.Image)
	}
	if err = mapSubset("Volumes", expected.Volumes, target.Volumes); err != nil {
		return err
	}
	if expected.WorkingDir != target.WorkingDir {
		return diffError("WorkingDir", expected.WorkingDir, target.WorkingDir)
	}
	if err = sequentialSubset("Entrypoint", expected.Entrypoint, target.Entrypoint); err != nil {
		return err
	}
	// NetworkDisabled
	if err = stringSubset("MacAddress", expected.MacAddress, target.MacAddress); err != nil {
		return err
	}
	// OnBuild
	if err = mapSubset("Labels", expected.Labels, target.Labels); err != nil {
		return err
	}
	if err = stringSubset("StopSignal", expected.StopSignal, target.StopSignal); err != nil {
		return err
	}
	if err = pointerSubset("StopTimeout", expected.StopTimeout, target.StopTimeout); err != nil {
		return err
	}
	if err = sequentialSubset("Shell", expected.Shell, target.Shell); err != nil {
		return err
	}
	return nil
}

func checkHostConfigConsistency(expected, target *container.HostConfig) (err error) {
	// TODO: implement
	/*
		Binds           []string      // List of volume bindings for this container
			ContainerIDFile string        // File (path) where the containerId is written
			LogConfig       LogConfig     // Configuration of the logs for this container
			NetworkMode     NetworkMode   // Network mode to use for the container
			PortBindings    nat.PortMap   // Port mapping between the exposed port (container) and the host
			RestartPolicy   RestartPolicy // Restart policy to be used for the container
			AutoRemove      bool          // Automatically remove container when it exits
			VolumeDriver    string        // Name of the volume driver used to mount volumes
			VolumesFrom     []string      // List of volumes to take from other container

			// Applicable to UNIX platforms
			CapAdd          strslice.StrSlice // List of kernel capabilities to add to the container
			CapDrop         strslice.StrSlice // List of kernel capabilities to remove from the container
			CgroupnsMode    CgroupnsMode      // Cgroup namespace mode to use for the container
			DNS             []string          `json:"Dns"`        // List of DNS server to lookup
			DNSOptions      []string          `json:"DnsOptions"` // List of DNSOption to look for
			DNSSearch       []string          `json:"DnsSearch"`  // List of DNSSearch to look for
			ExtraHosts      []string          // List of extra hosts
			GroupAdd        []string          // List of additional groups that the container process will run as
			IpcMode         IpcMode           // IPC namespace to use for the container
			Cgroup          CgroupSpec        // Cgroup to use for the container
			Links           []string          // List of links (in the name:alias form)
			OomScoreAdj     int               // Container preference for OOM-killing
			PidMode         PidMode           // PID namespace to use for the container
			Privileged      bool              // Is the container in privileged mode
			PublishAllPorts bool              // Should docker publish all exposed port for the container
			ReadonlyRootfs  bool              // Is the container root filesystem in read-only
			SecurityOpt     []string          // List of string values to customize labels for MLS systems, such as SELinux.
			StorageOpt      map[string]string `json:",omitempty"` // Storage driver options per container.
			Tmpfs           map[string]string `json:",omitempty"` // List of tmpfs (mounts) used for the container
			UTSMode         UTSMode           // UTS namespace to use for the container
			UsernsMode      UsernsMode        // The user namespace to use for the container
			ShmSize         int64             // Total shm memory usage
			Sysctls         map[string]string `json:",omitempty"` // List of Namespaced sysctls used for the container
			Runtime         string            `json:",omitempty"` // Runtime to use with this container

			// Applicable to Windows
			ConsoleSize [2]uint   // Initial console size (height,width)
			Isolation   Isolation // Isolation technology of the container (e.g. default, hyperv)

			// Contains container's resources (cgroups, ulimits)
			Resources

			// Mounts specs used by the container
			Mounts []mount.Mount `json:",omitempty"`

			// MaskedPaths is the list of paths to be masked inside the container (this overrides the default set of paths)
			MaskedPaths []string

			// ReadonlyPaths is the list of paths to be set as read-only inside the container (this overrides the default set of paths)
			ReadonlyPaths []string

			// Run a custom init inside the container, if null, use the daemon's configured settings
			Init *bool `json:",omitempty"`
	*/
	return nil
}

func checkEndpointSettingsConsistency(expected, target map[string]*network.EndpointSettings) (err error) {
	// TODO: implement
	/*
		// Configurations
		IPAMConfig *EndpointIPAMConfig
		Links      []string
		Aliases    []string
		// Operational data
		NetworkID           string
		EndpointID          string
		Gateway             string
		IPAddress           string
		IPPrefixLen         int
		IPv6Gateway         string
		GlobalIPv6Address   string
		GlobalIPv6PrefixLen int
		MacAddress          string
		DriverOpts          map[string]string
	*/
	return nil
}

func stringSubset(name, expected, target string) (err error) {
	if target == "" || target == expected {
		return nil
	}
	return diffError(name, expected, target)
}

func sliceSubset[T comparable](name string, expected, target []T) (err error) {
	if len(target) == 0 {
		return nil
	} else if len(expected) == 0 {
		return diffError(name, expected, target)
	}
	exp := make(map[T]bool)
	for _, t := range expected {
		exp[t] = true
	}
	for _, t := range target {
		if !exp[t] {
			return diffError(name, expected, target)
		}
	}
	return nil
}

func sequentialSubset[T comparable](name string, expected, target []T) (err error) {
	if len(target) == 0 {
		return nil
	} else if len(expected) == 0 || !reflect.DeepEqual(expected, target) {
		return diffError(name, expected, target)
	}
	return nil
}

func mapSubset[K, V comparable](name string, expected, target map[K]V) error {
	if len(target) == 0 {
		return nil
	} else if len(expected) == 0 {
		return diffError(name, expected, target)
	}
	for k, v := range target {
		e, ok := expected[k]
		if !ok || e != v {
			return diffError(name, expected, target)
		}
	}
	return nil
}

func pointerSubset[T any](name string, expected, target *T) (err error) {
	if target == nil {
		return nil
	} else if expected == nil || !reflect.DeepEqual(expected, target) {
		return diffError(name, expected, target)
	}
	return nil
}

func diffError(msg string, expected, target any) error {
	diff := cmp.Diff(expected, target)
	return fmt.Errorf("%s\n%s", msg, diff)
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
	name = d.namespace + name

	if exclusive {
		err = d.acquireMu.Lock(ctx, name)
	} else {
		err = d.acquireMu.RLock(ctx, name)
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		if err == nil {
			return
		}
		if exclusive {
			d.acquireMu.Unlock(name)
		} else {
			d.acquireMu.RUnlock(name)
		}
	}()

	d.m.RLock()
	c, ok := d.containers[name]
	d.m.RUnlock()
	if !ok {
		return nil, errors.New(containerNotFound(name))
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

func containerNotFound(name string) string {
	return fmt.Sprintf("dockerBackend: container %q not found", name)
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
