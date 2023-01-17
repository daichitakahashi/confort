package confort

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	composetypes "github.com/compose-spec/compose-go/types"
	"github.com/daichitakahashi/confort/internal/exclusion"
	"github.com/daichitakahashi/confort/wait"
	"github.com/docker/cli/cli/command"
	composecmd "github.com/docker/compose/v2/cmd/compose"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	"github.com/docker/docker/client"
	"github.com/lestrrat-go/option"
	"go.uber.org/multierr"
)

type ComposeProject struct {
	cli            *client.Client
	svc            api.Service
	proj           *composetypes.Project
	defaultTimeout time.Duration
	ex             exclusion.Control

	m        sync.Mutex
	services map[string]bool
}

type (
	composeIdent  interface{ compose() }
	ComposeOption interface {
		option.Interface
		composeIdent
	}
	identOptionProjectDir  struct{}
	identOptionProjectName struct{}
	composeOption          struct {
		option.Interface
		composeIdent
	}
)

// ModDir is a special value that indicates the location of go.mod of the test
// target module. Use with WithProjectDir option.
const ModDir = "\000mod\000"

// WithProjectDir sets project directory. The directory is used as working
// directory of the project.
// Also, file paths of configuration(compose.yaml) is resolved based on this
// directory. Default value is a current directory of the process.
//
// If ModDir is passed as a part of args, the value is replaced with the location
// of go.mod of the test target module.
// This allows any test code of the module to specify same configuration files.
func WithProjectDir(dir ...string) ComposeOption {
	return composeOption{
		Interface: option.New(identOptionProjectDir{}, dir),
	}
}

// WithProjectName sets project name, which works as namespace.
// Default name is a name of project directory.
func WithProjectName(name string) ComposeOption {
	return composeOption{
		Interface: option.New(identOptionProjectName{}, name),
	}
}

func Compose(ctx context.Context, configFiles []string, opts ...ComposeOption) (*ComposeProject, error) {
	if len(configFiles) == 0 {
		return nil, errors.New("no config file specified")
	}

	var (
		projectDir  = []string{"."}
		projectName string
		clientOpts  = []client.Opt{
			client.FromEnv,
		}
		timeout time.Duration
		ex      = exclusion.NewControl()
		err     error
	)
	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionProjectDir{}:
			projectDir = opt.Value().([]string)
		case identOptionProjectName{}:
			projectName = opt.Value().(string)
		case identOptionClientOptions{}:
			clientOpts = opt.Value().([]client.Opt)
		case identOptionDefaultTimeout{}:
			timeout = opt.Value().(time.Duration)
		}
	}

	ctx, cancel := applyTimeout(ctx, timeout)
	defer cancel()

	// create docker API client
	apiClient, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, err
	}
	apiClient.NegotiateAPIVersion(ctx)

	// create docker cli instance and compose service
	dockerCli, err := command.NewDockerCli(
		command.DockerCliOption(command.WithInitializeClient(func(dockerCli *command.DockerCli) (client.APIClient, error) {
			return apiClient, nil
		})),
	)
	if err != nil {
		return nil, err
	}
	service := api.NewServiceProxy().
		WithService(compose.NewComposeService(dockerCli))

	// load configurations
	project, err := prepareProject(ctx, projectDir, projectName, configFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration files correctly: %w", err)
	}
	for _, service := range project.Services {
		service.CustomLabels = service.CustomLabels.
			Add("CUSTOM_ENV1", "VALUE1").
			Add("CUSTOM_ENV2", "VALUE2")
	}

	return &ComposeProject{
		cli:            apiClient,
		svc:            service,
		proj:           project,
		defaultTimeout: timeout,
		ex:             ex,
		services:       map[string]bool{},
	}, nil
}

func (c *ComposeProject) Close() error {
	c.m.Lock()
	defer c.m.Unlock()

	services := make([]string, 0, len(c.services))
	for service := range c.services {
		services = append(services, service)
	}
	if len(services) == 0 {
		return nil
	}

	ctx := context.Background()
	return multierr.Append(
		c.svc.Stop(ctx, c.proj.Name, api.StopOptions{
			Services: services,
		}),
		c.svc.Remove(ctx, c.proj.Name, api.RemoveOptions{
			Volumes:  true, // TODO: requires consideration
			Force:    true,
			Services: services,
		}),
	)
}

func prepareProject(ctx context.Context, dir []string, name string, configFiles []string) (*composetypes.Project, error) {
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
	projectDir := filepath.Join(dir...)

	// Resolve config file paths with project directory.
	configFiles, err := resolveConfigFilePath(projectDir, configFiles)
	if err != nil {
		return nil, err
	}
	proj, err := (&composecmd.ProjectOptions{
		ConfigPaths: configFiles,
		ProjectName: name,
		ProjectDir:  projectDir,
	}).ToProject(nil) // Specify services to launch
	if err != nil {
		return nil, fmt.Errorf("failed to load project: %w", err)
	}
	// If projectDir indicates root directory, project name will be empty.
	// This causes invalid container name("{project}-{service}-{number}" => "-{service}-{number}"),
	// and container creation fails.
	// We treat this as an error before container creation.
	if proj.Name == "" {
		return nil, fmt.Errorf("project name required")
	}
	return proj, nil
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

func resolveConfigFilePath(base string, configFiles []string) (r []string, err error) {
	r = make([]string, 0, len(configFiles))
	for _, f := range configFiles {
		if f == "" || f == "-" {
			continue // Ignore empty value and stdin.
		}
		if !filepath.IsAbs(f) {
			// Adjust config file path based on specified directory.
			// It is required because cli.ProjectFromOptions resolves configuration file paths
			// based on working directory of its process.
			f = filepath.Join(base, f)
		}
		r = append(r, f)
	}
	return r, nil
}

type (
	upIdent  interface{ up() }
	UpOption interface {
		option.Interface
		upIdent
	}
	identOptionWaiter struct{}
	upOption          struct {
		option.Interface
		upIdent
	}
)

func WithWaiter(w *wait.Waiter) UpOption {
	return upOption{
		Interface: option.New(identOptionWaiter{}, w),
	}
}

type Service struct {
	c     *ComposeProject
	s     composetypes.ServiceConfig
	name  string
	ports Ports
}

func (c *ComposeProject) Up(ctx context.Context, service string, opts ...UpOption) (*Service, error) {
	// Check service name.
	serviceConfig, err := c.proj.GetService(service)
	if err != nil {
		return nil, fmt.Errorf("compose: %w", err)
	}

	ctx, cancel := applyTimeout(ctx, c.defaultTimeout)
	defer cancel()

	var w *wait.Waiter
	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionWaiter{}:
			w = opt.Value().(*wait.Waiter)
		}
	}

	name := fmt.Sprintf("%s-%s", c.proj.Name, service)
	unlock, err := c.ex.LockForContainerSetup(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock of %q: %w", service, err)
	}
	defer unlock()

	// Check service status.
	s, err := c.svc.Ps(ctx, c.proj.Name, api.PsOptions{
		Services: []string{service},
		All:      true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get service info: %w", err)
	}

	doUp := len(s) == 0
	if !doUp {
		switch s[0].State {
		case "running":
		case "created", "exiting":
			doUp = true
		case "paused":
			return nil, fmt.Errorf("cannot start %q, unpause is not supported", service)
		default:
			return nil, fmt.Errorf("cannot start %q, unexpected container state %q", service, s[0].State)
		}
	}
	if doUp {
		// If the running service doesn't exist, create and start it.
		err = c.svc.Up(ctx, c.proj, api.UpOptions{
			Create: api.CreateOptions{
				Services: []string{service},
			},
			Start: api.StartOptions{
				Services: []string{service},
				Wait:     true, // Wait until running/healthy state(depend on configuration).
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to launch service %q: %w", service, err)
		}

		s, err = c.svc.Ps(ctx, c.proj.Name, api.PsOptions{
			Services: []string{service},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get service info: %w", err)
		} else if len(s) == 0 {
			return nil, fmt.Errorf("service %q not found", service)
		}

		c.m.Lock()
		c.services[service] = true
		c.m.Unlock()
	}

	// Get bound ports.
	containerID := s[0].ID
	info, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get service container info: %w", err)
	}

	if w != nil {
		err = w.Wait(ctx, &fetcher{
			cli:         c.cli,
			containerID: containerID,
			ports:       info.NetworkSettings.Ports,
		})
		if err != nil {
			return nil, fmt.Errorf("error on waiting service: %w", err)
		}
	}

	return &Service{
		c:     c,
		s:     serviceConfig,
		name:  name,
		ports: Ports(info.NetworkSettings.Ports),
	}, nil
}

func (s *Service) Use(ctx context.Context, exclusive bool, opts ...UseOption) (Ports, ReleaseFunc, error) {
	return use(ctx, s, exclusive, opts...)
}

func (s *Service) containerIdent() string {
	return s.name
}

func (s *Service) containerPorts() Ports {
	return s.ports
}

func (s *Service) exclusionControl() exclusion.Control {
	return s.c.ex
}

// UseExclusive acquires an exclusive lock for using the container explicitly and returns its endpoint.
func (s *Service) UseExclusive(ctx context.Context, opts ...UseOption) (Ports, ReleaseFunc, error) {
	return use(ctx, s, true, opts...)
}

// UseShared acquires a shared lock for using the container explicitly and returns its endpoint.
func (s *Service) UseShared(ctx context.Context, opts ...UseOption) (Ports, ReleaseFunc, error) {
	return use(ctx, s, false, opts...)
}
