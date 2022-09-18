package confort

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/daichitakahashi/confort/internal/beaconutil"
	"github.com/daichitakahashi/confort/internal/exclusion"
	"github.com/daichitakahashi/confort/proto/beacon"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/lestrrat-go/option"
)

type Confort struct {
	backend        Backend
	namespace      Namespace
	defaultTimeout time.Duration
	ex             exclusion.Control
}

type (
	NewOption interface {
		option.Interface
		new() NewOption
	}
	identOptionClientOptions struct{}
	identOptionNamespace     struct{}
	namespaceOption          struct {
		namespace string
		force     bool
	}
	identOptionDefaultTimeout struct{}
	identOptionResourcePolicy struct{}
	identOptionBeacon         struct{}
	identOptionTerminateFunc  struct{}
	newOption                 struct{ option.Interface }
)

func (o newOption) new() NewOption { return o }

// WithClientOptions sets options for Docker API client.
// Default option is client.FromEnv.
// For detail, see client.NewClientWithOpts.
func WithClientOptions(opts ...client.Opt) NewOption {
	return newOption{
		Interface: option.New(identOptionClientOptions{}, opts),
	}.new()
}

// WithNamespace specifies namespace of Confort.
// Default namespace is the value of the CFT_NAMESPACE environment variable.
// The "confort test" command has "-namespace" option that overrides the variable.
// If force is true, the value of the argument namespace takes precedence.
//
// If neither CFT_NAMESPACE nor WithNamespace is set, New fails.
func WithNamespace(namespace string, force bool) NewOption {
	return newOption{
		Interface: option.New(identOptionNamespace{}, namespaceOption{
			namespace: namespace,
			force:     force,
		}),
	}.new()
}

// WithDefaultTimeout sets the default timeout for each request to the Docker API and beacon server.
// The default value of the "default timeout" is 1 min.
// If default timeout is 0, Confort doesn't apply any timeout for ctx.
//
// If a timeout has already been set to ctx, the default timeout is not applied.
func WithDefaultTimeout(d time.Duration) NewOption {
	return newOption{
		Interface: option.New(identOptionDefaultTimeout{}, d),
	}.new()
}

// WithResourcePolicy overrides the policy for handling Docker resources that already exist,
// such as containers and networks.
// By default, ResourcePolicyReuse or the value of the CFT_RESOURCE_POLICY environment variable, if set, is used.
// The "confort test" command has "-policy" option that overrides the variable.
func WithResourcePolicy(s ResourcePolicy) NewOption {
	return newOption{
		Interface: option.New(identOptionResourcePolicy{}, s),
	}.new()
}

// WithBeacon configures Confort to integrate with a starting beacon server.
// The beacon server is started by the "confort" command.
// Use Connection object given from ConnectBeacon as the argument conn.
//
// For detail, see ConnectBeacon and "confort help".
func WithBeacon(conn *Connection) NewOption {
	return newOption{
		Interface: option.New(identOptionBeacon{}, conn),
	}.new()
}

// WithTerminateFunc extracts the function to release all resources created by Confort.
// With this option, the responsibility for termination is passed to the user.
func WithTerminateFunc(f *func()) NewOption {
	return newOption{
		Interface: option.New(identOptionTerminateFunc{}, f),
	}
}

func New(tb testing.TB, ctx context.Context, opts ...NewOption) *Confort {
	tb.Helper()

	var (
		skipDeletion bool
		beaconAddr   string
		ex           = exclusion.NewControl()

		clientOps = []client.Opt{
			client.FromEnv,
		}
		namespace     = os.Getenv(beaconutil.NamespaceEnv)
		timeout       = time.Minute
		policy        = ResourcePolicyReuse
		terminateFunc *func()
	)
	if s := os.Getenv(beaconutil.ResourcePolicyEnv); s != "" {
		policy = ResourcePolicy(s)
	}

	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionClientOptions{}:
			clientOps = opt.Value().([]client.Opt)
		case identOptionNamespace{}:
			o := opt.Value().(namespaceOption)
			if namespace == "" || o.force {
				namespace = o.namespace
			}
		case identOptionDefaultTimeout{}:
			timeout = opt.Value().(time.Duration)
		case identOptionResourcePolicy{}:
			policy = opt.Value().(ResourcePolicy)
		case identOptionBeacon{}:
			c := opt.Value().(*Connection)
			if c.Enabled() {
				ex = exclusion.NewBeaconControl(
					beacon.NewBeaconServiceClient(c.conn),
				)
				skipDeletion = true
				beaconAddr = c.addr
			}
		case identOptionTerminateFunc{}:
			terminateFunc = opt.Value().(*func())
		}
	}
	if namespace == "" {
		tb.Fatal("confort: empty namespace")
	}
	if !beaconutil.ValidResourcePolicy(string(policy)) {
		tb.Fatalf("confort: invalid resource policy %q", policy)
	}

	ctx, cancel := applyTimeout(ctx, timeout)
	defer cancel()
	cli, err := client.NewClientWithOpts(clientOps...)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
	cli.NegotiateAPIVersion(ctx)

	backend := &dockerBackend{
		cli:    cli,
		policy: policy,
		labels: map[string]string{
			beaconutil.LabelIdentifier: beaconutil.Identifier(beaconAddr),
		},
	}

	unlock, err := ex.LockForNamespace(ctx)
	if err != nil {
		tb.Fatal(err)
	}
	defer unlock()

	ns, err := backend.Namespace(ctx, namespace)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
	term := func() {
		tb.Helper()
		if skipDeletion {
			// if beacon is enabled, do not delete
			return
		}
		// release all resources
		err := ns.Release(context.Background())
		if err != nil {
			tb.Log(err)
		}
	}
	if terminateFunc != nil {
		*terminateFunc = term
	} else {
		tb.Cleanup(term)
	}

	return &Confort{
		backend:        backend,
		namespace:      ns,
		defaultTimeout: timeout,
		ex:             ex,
	}
}

func applyTimeout(ctx context.Context, defaultTimeout time.Duration) (context.Context, context.CancelFunc) {
	if defaultTimeout == 0 {
		return ctx, func() {}
	}
	_, ok := ctx.Deadline()
	if ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, defaultTimeout)
}

// Namespace returns namespace associated with cft.
func (cft *Confort) Namespace() string {
	return cft.namespace.Namespace()
}

type (
	BuildOption interface {
		option.Interface
		build() BuildOption
	}
	identOptionImageBuildOptions struct{}
	identOptionForceBuild        struct{}
	identOptionBuildOutput       struct{}
	buildOption                  struct{ option.Interface }
)

func (o buildOption) build() BuildOption { return o }

// WithImageBuildOptions modifies the configuration of build.
// The argument `option` already contains required values, according to Build.
func WithImageBuildOptions(f func(option *types.ImageBuildOptions)) BuildOption {
	return buildOption{
		Interface: option.New(identOptionImageBuildOptions{}, f),
	}.build()
}

// WithForceBuild forces to build an image even if it already exists.
func WithForceBuild() BuildOption {
	return buildOption{
		Interface: option.New(identOptionForceBuild{}, true),
	}.build()
}

// WithBuildOutput sets dst that the output during build will be written.
func WithBuildOutput(dst io.Writer) BuildOption {
	return buildOption{
		Interface: option.New(identOptionBuildOutput{}, dst),
	}.build()
}

type BuildParams struct {
	Image      string
	Dockerfile string
	ContextDir string
	BuildArgs  map[string]*string
	Platform   string
}

// Build creates new image from given Dockerfile and context directory.
//
// When same name image already exists, it doesn't perform building.
// WithForceBuild enables us to build image on every call of Build.
func (cft *Confort) Build(tb testing.TB, ctx context.Context, b *BuildParams, opts ...BuildOption) {
	tb.Helper()

	buildOut := io.Discard

	ctx, cancel := applyTimeout(ctx, cft.defaultTimeout)
	defer cancel()

	var modifyBuildOptions func(option *types.ImageBuildOptions)
	var force bool
	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionImageBuildOptions{}:
			modifyBuildOptions = opt.Value().(func(option *types.ImageBuildOptions))
		case identOptionForceBuild{}:
			force = opt.Value().(bool)
		case identOptionBuildOutput{}:
			out := opt.Value().(io.Writer)
			if out != nil {
				buildOut = out
			}
		}
	}

	tarball, relDockerfile, err := createArchive(b.ContextDir, b.Dockerfile)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, tarball)
	}()

	buildOption := types.ImageBuildOptions{
		Tags:           []string{b.Image},
		SuppressOutput: buildOut == io.Discard,
		Remove:         true,
		PullParent:     true,
		Dockerfile:     relDockerfile,
		BuildArgs:      b.BuildArgs,
		Target:         "",
		SessionID:      "",
		Platform:       b.Platform,
	}
	if modifyBuildOptions != nil {
		modifyBuildOptions(&buildOption)
	}

	if len(buildOption.Tags) == 0 {
		tb.Fatal("image tag not specified")
	}
	unlock, err := cft.ex.LockForBuild(ctx, buildOption.Tags[0])
	if err != nil {
		tb.Fatal(err)
	}
	defer unlock()

	err = cft.backend.BuildImage(ctx, tarball, buildOption, force, buildOut)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
}

type ContainerParams struct {
	Name         string
	Image        string
	Env          map[string]string
	Cmd          []string
	Entrypoint   []string
	ExposedPorts []string
	Waiter       *Waiter
}

func (cft *Confort) createContainer(ctx context.Context, name, alias string, c *ContainerParams, opts ...RunOption) (string, error) {
	var modifyContainer func(config *container.Config)
	var modifyHost func(config *container.HostConfig)
	var modifyNetworking func(config *network.NetworkingConfig)
	checkConsistency := true
	var pullOpts *types.ImagePullOptions
	pullOut := io.Discard

	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionContainerConfig{}:
			modifyContainer = opt.Value().(func(config *container.Config))
		case identOptionHostConfig{}:
			modifyHost = opt.Value().(func(config *container.HostConfig))
		case identOptionNetworkingConfig{}:
			modifyNetworking = opt.Value().(func(config *network.NetworkingConfig))
		case identOptionConfigConsistency{}:
			checkConsistency = opt.Value().(bool)
		case identOptionPullOption{}:
			o := opt.Value().(pullOptions)
			pullOpts = o.pullOption
			if o.pullOut != nil {
				pullOut = o.pullOut
			}
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
	networkID := cft.namespace.Network().ID
	nc := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			networkID: {
				NetworkID: networkID,
				Aliases:   []string{alias},
			},
		},
	}
	if modifyNetworking != nil {
		modifyNetworking(nc)
	}

	return cft.namespace.CreateContainer(ctx, name, cc, hc, nc, checkConsistency, c.Waiter, pullOpts, pullOut)
}

type (
	RunOption interface {
		option.Interface
		run() RunOption
	}
	identOptionContainerConfig   struct{}
	identOptionHostConfig        struct{}
	identOptionNetworkingConfig  struct{}
	identOptionConfigConsistency struct{}
	identOptionPullOption        struct{}
	pullOptions                  struct {
		pullOption *types.ImagePullOptions
		pullOut    io.Writer
	}
	runOption struct{ option.Interface }
)

func (o runOption) run() RunOption { return o }

// WithContainerConfig modifies the configuration of container.
// The argument `config` already contains required values to create container,
// apply your values with care.
func WithContainerConfig(f func(config *container.Config)) RunOption {
	return runOption{
		Interface: option.New(identOptionContainerConfig{}, f),
	}.run()
}

// WithHostConfig modifies the configuration of container from host side.
// The argument `config` already contains required values to create container,
// apply your values with care.
func WithHostConfig(f func(config *container.HostConfig)) RunOption {
	return runOption{
		Interface: option.New(identOptionHostConfig{}, f),
	}.run()
}

// WithNetworkingConfig modifies the configuration of network.
// The argument `config` already contains required values to connecting to bridge network,
// and a container cannot join multi-networks on container creation.
func WithNetworkingConfig(f func(config *network.NetworkingConfig)) RunOption {
	return runOption{
		Interface: option.New(identOptionNetworkingConfig{}, f),
	}.run()
}

// WithConfigConsistency enables/disables the test checking consistency of configurations.
// By default, this test is enabled.
// NOTICE: This is quite experimental feature.
func WithConfigConsistency(check bool) RunOption {
	return runOption{
		Interface: option.New(identOptionConfigConsistency{}, check),
	}.run()
}

// WithPullOptions enables to pull image that not exists.
// For example, if you want to use an image hosted in private repository,
// you have to fill RegistryAuth field.
//
// The output will be written to `out`. If nil, io.Discard will be used.
func WithPullOptions(opts *types.ImagePullOptions, out io.Writer) RunOption {
	return runOption{
		Interface: option.New(identOptionPullOption{}, pullOptions{
			pullOption: opts,
			pullOut:    out,
		}),
	}.run()
}

// Container represents a created container and its controller.
type Container struct {
	cft   *Confort
	id    string
	name  string
	alias string
	ports Ports
}

// LazyRun creates container but doesn't start.
// When container is required by UseShared or UseExclusive, the container starts.
//
// If container is already created/started by other test or process, LazyRun just
// store container info. It makes no error.
func (cft *Confort) LazyRun(tb testing.TB, ctx context.Context, c *ContainerParams, opts ...RunOption) *Container {
	tb.Helper()
	alias := c.Name
	name := cft.namespace.Namespace() + c.Name

	ctx, cancel := applyTimeout(ctx, cft.defaultTimeout)
	defer cancel()

	unlock, err := cft.ex.LockForContainerSetup(ctx, name)
	if err != nil {
		tb.Fatal(err)
	}
	defer unlock()

	containerID, err := cft.createContainer(ctx, name, alias, c, opts...)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
	return &Container{
		cft:   cft,
		id:    containerID,
		name:  name,
		alias: alias,
	}
}

// Run starts container with given parameters.
// If container already exists and not started, it starts.
// It reuses already started container and its endpoint information.
//
// When container is already existing and connected to another network, Run and other
// methods let the container connect to this network and create alias.
// For now, without specifying host port, container loses the port binding occasionally.
// If you want to use port binding and use a container with several network,
// and encounter such trouble, give it a try.
func (cft *Confort) Run(tb testing.TB, ctx context.Context, c *ContainerParams, opts ...RunOption) *Container {
	tb.Helper()
	alias := c.Name
	name := cft.namespace.Namespace() + c.Name

	ctx, cancel := applyTimeout(ctx, cft.defaultTimeout)
	defer cancel()

	unlock, err := cft.ex.LockForContainerSetup(ctx, name)
	if err != nil {
		tb.Fatal(err)
	}
	defer unlock()

	containerID, err := cft.createContainer(ctx, name, alias, c, opts...)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}

	ports, err := cft.namespace.StartContainer(ctx, name)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
	return &Container{
		cft:   cft,
		id:    containerID,
		name:  name,
		alias: alias,
		ports: ports,
	}
}

type (
	UseOption interface {
		option.Interface
		use() UseOption
	}
	identOptionReleaseFunc struct{}
	identOptionInitFunc    struct{}
	useOption              struct {
		option.Interface
	}
)

func (o useOption) use() UseOption { return o }

// WithReleaseFunc extracts the function to release lock of the container.
// With this option, the responsibility for releasing the container is passed to the user.
func WithReleaseFunc(f *func()) UseOption {
	return useOption{
		Interface: option.New(identOptionReleaseFunc{}, f),
	}.use()
}

type InitFunc func(ctx context.Context, ports Ports) error

// WithInitFunc sets initializer to set up container using the given port.
// The init will be performed only once per container, executed with an exclusive lock.
// If you use a container with Confort.UseShared, the lock state is downgraded to the shared lock after init.
//
// The returned error makes the acquired lock released and testing.TB fail.
// After that, you can attempt to use the container and init again.
func WithInitFunc(init InitFunc) UseOption {
	return useOption{
		Interface: option.New(identOptionInitFunc{}, init),
	}.use()
}

// Use acquires a lock for using the container and returns its endpoint. If exclusive is true, it requires to
// use the container exclusively.
// When other tests have already acquired an exclusive or shared lock for the container, it blocks until all
// previous locks are released.
func (c *Container) Use(tb testing.TB, ctx context.Context, exclusive bool, opts ...UseOption) Ports {
	tb.Helper()

	var releaseFunc *func()
	var initFunc InitFunc
	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionReleaseFunc{}:
			releaseFunc = opt.Value().(*func())
		case identOptionInitFunc{}:
			initFunc = opt.Value().(InitFunc)
		}
	}

	if c.ports == nil {
		unlock, err := c.cft.ex.LockForContainerSetup(ctx, c.name)
		if err != nil {
			tb.Fatal(err)
		}
		defer unlock()

		c.ports, err = c.cft.namespace.StartContainer(ctx, c.name)
		if err != nil {
			tb.Fatalf("confort: %s", err)
		}
	}

	var init func() error
	if initFunc != nil {
		init = func() error {
			return initFunc(ctx, c.ports)
		}
	}
	// If initFunc is not nil, it will be called after acquisition of exclusive lock.
	// After that, the lock is downgraded to shared lock when exclusive is false.
	// When initFunc returns error, the acquisition of lock fails.
	release, err := c.cft.ex.LockForContainerUse(ctx, c.name, exclusive, init)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}

	if releaseFunc != nil {
		*releaseFunc = release
	} else {
		tb.Cleanup(release)
	}
	return c.ports
}

func (c *Container) UseExclusive(tb testing.TB, ctx context.Context, opts ...UseOption) Ports {
	tb.Helper()
	return c.Use(tb, ctx, true, opts...)
}

func (c *Container) UseShared(tb testing.TB, ctx context.Context, opts ...UseOption) Ports {
	tb.Helper()
	return c.Use(tb, ctx, false, opts...)
}

// Network returns docker network representation associated with Confort.
func (cft *Confort) Network() *types.NetworkResource {
	return cft.namespace.Network()
}
