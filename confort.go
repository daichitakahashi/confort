package confort

import (
	"context"
	"fmt"
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

func New(ctx context.Context, opts ...NewOption) (*Confort, error) {
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
		return nil, err
	}
	backend := &dockerBackend{
		buildMu: newKeyedLock(),
		cli:     cli,
		policy:  policy,
	}
	ns, err := backend.Namespace(ctx, namespace)
	if err != nil {
		return nil, err
	}

	return &Confort{
		backend:        backend,
		namespace:      ns,
		defaultTimeout: defaultTimeout,
	}, nil
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
		buildAndRun() // TODO remove
	}
	identOptionImageBuildOptions struct{}
	buildOption                  struct{ option.Interface }
)

func (buildOption) build() {}

func WithImageBuildOptions(f func(option *types.ImageBuildOptions)) BuildOption {
	return buildOption{
		Interface: option.New(identOptionImageBuildOptions{}, f),
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

func (cft *Confort) Build(ctx context.Context, b *Build, opts ...BuildOption) error {
	ctx, cancel := cft.applyTimeout(ctx)
	defer cancel()

	var modifyBuildOptions func(option *types.ImageBuildOptions)

	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionImageBuildOptions{}:
			modifyBuildOptions = opt.Value().(func(option *types.ImageBuildOptions))
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

	out, err := cft.backend.BuildImage(ctx, b.ContextDir, buildOption)
	if err != nil {
		return err
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
		return fmt.Errorf("confort: %w", err)
	}
	return nil
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
		buildAndRun() // TODO: remove
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

func (cft *Confort) LazyRun(ctx context.Context, name string, c *Container, opts ...RunOption) error {
	ctx, cancel := cft.applyTimeout(ctx)
	defer cancel()

	return cft.createContainer(ctx, name, c, opts...)
}

func (cft *Confort) Run(ctx context.Context, name string, c *Container, opts ...RunOption) error {
	ctx, cancel := cft.applyTimeout(ctx)
	defer cancel()

	err := cft.createContainer(ctx, name, c, opts...)
	if err != nil {
		return err
	}

	_, err = cft.namespace.StartContainer(ctx, name, false)
	return cft.namespace.ReleaseContainer(ctx, name, false)
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
	var releaseFunc *func()
	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionReleaseFunc{}:
			releaseFunc = opt.Value().(*func())
		}
	}

	ports, err := cft.namespace.StartContainer(ctx, name, exclusive)
	if err != nil {
		tb.Fatal(err)
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

func (cft *Confort) UseShared(tb testing.TB, ctx context.Context, name string, opts ...UseOption) Ports {
	return cft.use(tb, ctx, name, false, opts...)
}

func (cft *Confort) UseExclusive(tb testing.TB, ctx context.Context, name string, opts ...UseOption) Ports {
	return cft.use(tb, ctx, name, true, opts...)
}
