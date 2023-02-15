package confort

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/daichitakahashi/confort/compose"
	"github.com/daichitakahashi/confort/internal/beacon"
	"github.com/daichitakahashi/confort/internal/beacon/proto"
	"github.com/daichitakahashi/confort/internal/exclusion"
	"github.com/daichitakahashi/confort/internal/logging"
	"github.com/daichitakahashi/confort/wait"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	"github.com/lestrrat-go/option"
)

type ComposeProject struct {
	composer       compose.Composer
	cli            *client.Client
	ex             exclusion.Control
	defaultTimeout time.Duration
	skipDeletion   bool
}

type (
	composeIdent  interface{ compose() }
	ComposeOption interface {
		option.Interface
		composeIdent
	}
	identOptionProjectDir          struct{}
	identOptionProjectName         struct{}
	identOptionOverrideConfigFiles struct{}
	identOptionProfiles            struct{}
	identOptionEnvFile             struct{}
	identOptionScalingPolicies     struct{}
	identOptionComposeBackend      struct{}
	composeOption                  struct {
		option.Interface
		composeIdent
	}
)

// ModDir is a special value that indicates the location of go.mod of the test
// target module. Use with WithProjectDir option.
const ModDir = "\000mod\000"

func WithProjectDir(dir ...string) ComposeOption {
	return composeOption{
		Interface: option.New(identOptionProjectDir{}, dir),
	}
}

func WithProjectName(name string) ComposeOption {
	return composeOption{
		Interface: option.New(identOptionProjectName{}, name),
	}
}

func WithOverrideConfigfiles(configFiles ...string) ComposeOption {
	return composeOption{
		Interface: option.New(identOptionOverrideConfigFiles{}, configFiles),
	}
}

func WithProfiles(profiles ...string) ComposeOption {
	return composeOption{
		Interface: option.New(identOptionProfiles{}, profiles),
	}
}

func WithEnvFile(filename string) ComposeOption {
	return composeOption{
		Interface: option.New(identOptionEnvFile{}, filename),
	}
}

// WithScalingPolicies declares scaling policies of every service. It works as a constraint in a ComposeProject.
//
//	WithScalingPolicies(map[string)compose.ScalingPolicy{
//	    "serviceA": compose.ScalingPolicyScalable, // It's default value, allows service to scale out.
//	    "serviceB": compose.ScalingPolicyConsistent, // Prohibit service from scaling out.
//	})
func WithScalingPolicies(p map[string]compose.ScalingPolicy) ComposeOption {
	return composeOption{
		Interface: option.New(identOptionScalingPolicies{}, p),
	}
}

func WithComposeBackend(b compose.Backend) ComposeOption {
	return composeOption{
		Interface: option.New(identOptionComposeBackend{}, b),
	}
}

func Compose(ctx context.Context, configFile string, opts ...ComposeOption) (*ComposeProject, error) {
	var (
		projectDir          string
		projectName         string
		overrideConfigFiles []string
		profiles            []string
		envFile             string
		policy                              = ResourcePolicyReuse
		scalingPolicies                     = map[string]compose.ScalingPolicy{}
		be                  compose.Backend = &composeBackend{}
		clientOpts                          = []client.Opt{
			client.FromEnv,
		}
		timeout      time.Duration
		beaconConn   *beacon.Connection
		ex           = exclusion.NewControl()
		skipDeletion bool
	)

	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionProjectDir{}:
			dir := opt.Value().([]string)
			for i := range dir {
				// ModDir is a special value.
				// Retrieve module file path and use its parent directory as a project directory.
				if dir[i] == ModDir {
					val, err := resolveGoModDir(ctx)
					if err != nil {
						return nil, fmt.Errorf("failed to get module directory: %w", err)
					}
					dir[i] = val
				}
			}
			projectDir = filepath.Join(dir...)
		case identOptionProjectName{}:
			projectName = opt.Value().(string)
		case identOptionOverrideConfigFiles{}:
			overrideConfigFiles = opt.Value().([]string)
		case identOptionProfiles{}:
			profiles = opt.Value().([]string)
		case identOptionEnvFile{}:
			envFile = opt.Value().(string)
		case identOptionScalingPolicies{}:
			scalingPolicies = opt.Value().(map[string]compose.ScalingPolicy)
		case identOptionComposeBackend{}:
			be = opt.Value().(compose.Backend)
		case identOptionClientOptions{}:
			clientOpts = opt.Value().([]client.Opt)
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

	ctx, cancel := applyTimeout(ctx, timeout)
	defer cancel()

	if !beacon.ValidResourcePolicy(string(policy)) {
		return nil, fmt.Errorf("confort: invalid resource policy: %s", policy)
	}

	// create docker API client
	apiClient, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, err
	}
	apiClient.NegotiateAPIVersion(ctx)

	identifier := uuid.NewString()
	if beaconConn.Enabled() {
		identifier = beacon.Identifier(beaconConn.Addr)
	}

	composer, err := be.Load(ctx, configFile, compose.LoadOptions{
		ProjectDir:              projectDir,
		ProjectName:             projectName,
		OverrideConfigFiles:     overrideConfigFiles,
		Profiles:                profiles,
		EnvFile:                 envFile,
		ResourcePolicy:          convertResourcePolicy(policy),
		ScalingPolicies:         scalingPolicies,
		ResourceIdentifierLabel: beacon.LabelIdentifier,
		ResourceIdentifier:      identifier,
	})
	if err != nil {
		return nil, fmt.Errorf("compose: %w", err)
	}

	return &ComposeProject{
		composer:       composer,
		cli:            apiClient,
		ex:             ex,
		defaultTimeout: timeout,
		skipDeletion:   skipDeletion,
	}, nil
}

func (c *ComposeProject) Close() error {
	if c.skipDeletion {
		return nil
	}
	return c.composer.RemoveCreated(context.Background(), compose.RemoveOptions{
		RemoveAnonymousVolumes: true,
	})
}

func resolveGoModDir(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "go", "env", "GOMOD").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get module directory: %w", err)
	}
	v := strings.TrimSpace(string(out))
	if v == os.DevNull {
		// If go.mod doesn't exist, use current directory.
		return ".", nil
	}
	_, err = os.Stat(v)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("go.mod not found: %s", v)
		}
		return "", fmt.Errorf("failed to check go.mod: %w", err)
	}
	return filepath.Dir(v), nil
}

func convertResourcePolicy(p ResourcePolicy) compose.ResourcePolicy {
	var policy compose.ResourcePolicy
	switch p {
	case beacon.ResourcePolicyReuse:
		policy = compose.ResourcePolicy{
			AllowReuse: true,
			Remove:     true,
		}
	case beacon.ResourcePolicyReusable:
		policy = compose.ResourcePolicy{
			AllowReuse: true,
		}
	case beacon.ResourcePolicyTakeOver:
		policy = compose.ResourcePolicy{
			AllowReuse: true,
			Remove:     true,
			Takeover:   true,
		}
	case beacon.ResourcePolicyError:
		policy = compose.ResourcePolicy{
			Remove: true,
		}
	}
	return policy
}

type (
	upIdent  interface{ up() }
	UpOption interface {
		option.Interface
		upIdent
	}
	identOptionScale  struct{}
	identOptionWaiter struct{}
	upOption          struct {
		option.Interface
		upIdent
	}
)

func WithScale(n int) UpOption {
	return upOption{
		Interface: option.New(identOptionScale{}, n),
	}
}

func WithWaiter(w *wait.Waiter) UpOption {
	return upOption{
		Interface: option.New(identOptionWaiter{}, w),
	}
}

type Service struct {
	c      *ComposeProject
	svc    *compose.Service
	ports  Ports
	waiter *wait.Waiter
}

func (c *ComposeProject) Up(ctx context.Context, service string, opts ...UpOption) (*Service, error) {
	ctx, cancel := applyTimeout(ctx, c.defaultTimeout)
	defer cancel()

	var (
		scale  int
		waiter *wait.Waiter
	)
	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionScale{}:
			scale = opt.Value().(int)
		case identOptionWaiter{}:
			waiter = opt.Value().(*wait.Waiter)
		}
	}

	svc, err := c.composer.Up(ctx, service, compose.UpOptions{
		Scale: scale,
	})
	if err != nil {
		return nil, err
	}

	var (
		ports    Ports
		fetchers []wait.Fetcher
	)
	for _, id := range svc.ContainerIDs {
		info, err := c.cli.ContainerInspect(ctx, id)
		if err != nil {
			return nil, err
		}
		boundPorts := make(nat.PortMap, len(info.NetworkSettings.Ports))
		// filter & merge
		for containerPort, binding := range info.NetworkSettings.Ports {
			if len(binding) == 0 {
				continue
			}
			boundPorts[containerPort] = binding
			ports[containerPort] = append(ports[containerPort], binding...)
		}
		fetchers = append(fetchers, &fetcher{
			cli:         c.cli,
			containerID: id,
			ports:       boundPorts,
		})
	}

	if waiter != nil {
		// Check container readiness respectively
		err = waiter.WaitForReady(ctx, fetchers)
		if err != nil {
			return nil, err
		}
	}

	return &Service{
		c:      c,
		svc:    svc,
		ports:  ports,
		waiter: waiter,
	}, nil
}

func (s *Service) Use(ctx context.Context, exclusive bool, opts ...UseOption) (Ports, ReleaseFunc, error) {
	return use(ctx, s, exclusive, opts...)
}

func (s *Service) containerIdent() string {
	return s.svc.Name
}

func (s *Service) containerPorts() Ports {
	return s.ports
}

func (s *Service) exclusionControl() exclusion.Control {
	return s.c.ex
}

func (s *Service) UseExclusive(ctx context.Context, opts ...UseOption) (Ports, ReleaseFunc, error) {
	return use(ctx, s, true, opts...)
}

func (s *Service) UseShared(ctx context.Context, opts ...UseOption) (Ports, ReleaseFunc, error) {
	return use(ctx, s, false, opts...)
}
