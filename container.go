package confort

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/lestrrat-go/option"
)

type Group struct {
	cli       *client.Client
	namespace string
	network   *types.NetworkResource

	m          sync.Mutex
	terminate  []TerminateFunc
	containers map[string]*containerInfo
}

type Container struct {
	Image        string
	Env          map[string]string
	Cmd          []string
	Entrypoint   []string
	ExposedPorts []string
	Waiter       *Waiter
}

type containerInfo struct {
	name      string
	c         *Container
	opts      []RunOption
	endpoints map[nat.Port]string
	started   bool
}

type TerminateFunc func()

type (
	GroupOption interface {
		option.Interface
		group()
	}
	identOptionNamespace  struct{}
	identOptionNetwork    struct{}
	identOptionClientOpts struct{}
	groupOption           struct{ option.Interface }
)

func (groupOption) group() {}

func WithNamespace(s string) GroupOption {
	return groupOption{
		Interface: option.New(identOptionNamespace{}, s),
	}
}

func WithNetwork(s string) GroupOption {
	return groupOption{
		Interface: option.New(identOptionNetwork{}, s),
	}
}

func WithClientOpts(opts ...client.Opt) GroupOption {
	return groupOption{
		Interface: option.New(identOptionClientOpts{}, opts),
	}
}

func NewGroup(ctx context.Context, tb testing.TB, opts ...GroupOption) (*Group, TerminateFunc) {
	tb.Helper()

	namespace := os.Getenv("CFT_NAMESPACE")
	networkName := os.Getenv("CFT_NETWORK")
	clientOpts := []client.Opt{
		client.FromEnv,
	}

	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionNamespace{}:
			namespace = opt.Value().(string)
		case identOptionNetwork{}:
			networkName = opt.Value().(string)
		case identOptionClientOpts{}:
			clientOpts = opt.Value().([]client.Opt)
		}
	}

	if namespace != "" && networkName != "" {
		networkName = namespace + "-" + networkName
	}

	cli, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		tb.Fatal(err)
	}
	cli.NegotiateAPIVersion(ctx)

	var nw *types.NetworkResource
	if networkName != "" {
		// create network if not exists
		list, err := cli.NetworkList(ctx, types.NetworkListOptions{})
		if err != nil {
			tb.Fatal(err)
		}

		var found bool
		for _, n := range list {
			if n.Name == networkName {
				found = true
				break
			}
		}
		if !found {
			created, err := cli.NetworkCreate(ctx, networkName, types.NetworkCreate{
				Driver: "bridge",
			})
			if err != nil {
				tb.Fatal(err)
			}
			n, err := cli.NetworkInspect(ctx, created.ID, types.NetworkInspectOptions{
				Verbose: true,
			})
			if err != nil {
				tb.Fatal(err)
			}
			nw = &n
		}
	}

	g := &Group{
		cli:        cli,
		namespace:  namespace,
		network:    nw,
		containers: map[string]*containerInfo{},
	}
	term := func() {
		ctx := context.Background()

		last := len(g.terminate) - 1
		for i := range g.terminate {
			g.terminate[last-i]()
		}

		if g.network != nil {
			err := g.cli.NetworkRemove(ctx, g.network.ID)
			if err != nil {
				tb.Logf("error occurred on remove network %q: %s", networkName, err)
			}
		}
		err = g.cli.Close()
		if err != nil {
			tb.Log(err)
		}
	}

	return g, term
}

type (
	RunOption interface {
		option.Interface
		run()
		build()
	}
	identOptionContainerConfig  struct{}
	identOptionHostConfig       struct{}
	identOptionNetworkingConfig struct{}
	identOptionPullOptions      struct{}
	runOption                   struct{ option.Interface }
)

func (runOption) run()   {}
func (runOption) build() {}

func WithContainerConfig(f func(config *container.Config)) RunOption {
	return runOption{
		Interface: option.New(identOptionContainerConfig{}, f),
	}
}

func WithHostConfig(f func(config *container.HostConfig)) RunOption {
	return runOption{
		Interface: option.New(identOptionHostConfig{}, f),
	}
}

func WithNetworkingConfig(f func(config *network.NetworkingConfig)) RunOption {
	return runOption{
		Interface: option.New(identOptionNetworkingConfig{}, f),
	}
}

func WithPullOptions(opts types.ImagePullOptions) RunOption {
	return runOption{
		Interface: option.New(identOptionPullOptions{}, opts),
	}
}

// Run starts container with given parameters.
// If container already exists and not started, it starts.
// It reuses already started container and its endpoint information.
func (g *Group) Run(ctx context.Context, tb testing.TB, name string, c *Container, opts ...RunOption) map[nat.Port]string {
	tb.Helper()

	if g.namespace != "" {
		name = g.namespace + "-" + name
	}

	g.m.Lock()
	defer g.m.Unlock()

	// find existing container in Group
	info, ok := g.containers[name]
	if ok && info.started {
		return info.endpoints
	}

	return g.run(ctx, tb, name, c, info, opts...)
}

func (g *Group) run(ctx context.Context, tb testing.TB, name string, c *Container, info *containerInfo, opts ...RunOption) map[nat.Port]string {
	// find existing container
	var containerID string
	existing, err := g.existingContainer(ctx, c.Image, name)
	if err != nil {
		tb.Fatal(err)
	} else if existing != nil {
		switch existing.State {
		case "running":
			endpoints := map[nat.Port]string{}
			for _, p := range existing.Ports {
				if p.IP != "" {
					np, err := nat.NewPort(p.Type, fmt.Sprint(p.PrivatePort))
					if err != nil {
						tb.Fatal(err)
					}
					endpoints[np] = p.IP + ":" + fmt.Sprint(p.PublicPort)
				}
			}
			if info != nil {
				info.endpoints = endpoints
			} else {
				g.containers[name] = &containerInfo{
					name:      name,
					c:         c,
					opts:      opts,
					endpoints: endpoints,
					started:   true,
				}
			}
			return endpoints

		case "created": // LazyRun
			containerID = existing.ID

		case "paused":
			// MEMO: bound port is still existing
			tb.Fatalf("cannot start %q: unpause is not supported", name)

		default:
			tb.Fatalf("cannot start %q: unexpected container state %s", name, existing.State)
		}
	}

	// create container if not exists
	if containerID == "" {
		containerID, err = g.createContainer(ctx, name, c, opts...)
		if err != nil {
			tb.Fatal(err)
		}
	}

	err = g.cli.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
	if err != nil {
		tb.Fatal(err)
	}

	var success bool
	term := func() {
		err := g.cli.ContainerStop(context.Background(), containerID, nil)
		if err != nil {
			tb.Log(err)
		}
	}
	defer func() {
		if !success {
			term()
		}
	}()

	i, err := g.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		tb.Fatal(err)
	}
	endpoints := map[nat.Port]string{}
	for p, bindings := range i.NetworkSettings.Ports {
		if len(bindings) == 0 {
			continue
		}
		b := bindings[0]
		if b.HostPort == "" {
			continue
		}
		endpoints[p] = b.HostIP + ":" + b.HostPort
	}

	if info != nil {
		info.endpoints = endpoints
	} else {
		g.containers[name] = &containerInfo{
			name:      name,
			c:         c,
			opts:      opts,
			endpoints: endpoints,
			started:   true,
		}
	}

	if c.Waiter != nil {
		err = c.Waiter.Wait(ctx, &fetcher{
			cli:         g.cli,
			containerID: containerID,
			endpoints:   endpoints,
		})
		if err != nil {
			tb.Fatal(err)
		}
	}

	g.terminate = append(g.terminate, term)
	success = true
	return endpoints
}

func (g *Group) existingContainer(ctx context.Context, image, name string) (*types.Container, error) {
	name = "/" + name

	containers, err := g.cli.ContainerList(ctx, types.ContainerListOptions{
		All: true, // contains exiting/paused images
	})
	if err != nil {
		return nil, err
	}
	for _, c := range containers {
		for _, n := range c.Names {
			if name == n {
				if c.Image == image {
					return &c, nil
				}
				return nil, fmt.Errorf("container name %q already exists but image is not %q(%q)", name, image, c.Image)
			}
		}
	}
	return nil, nil
}

func (g *Group) createContainer(ctx context.Context, name string, c *Container, opts ...RunOption) (string, error) {
	var modifyContainer func(config *container.Config)
	var modifyHost func(config *container.HostConfig)
	var modifyNetworking func(config *network.NetworkingConfig)
	var pullOptions *types.ImagePullOptions

	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionContainerConfig{}:
			modifyContainer = opt.Value().(func(config *container.Config))
		case identOptionHostConfig{}:
			modifyHost = opt.Value().(func(config *container.HostConfig))
		case identOptionNetworkingConfig{}:
			modifyNetworking = opt.Value().(func(config *network.NetworkingConfig))
		case identOptionPullOptions{}:
			o := opt.Value().(types.ImagePullOptions)
			pullOptions = &o
		}
	}

	if pullOptions != nil {
		rc, err := g.cli.ImagePull(ctx, c.Image, *pullOptions)
		if err != nil {
			return "", fmt.Errorf("pull: %s", err)
		}
		_, err = io.ReadAll(rc)
		if err != nil {
			return "", fmt.Errorf("pull: %s", err)
		}
		err = rc.Close()
		if err != nil {
			return "", fmt.Errorf("pull: %s", err)
		}
	}

	portSet, portBindings, err := nat.ParsePortSpecs(c.ExposedPorts)
	if err != nil {
		return "", err
	}

	env := make([]string, 0, len(c.Env))
	for envKey, envVar := range c.Env {
		env = append(env, envKey+"="+envVar)
	}

	cc := &container.Config{
		Image:        c.Image,
		ExposedPorts: portSet,
		Env:          env,
		Cmd:          c.Cmd,
		Entrypoint:   c.Entrypoint,
	}
	if modifyContainer != nil {
		modifyContainer(cc)
	}
	hc := &container.HostConfig{
		PortBindings: portBindings,
		AutoRemove:   true,
	}
	if modifyHost != nil {
		modifyHost(hc)
	}
	nc := &network.NetworkingConfig{}
	if g.network != nil {
		nc.EndpointsConfig = map[string]*network.EndpointSettings{
			g.network.ID: {
				NetworkID: g.network.ID,
			},
		}
	}
	if modifyNetworking != nil {
		modifyNetworking(nc)
	}

	created, err := g.cli.ContainerCreate(ctx, cc, hc, nc, nil, name)
	return created.ID, err
}

// LazyRun creates container but do not start.
// If container is already created/started by other test or process, LazyRun just
// store container info. It makes no error.
//
// We can start created container by Group.Run or Group.Use. The latter is an easier
// way because it only requires container name.
func (g *Group) LazyRun(ctx context.Context, tb testing.TB, name string, c *Container, opts ...RunOption) {
	tb.Helper()

	if g.namespace != "" {
		name = g.namespace + "-" + name
	}

	g.m.Lock()
	defer g.m.Unlock()

	// find existing container in Group
	_, foundContainerInfo := g.containers[name]
	if foundContainerInfo {
		return
	}

	// find existing container
	existing, err := g.existingContainer(ctx, c.Image, name)
	if err != nil {
		tb.Fatal(err)
	}
	if existing == nil {
		// create if not exists
		_, err = g.createContainer(ctx, name, c, opts...)
		if err != nil {
			tb.Fatal(err)
		}
	}

	g.containers[name] = &containerInfo{
		name:      name,
		c:         c,
		opts:      opts,
		endpoints: nil,
		started:   false,
	}
}

// Use starts container created by LazyRun and returns endpoint info.
// If the container is already started by other test or process, Use reuse it.
func (g *Group) Use(ctx context.Context, tb testing.TB, name string) map[nat.Port]string {
	tb.Helper()

	if g.namespace != "" {
		name = g.namespace + "-" + name
	}

	g.m.Lock()
	defer g.m.Unlock()

	// find LazyRun container
	info, ok := g.containers[name]
	if !ok {
		tb.Fatalf("container %q not found", name)
	} else if info.started {
		return info.endpoints
	}

	return g.run(ctx, tb, name, info.c, info, info.opts...)
}
