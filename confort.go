package confort

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/daichitakahashi/confort/internal/beaconutil"
	"github.com/daichitakahashi/confort/proto/beacon"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/lestrrat-go/option"
)

type (
	NewOption interface {
		option.Interface
		new()
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
	newOption                 struct{ option.Interface }
)

func (newOption) new() {}

func WithClientOptions(opts ...client.Opt) NewOption {
	return newOption{
		Interface: option.New(identOptionClientOptions{}, opts),
	}
}

func WithNamespace(namespace string, force bool) NewOption {
	return newOption{
		Interface: option.New(identOptionNamespace{}, namespaceOption{
			namespace: namespace,
			force:     force,
		}),
	}
}

func WithDefaultTimeout(d time.Duration) NewOption {
	return newOption{
		Interface: option.New(identOptionDefaultTimeout{}, d),
	}
}

func WithResourcePolicy(s ResourcePolicy) NewOption {
	return newOption{
		Interface: option.New(identOptionResourcePolicy{}, s),
	}
}

func WithBeacon(conn *Connection) NewOption {
	return newOption{
		Interface: option.New(identOptionBeacon{}, conn),
	}
}

func New(tb testing.TB, ctx context.Context, opts ...NewOption) (*Confort, func()) {
	tb.Helper()

	var ex ExclusionControl = NewExclusionControl()
	var skipDeletion bool
	var beaconAddr string

	unlock, err := ex.NamespaceLock(ctx)
	if err != nil {
		tb.Fatal(err)
	}
	defer unlock()

	clientOps := []client.Opt{
		client.FromEnv,
	}
	namespace := os.Getenv(beaconutil.NamespaceEnv)
	defaultTimeout := time.Second * 30
	policy := ResourcePolicyReuse

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
			defaultTimeout = opt.Value().(time.Duration)
		case identOptionResourcePolicy{}:
			policy = opt.Value().(ResourcePolicy)
		case identOptionBeacon{}:
			c := opt.Value().(*Connection)
			if c.conn != nil {
				ex = &beaconControl{
					cli: beacon.NewBeaconServiceClient(c.conn),
				}
				skipDeletion = true
				beaconAddr = c.addr
			}
		}
	}
	if namespace == "" {
		tb.Fatal("confort: empty namespace")
	}

	cli, err := client.NewClientWithOpts(clientOps...)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
	cli.NegotiateAPIVersion(ctx)

	backend := &dockerBackend{
		cli:    cli,
		policy: policy,
		labels: map[string]string{
			beaconutil.LabelAddr: beaconAddr,
		},
	}
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

	return &Confort{
		backend:        backend,
		namespace:      ns,
		defaultTimeout: defaultTimeout,
		ex:             ex,
	}, term
}

type Confort struct {
	backend        Backend
	namespace      Namespace
	defaultTimeout time.Duration
	ex             ExclusionControl
}

func (cft *Confort) applyTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if cft.defaultTimeout == 0 {
		return ctx, func() {}
	}
	_, ok := ctx.Deadline()
	if ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, cft.defaultTimeout)
}

type (
	BuildOption interface {
		option.Interface
		build()
	}
	identOptionImageBuildOptions struct{}
	identOptionForceBuild        struct{}
	identOptionBuildOutput       struct{}
	buildOption                  struct{ option.Interface }
)

func (buildOption) build() {}

func WithImageBuildOptions(f func(option *types.ImageBuildOptions)) BuildOption {
	return buildOption{
		Interface: option.New(identOptionImageBuildOptions{}, f),
	}
}

func WithForceBuild() BuildOption {
	return buildOption{
		Interface: option.New(identOptionForceBuild{}, true),
	}
}

func WithBuildOutput(dst io.Writer) BuildOption {
	return buildOption{
		Interface: option.New(identOptionBuildOutput{}, dst),
	}
}

type Build struct {
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
func (cft *Confort) Build(tb testing.TB, ctx context.Context, b *Build, opts ...BuildOption) {
	tb.Helper()

	buildOut := io.Discard

	ctx, cancel := cft.applyTimeout(ctx)
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
			buildOut = opt.Value().(io.Writer)
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
	unlock, err := cft.ex.BuildLock(ctx, buildOption.Tags[0])
	if err != nil {
		tb.Fatal(err)
	}
	defer unlock()

	err = cft.backend.BuildImage(ctx, tarball, buildOption, force, buildOut)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
}

type Container struct {
	Image        string
	Env          map[string]string
	Cmd          []string
	Entrypoint   []string
	ExposedPorts []string
	Waiter       *Waiter
}

func (cft *Confort) createContainer(ctx context.Context, name, alias string, c *Container, opts ...RunOption) error {
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
		return err
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

	_, err = cft.namespace.CreateContainer(ctx, name, cc, hc, nc, checkConsistency, c.Waiter, pullOpts, pullOut)
	return err
}

type (
	RunOption interface {
		option.Interface
		run()
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

func (runOption) run() {}

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

func WithConfigConsistency(check bool) RunOption {
	return runOption{
		Interface: option.New(identOptionConfigConsistency{}, check),
	}
}

func WithPullOptions(opts *types.ImagePullOptions, out io.Writer) RunOption {
	return runOption{
		Interface: option.New(identOptionPullOption{}, pullOptions{
			pullOption: opts,
			pullOut:    out,
		}),
	}
}

// LazyRun creates container but doesn't start.
// When container is required by UseShared or UseExclusive, the container starts.
//
// If container is already created/started by other test or process, LazyRun just
// store container info. It makes no error.
func (cft *Confort) LazyRun(tb testing.TB, ctx context.Context, name string, c *Container, opts ...RunOption) {
	tb.Helper()
	alias := name
	name = cft.namespace.Namespace() + name

	ctx, cancel := cft.applyTimeout(ctx)
	defer cancel()

	unlock, err := cft.ex.InitContainerLock(ctx, name)
	if err != nil {
		tb.Fatal(err)
	}
	defer unlock()

	err = cft.createContainer(ctx, name, alias, c, opts...)
	if err != nil {
		tb.Fatalf("confort: %s", err)
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
func (cft *Confort) Run(tb testing.TB, ctx context.Context, name string, c *Container, opts ...RunOption) {
	tb.Helper()
	alias := name
	name = cft.namespace.Namespace() + name

	ctx, cancel := cft.applyTimeout(ctx)
	defer cancel()

	unlock, err := cft.ex.InitContainerLock(ctx, name)
	if err != nil {
		tb.Fatal(err)
	}
	defer unlock()

	err = cft.createContainer(ctx, name, alias, c, opts...)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}

	_, err = cft.namespace.StartContainer(ctx, name)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
}

type (
	UseOption interface {
		option.Interface
		use()
	}
	identOptionReleaseFunc struct{}
	identOptionInitFunc    struct{}
	useOption              struct {
		option.Interface
	}
)

func (useOption) use() {}

func WithReleaseFunc(f *func()) UseOption {
	return useOption{
		Interface: option.New(identOptionReleaseFunc{}, f),
	}
}

type InitFunc func(ctx context.Context) error

func WithInitFunc(init InitFunc) UseOption {
	return useOption{
		Interface: option.New(identOptionInitFunc{}, init),
	}
}

func (cft *Confort) use(tb testing.TB, ctx context.Context, name string, exclusive bool, opts ...UseOption) Ports {
	tb.Helper()
	name = cft.namespace.Namespace() + name

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

	unlock, err := cft.ex.InitContainerLock(ctx, name)
	if err != nil {
		tb.Fatal(err)
	}
	var unlocked bool
	defer func() {
		if !unlocked {
			unlock()
		}
	}()

	ports, err := cft.namespace.StartContainer(ctx, name)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
	var release func()
	if !exclusive && initFunc != nil {
		downgrade, cancel, ok, err := cft.ex.TryAcquireContainerInitLock(ctx, name)
		if err != nil {
			tb.Fatal(err)
		}
		if ok {
			err = initFunc(ctx)
			if err != nil {
				cancel()
				tb.Fatal(err)
			}
		}
		release, err = downgrade()
		if err != nil {
			cancel()
			tb.Fatal(err)
		}
	} else {
		release, err = cft.ex.AcquireContainerLock(ctx, name, exclusive)
		if err != nil {
			tb.Fatal(err)
		}
		if initFunc != nil {
			err = initFunc(ctx)
			if err != nil {
				release()
				tb.Fatalf("initFunc: %s", err)
			}
		}
	}

	unlock()
	unlocked = true

	if releaseFunc != nil {
		*releaseFunc = release
	} else {
		tb.Cleanup(release)
	}

	return ports
}

// UseShared tries to start container created by Run or LazyRun and returns endpoint info.
// If the container is already started by other test or process, UseShared reuse it.
//
// UseShared marks container "in use", but other call of UseShared is permitted.
func (cft *Confort) UseShared(tb testing.TB, ctx context.Context, name string, opts ...UseOption) Ports {
	tb.Helper()

	return cft.use(tb, ctx, name, false, opts...)
}

// UseExclusive tries to start container created by Run or LazyRun and returns endpoint
// info.
//
// UseExclusive requires to use container exclusively. When other UseShared marks
// the container "in use", it blocks until acquire exclusive control.
func (cft *Confort) UseExclusive(tb testing.TB, ctx context.Context, name string, opts ...UseOption) Ports {
	tb.Helper()

	return cft.use(tb, ctx, name, true, opts...)
}
