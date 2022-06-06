package confort

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
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
		overwrite bool
	}
	identOptionDefaultTimeout struct{}
	identOptionResourcePolicy struct{}
	newOption                 struct{ option.Interface }
)

func (newOption) new() {}

func WithClientOptions(opts ...client.Opt) NewOption {
	return newOption{
		Interface: option.New(identOptionClientOptions{}, opts),
	}
}

func WithNamespace(namespace string, overwrite bool) NewOption {
	return newOption{
		Interface: option.New(identOptionNamespace{}, namespaceOption{
			namespace: namespace,
			overwrite: overwrite,
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

func New(tb testing.TB, ctx context.Context, opts ...NewOption) (*Confort, func()) {
	tb.Helper()

	clientOps := []client.Opt{
		client.FromEnv,
	}
	namespace := os.Getenv("CFT_NAMESPACE")
	defaultTimeout := time.Second * 30
	policy := ResourcePolicyReuse

	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionClientOptions{}:
			clientOps = opt.Value().([]client.Opt)
		case identOptionNamespace{}:
			o := opt.Value().(namespaceOption)
			if namespace == "" || o.overwrite {
				namespace = o.namespace
			}
		case identOptionDefaultTimeout{}:
			defaultTimeout = opt.Value().(time.Duration)
		case identOptionResourcePolicy{}:
			policy = opt.Value().(ResourcePolicy)
		}
	}

	cli, err := client.NewClientWithOpts(clientOps...)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
	cli.NegotiateAPIVersion(ctx)

	backend := &dockerBackend{
		buildMu: newKeyedLock(),
		cli:     cli,
		policy:  policy,
	}
	ns, err := backend.Namespace(ctx, namespace)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
	term := func() {
		tb.Helper()
		err := ns.Release(context.Background())
		if err != nil {
			tb.Log(err)
		}
	}

	return &Confort{
		backend:        backend,
		namespace:      ns,
		defaultTimeout: defaultTimeout,
	}, term
}

type Confort struct {
	backend        Backend
	namespace      Namespace
	defaultTimeout time.Duration
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

type Build struct {
	Image        string
	Dockerfile   string
	ContextDir   string
	BuildArgs    map[string]*string
	Output       bool
	Platform     string
	Env          map[string]string
	Cmd          []string
	Entrypoint   []string
	ExposedPorts []string
	Waiter       *Waiter
}

// Build creates new image from given Dockerfile and context directory.
//
// When same name image already exists, it doesn't perform building.
// WithForceBuild enables us to build image on every call of Build.
func (cft *Confort) Build(tb testing.TB, ctx context.Context, b *Build, opts ...BuildOption) {
	tb.Helper()

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
		}
	}

	buildOption := types.ImageBuildOptions{
		Tags:           []string{b.Image},
		SuppressOutput: !b.Output,
		Remove:         true,
		PullParent:     true,
		Dockerfile:     b.Dockerfile,
		BuildArgs:      b.BuildArgs,
		Target:         "",
		SessionID:      "",
		Platform:       b.Platform,
	}
	if modifyBuildOptions != nil {
		modifyBuildOptions(&buildOption)
	}

	out, err := cft.backend.BuildImage(ctx, b.ContextDir, buildOption, force)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
	if out == nil {
		return
	}
	err = func() error {
		defer func() {
			_, _ = io.ReadAll(out)
			_ = out.Close()
		}()
		if !b.Output {
			return nil
		}
		return jsonmessage.DisplayJSONMessagesStream(out, os.Stdout, 0, false, nil)
	}()
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

func (cft *Confort) createContainer(ctx context.Context, name string, c *Container, opts ...RunOption) error {
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
				Aliases:   []string{name},
			},
		},
	}
	if modifyNetworking != nil {
		modifyNetworking(nc)
	}

	_, err = cft.namespace.CreateContainer(ctx, name, cc, hc, nc, pullOptions)
	return err
}

type (
	RunOption interface {
		option.Interface
		run()
	}
	identOptionContainerConfig  struct{}
	identOptionHostConfig       struct{}
	identOptionNetworkingConfig struct{}
	identOptionPullOptions      struct{}
	runOption                   struct{ option.Interface }
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

func WithPullOptions(opts types.ImagePullOptions) RunOption {
	return runOption{
		Interface: option.New(identOptionPullOptions{}, opts),
	}
}

// LazyRun creates container but doesn't start.
// When container is required by UseShared or UseExclusive, the container starts.
//
// If container is already created/started by other test or process, LazyRun just
// store container info. It makes no error.
func (cft *Confort) LazyRun(tb testing.TB, ctx context.Context, name string, c *Container, opts ...RunOption) {
	tb.Helper()

	ctx, cancel := cft.applyTimeout(ctx)
	defer cancel()

	err := cft.createContainer(ctx, name, c, opts...)
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

	ctx, cancel := cft.applyTimeout(ctx)
	defer cancel()

	err := cft.createContainer(ctx, name, c, opts...)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}

	_, err = cft.namespace.StartContainer(ctx, name, false)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
	err = cft.namespace.ReleaseContainer(ctx, name, false)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
}

type Ports map[nat.Port][]string

func (p Ports) Binding(port nat.Port) (string, bool) {
	bindings, ok := p[port]
	if !ok || len(bindings) == 0 {
		return "", false
	}
	return bindings[0], true
}

type (
	UseOption interface {
		option.Interface
		use()
	}
	identOptionReleaseFunc struct{}
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

func (cft *Confort) use(tb testing.TB, ctx context.Context, name string, exclusive bool, opts ...UseOption) Ports {
	tb.Helper()

	var releaseFunc *func()
	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionReleaseFunc{}:
			releaseFunc = opt.Value().(*func())
		}
	}

	ports, err := cft.namespace.StartContainer(ctx, name, exclusive)
	if err != nil {
		tb.Fatalf("confort: %s", err)
	}
	p := make(Ports, len(ports))
	for port, bindings := range ports {
		endpoints := make([]string, len(bindings))
		for i, b := range bindings {
			endpoints[i] = b.HostIP + ":" + b.HostPort // TODO: specify host ip ???
		}
		p[port] = endpoints
	}
	release := func() {
		_ = cft.namespace.ReleaseContainer(ctx, name, exclusive)
	}
	if releaseFunc != nil {
		*releaseFunc = release
	} else {
		tb.Cleanup(release)
	}

	return p
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
