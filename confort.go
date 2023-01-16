package confort

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/daichitakahashi/confort/internal/beacon"
	"github.com/daichitakahashi/confort/internal/beacon/proto"
	"github.com/daichitakahashi/confort/internal/exclusion"
	"github.com/daichitakahashi/confort/internal/logging"
	"github.com/daichitakahashi/confort/wait"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/lestrrat-go/option"
	"go.uber.org/multierr"
)

var initOnce sync.Once

func lazyInit() {
	initOnce.Do(func() {
		v, ok := os.LookupEnv(beacon.LogLevelEnv)
		if !ok {
			return
		}
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			log.Printf("confort: failed to parse the value of %s, default value is 2", beacon.LogLevelEnv)
			return
		}
		logging.SetLogLevel(logging.LogLevel(i))
	})
}

type Confort struct {
	backend        Backend
	namespace      Namespace
	defaultTimeout time.Duration
	ex             exclusion.Control
	term           func() error
}

type (
	newIdent interface {
		new()
	}
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
	newOption                 struct {
		option.Interface
		newIdent
	}

	NewComposeOption interface {
		option.Interface
		newIdent
		composeIdent
	}
	newComposeOption struct {
		option.Interface
		newIdent
		composeIdent
	}
)

// WithClientOptions sets options for Docker API client.
// Default option is client.FromEnv.
// For detail, see client.NewClientWithOpts.
func WithClientOptions(opts ...client.Opt) NewComposeOption {
	return newComposeOption{
		Interface: option.New(identOptionClientOptions{}, opts),
	}
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
	}
}

// WithDefaultTimeout sets the default timeout for each request to the Docker API and beacon server.
// The default value of the "default timeout" is 1 min.
// If default timeout is 0, Confort doesn't apply any timeout for ctx.
//
// If a timeout has already been set to ctx, the default timeout is not applied.
func WithDefaultTimeout(d time.Duration) NewComposeOption {
	return newComposeOption{
		Interface: option.New(identOptionDefaultTimeout{}, d),
	}
}

// WithResourcePolicy overrides the policy for handling Docker resources that already exist,
// such as containers and networks.
// By default, ResourcePolicyReuse or the value of the CFT_RESOURCE_POLICY environment variable, if set, is used.
// The "confort test" command has "-policy" option that overrides the variable.
func WithResourcePolicy(s ResourcePolicy) NewOption {
	return newOption{
		Interface: option.New(identOptionResourcePolicy{}, s),
	}
}

// WithBeacon configures Confort to integrate with a starting beacon server.
// The beacon server is started by the "confort" command.
// The address of server will be read from CFT_BEACON_ADDR or lock file specified as CFT_LOCKFILE.
//
// # With `confort test` command
//
// This command starts beacon server and sets the address as CFT_BEACON_ADDR automatically.
//
// # With `confort start` command
//
// This command starts beacon server and creates a lock file that contains the address.
// The default filename is ".confort.lock" and you don't need to set the file name as CFT_LOCKFILE.
// If you set a custom filename with "-lock-file" option, also you have to set the file name as CFT_LOCKFILE,
// or you can set address that read from lock file as CFT_BEACON_ADDR.
func WithBeacon() NewOption {
	return newOption{
		Interface: option.New(identOptionBeacon{}, true),
	}
}

// New creates Confort instance which is an interface of controlling containers.
// Confort creates docker resources like a network and containers. Also, it
// provides an exclusion control of container usage.
//
// If you want to control the same containers across parallelized tests, enable
// integration with the beacon server by using `confort` command and WithBeacon
// option.
func New(ctx context.Context, opts ...NewOption) (cft *Confort, err error) {
	lazyInit()

	var (
		skipDeletion bool
		beaconConn   *beacon.Connection
		ex           = exclusion.NewControl()

		clientOpts = []client.Opt{
			client.FromEnv,
		}
		namespace = os.Getenv(beacon.NamespaceEnv)
		timeout   = time.Minute
		policy    ResourcePolicy
	)
	if s := os.Getenv(beacon.ResourcePolicyEnv); s != "" {
		policy = ResourcePolicy(s)
	}

	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionClientOptions{}:
			clientOpts = opt.Value().([]client.Opt)
		case identOptionNamespace{}:
			o := opt.Value().(namespaceOption)
			if namespace == "" || o.force {
				if namespace != "" {
					logging.Infof("namespace is overwritten by WithNamespace: %q -> %q", namespace, o.namespace)
				}
				namespace = o.namespace
			}
		case identOptionDefaultTimeout{}:
			timeout = opt.Value().(time.Duration)
		case identOptionResourcePolicy{}:
			newPolicy := opt.Value().(ResourcePolicy)
			if policy != "" && policy != newPolicy {
				logging.Infof("resource policy is overwritten by WithResourcePolicy: %q -> %q", policy, newPolicy)
			}
			policy = newPolicy
		case identOptionBeacon{}:
			conn, err := beacon.Connect(ctx)
			if err != nil {
				return nil, err
			}
			if conn.Enabled() {
				ex = exclusion.NewBeaconControl(
					proto.NewBeaconServiceClient(conn.Conn),
				)
				skipDeletion = true
				beaconConn = conn
			}
		}
	}
	if beaconConn != nil {
		defer func() {
			if err != nil {
				_ = beaconConn.Close()
			}
		}()
	}

	if namespace == "" {
		return nil, errors.New("confort: empty namespace")
	}
	logging.Debugf("namespace: %s", namespace)

	if policy == "" {
		policy = ResourcePolicyReuse // default
	}
	if !beacon.ValidResourcePolicy(string(policy)) {
		return nil, fmt.Errorf("confort: invalid resource policy: %s", policy)
	}
	logging.Debugf("resource policy: %s", policy)

	ctx, cancel := applyTimeout(ctx, timeout)
	defer cancel()
	cli, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("confort: %w", err)
	}
	cli.NegotiateAPIVersion(ctx)

	var beaconAddr string
	if beaconConn.Enabled() {
		beaconAddr = beaconConn.Addr
	}
	backend := &dockerBackend{
		cli:    cli,
		policy: policy,
		labels: map[string]string{
			beacon.LabelIdentifier: beacon.Identifier(beaconAddr),
		},
	}

	logging.Debug("acquire LockForNamespace")
	unlock, err := ex.LockForNamespace(ctx)
	if err != nil {
		return nil, fmt.Errorf("confort: %w", err)
	}
	defer func() {
		logging.Debug("release LockForNamespace")
		unlock()
	}()

	logging.Debugf("create namespace %q", namespace)
	ns, err := backend.Namespace(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("confort: %w", err)
	}

	term := func() error {
		var err error
		if beaconConn.Enabled() {
			// TODO: disconnected from beacon server
			err = beaconConn.Close()
		}
		if skipDeletion {
			return err
		}
		// release all resources
		logging.Debugf("release all resources bound with namespace %q", namespace)
		return multierr.Append(err, ns.Release(context.Background()))
	}

	return &Confort{
		backend:        backend,
		namespace:      ns,
		defaultTimeout: timeout,
		ex:             ex,
		term:           term,
	}, nil
}

// Close releases all created resources with cft.
func (cft *Confort) Close() error {
	return cft.term()
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
	buildIdent interface {
		build()
	}
	BuildOption interface {
		option.Interface
		buildIdent
	}
	identOptionImageBuildOptions struct{}
	identOptionForceBuild        struct{}
	identOptionBuildOutput       struct{}
	buildOption                  struct {
		option.Interface
		buildIdent
	}
)

// WithImageBuildOptions modifies the configuration of build.
// The argument `option` already contains required values, according to Build.
func WithImageBuildOptions(f func(option *types.ImageBuildOptions)) BuildOption {
	return buildOption{
		Interface: option.New(identOptionImageBuildOptions{}, f),
	}
}

// WithForceBuild forces to build an image even if it already exists.
func WithForceBuild() BuildOption {
	return buildOption{
		Interface: option.New(identOptionForceBuild{}, true),
	}
}

// WithBuildOutput sets dst that the output during build will be written.
func WithBuildOutput(dst io.Writer) BuildOption {
	return buildOption{
		Interface: option.New(identOptionBuildOutput{}, dst),
	}
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
func (cft *Confort) Build(ctx context.Context, b *BuildParams, opts ...BuildOption) error {
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
		return fmt.Errorf("confort: %w", err)
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
		return errors.New("confort: image tag not specified")
	}
	logging.Debugf("LockForBuild: %s", buildOption.Tags[0])
	unlock, err := cft.ex.LockForBuild(ctx, buildOption.Tags[0])
	if err != nil {
		return fmt.Errorf("confort: %w", err)
	}
	defer func() {
		logging.Debugf("release LockForBuild: %s", buildOption.Tags[0])
		unlock()
	}()

	logging.Debugf("build image %q", buildOption.Tags[0])
	err = cft.backend.BuildImage(ctx, tarball, buildOption, force, buildOut)
	if err != nil {
		return fmt.Errorf("confort: %w", err)
	}
	return nil
}

type ContainerParams struct {
	Name         string
	Image        string
	Env          map[string]string
	Cmd          []string
	Entrypoint   []string
	ExposedPorts []string
	Waiter       *wait.Waiter
}

func (cft *Confort) createContainer(ctx context.Context, name, alias string, c *ContainerParams, opts ...RunOption) (string, error) {
	var modifyContainer func(config *container.Config)
	var modifyHost func(config *container.HostConfig)
	var modifyNetworking func(config *network.NetworkingConfig)
	var checkConsistency bool
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
	nw := cft.namespace.Network()
	nc := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			nw.Name: {
				NetworkID: nw.ID,
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
	runIdent interface {
		run()
	}
	RunOption interface {
		option.Interface
		runIdent
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
	runOption struct {
		option.Interface
		runIdent
	}
)

// WithContainerConfig modifies the configuration of container.
// The argument `config` already contains required values to create container,
// apply your values with care.
func WithContainerConfig(f func(config *container.Config)) RunOption {
	return runOption{
		Interface: option.New(identOptionContainerConfig{}, f),
	}
}

// WithHostConfig modifies the configuration of container from host side.
// The argument `config` already contains required values to create container,
// apply your values with care.
func WithHostConfig(f func(config *container.HostConfig)) RunOption {
	return runOption{
		Interface: option.New(identOptionHostConfig{}, f),
	}
}

// WithNetworkingConfig modifies the configuration of network.
// The argument `config` already contains required values to connecting to bridge network,
// and a container cannot join multi-networks on container creation.
func WithNetworkingConfig(f func(config *network.NetworkingConfig)) RunOption {
	return runOption{
		Interface: option.New(identOptionNetworkingConfig{}, f),
	}
}

// WithConfigConsistency enables/disables the test checking consistency of configurations.
// By default, this test is disabled.
// NOTICE: This is quite experimental feature.
func WithConfigConsistency(check bool) RunOption {
	return runOption{
		Interface: option.New(identOptionConfigConsistency{}, check),
	}
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
	}
}

// Container represents a created container and its controller.
type Container struct {
	cft   *Confort
	id    string
	name  string
	alias string
	ports Ports
}

// ID returns its container id.
func (c *Container) ID() string { return c.id }

// Name returns an actual name of the container.
func (c *Container) Name() string { return c.name }

// Alias returns a host name of the container. The alias is valid only in
// a docker network created in New or attached by Confort.Run.
func (c *Container) Alias() string { return c.alias }

// Run starts container with given parameters.
// If container already exists and not started, it starts.
// It reuses already started container and its endpoint information.
//
// When container is already existing and connected to another network, Run and other
// methods let the container connect to this network and create alias.
// For now, without specifying host port, container loses the port binding occasionally.
// If you want to use port binding and use a container with several network,
// and encounter such trouble, give it a try.
func (cft *Confort) Run(ctx context.Context, c *ContainerParams, opts ...RunOption) (*Container, error) {
	alias := c.Name
	name := cft.namespace.Namespace() + c.Name

	ctx, cancel := applyTimeout(ctx, cft.defaultTimeout)
	defer cancel()

	logging.Debugf("acquire LockForContainerSetup: %s", name)
	unlock, err := cft.ex.LockForContainerSetup(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("confort: %w", err)
	}
	defer func() {
		logging.Debugf("release LockForContainerSetup: %s", name)
		unlock()
	}()

	logging.Debugf("create container if not exists: %s", name)
	containerID, err := cft.createContainer(ctx, name, alias, c, opts...)
	if err != nil {
		return nil, fmt.Errorf("confort: %w", err)
	}

	logging.Debugf("start container if not started: %s", name)
	ports, err := cft.namespace.StartContainer(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("confort: %w", err)
	}
	return &Container{
		cft:   cft,
		id:    containerID,
		name:  name,
		alias: alias,
		ports: ports,
	}, nil
}

type (
	useIdent interface {
		use()
	}
	UseOption interface {
		option.Interface
		useIdent
	}
	identOptionInitFunc struct{}
	useOption           struct {
		option.Interface
		useIdent
	}
)

type (
	ReleaseFunc func()
	InitFunc    func(ctx context.Context, ports Ports) error
)

// WithInitFunc sets initializer to set up container using the given port.
// The init will be performed only once per container, executed with an exclusive lock.
// If you use a container with Confort.UseShared, the lock state is downgraded to the shared lock after init.
//
// The returned error makes the acquired lock released.
// After that, you can attempt to use the container and init again.
func WithInitFunc(init InitFunc) UseOption {
	return useOption{
		Interface: option.New(identOptionInitFunc{}, init),
	}
}

// Use acquires a lock for using the container and returns its endpoint. If exclusive is true, it requires to
// use the container exclusively.
// When other tests have already acquired an exclusive or shared lock for the container, it blocks until all
// previous locks are released.
func (c *Container) Use(ctx context.Context, exclusive bool, opts ...UseOption) (Ports, ReleaseFunc, error) {
	var initFunc InitFunc
	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionInitFunc{}:
			initFunc = opt.Value().(InitFunc)
		}
	}

	var init func(ctx context.Context) error
	if initFunc != nil {
		init = func(ctx context.Context) error {
			logging.Debugf("call InitFunc: %s", c.name)
			return initFunc(ctx, c.ports)
		}
	}
	// If initFunc is not nil, it will be called after acquisition of exclusive lock.
	// After that, the lock is downgraded to shared lock when exclusive is false.
	// When initFunc returns error, the acquisition of lock fails.
	logging.Debugf("acquire LockForContainerUse: %s(exclusive=%t)", c.name, exclusive)
	unlockContainer, err := c.cft.ex.LockForContainerUse(ctx, map[string]exclusion.ContainerUseParam{
		c.name: {
			Exclusive: exclusive,
			Init:      init,
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("confort: %w", err)
	}
	release := func() {
		logging.Debugf("release LockForContainerUse: %s(exclusive=%t)", c.name, exclusive)
		unlockContainer()
	}

	return c.ports, release, nil
}

// UseExclusive acquires an exclusive lock for using the container explicitly and returns its endpoint.
func (c *Container) UseExclusive(ctx context.Context, opts ...UseOption) (Ports, ReleaseFunc, error) {
	return c.Use(ctx, true, opts...)
}

// UseShared acquires a shared lock for using the container explicitly and returns its endpoint.
func (c *Container) UseShared(ctx context.Context, opts ...UseOption) (Ports, ReleaseFunc, error) {
	return c.Use(ctx, false, opts...)
}

// Network returns docker network representation associated with Confort.
func (cft *Confort) Network() *types.NetworkResource {
	return cft.namespace.Network()
}

type Acquirer struct {
	targets []*Container
	params  map[string]exclusion.ContainerUseParam
}

// Acquire initiates the acquisition of locks of the multi-containers.
// To avoid the deadlock in your test cases, use Acquire as below:
//
//	ports, release, err := Acquire().
//		Use(container1, true).
//		Use(container2, false, WithInitFunc(initContainer2)).
//		Do(ctx)
//	if err != nil {
//		t.Fatal(err)
//	}
//	t.Cleanup(release)
//
//	ports1 := ports[container1]
//	ports2 := ports[container2]
//
//	* Acquire locks of container1 and container2 at the same time
//	* If either lock acquisition or initContainer2 fails, lock acquisition for all containers fails
//	* If initContainer2 succeeded but acquisition failed, the successful result of init is preserved
//	* Returned func releases all acquired locks
func Acquire() *Acquirer {
	return &Acquirer{
		params: map[string]exclusion.ContainerUseParam{},
	}
}

// Use registers a container as the target of acquiring lock.
func (a *Acquirer) Use(c *Container, exclusive bool, opts ...UseOption) *Acquirer {
	var initFunc InitFunc
	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionInitFunc{}:
			initFunc = opt.Value().(InitFunc)
		}
	}

	var init func(ctx context.Context) error
	if initFunc != nil {
		init = func(ctx context.Context) error {
			logging.Debugf("call InitFunc: %s", c.name)
			return initFunc(ctx, c.ports)
		}
	}

	logging.Debugf("register target for LockForContainerUse: %s(exclusive=%t) to %p", c.name, exclusive, a)
	a.targets = append(a.targets, c)
	a.params[c.name] = exclusion.ContainerUseParam{
		Exclusive: exclusive,
		Init:      init,
	}
	return a
}

// UseExclusive registers a container as the target of acquiring exclusive lock.
func (a *Acquirer) UseExclusive(c *Container, opts ...UseOption) *Acquirer {
	return a.Use(c, true, opts...)
}

// UseShared registers a container as the target of acquiring shared lock.
func (a *Acquirer) UseShared(c *Container, opts ...UseOption) *Acquirer {
	return a.Use(c, false, opts...)
}

// Do acquisition of locks.
func (a *Acquirer) Do(ctx context.Context) (map[*Container]Ports, ReleaseFunc, error) {
	if len(a.targets) == 0 {
		return nil, nil, errors.New("no targets")
	}
	ex := a.targets[0].cft.ex

	logging.Debugf("acquire LockForContainerUse: %p", a)
	release, err := ex.LockForContainerUse(ctx, a.params)
	if err != nil {
		return nil, nil, err
	}

	ports := map[*Container]Ports{}
	for _, c := range a.targets {
		ports[c] = c.ports
	}

	logging.Debugf("release LockForContainerUse: %p", a)
	return ports, release, nil
}
