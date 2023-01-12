package compose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	composetypes "github.com/compose-spec/compose-go/types"
	"github.com/docker/cli/cli/command"
	composecmd "github.com/docker/compose/v2/cmd/compose"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	"github.com/docker/docker/client"
	"github.com/lestrrat-go/option"
)

type Compose struct {
	svc            api.Service
	proj           *composetypes.Project
	defaultTimeout time.Duration
}

type (
	NewOption interface {
		option.Interface
		new() NewOption
	}
	identOptionProjectDir     struct{}
	identOptionProjectName    struct{}
	identOptionClientOptions  struct{}
	identOptionDefaultTimeout struct{}
	newOption                 struct{ option.Interface }
)

func (o newOption) new() NewOption { return o }

// ModDir is a special value that indicates the location of go.mod of the test
// target module. Use with WithProjectDir option.
const ModDir = "\000mod\000"

// WithProjectDir sets project directory. The directory is used as working
// directory of the project.
// Also, file paths of configuration(compose.yaml) is resolved based on this
// directory. Default value is a current directory of the process.
//
// ModDir is a special value that indicate the location of go.mod of the test
// target module. This allows any package of modules to specify a configuration
// file with a relative path starting from the module's root directory.
func WithProjectDir(dir string) NewOption {
	return newOption{
		Interface: option.New(identOptionProjectDir{}, dir),
	}.new()
}

// WithProjectName sets project name, which works as namespace.
// Default name is a name of project directory.
func WithProjectName(name string) NewOption {
	return newOption{
		Interface: option.New(identOptionProjectName{}, name),
	}.new()
}

// WithClientOptions sets options for Docker API client.
// Default option is client.FromEnv.
// For detail, see client.NewClientWithOpts.
func WithClientOptions(opts ...client.Opt) NewOption {
	return newOption{
		Interface: option.New(identOptionClientOptions{}, opts),
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

func New(ctx context.Context, configFiles []string, opts ...NewOption) (*Compose, error) {
	if len(configFiles) == 0 {
		return nil, errors.New("no config file specified")
	}

	var (
		projectDir  = "."
		projectName string
		clientOpts  = []client.Opt{
			client.FromEnv,
		}
		timeout time.Duration
		err     error
	)
	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionProjectDir{}:
			projectDir = opt.Value().(string)
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

	// create docker cli instance and compose service
	dockerCli, err := command.NewDockerCli(
		command.DockerCliOption(command.WithInitializeClient(func(dockerCli *command.DockerCli) (client.APIClient, error) {
			apiClient, err := client.NewClientWithOpts(clientOpts...)
			if err != nil {
				return nil, err
			}
			apiClient.NegotiateAPIVersion(ctx)
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

	return &Compose{
		svc:            service,
		proj:           project,
		defaultTimeout: timeout,
	}, nil
}

func prepareProject(ctx context.Context, dir, name string, configFiles []string) (*composetypes.Project, error) {
	// ModDir is a special value.
	// Retrieve module file path and use its parent directory as a project directory.
	if dir == ModDir {
		val, err := resolveGoModDir(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get module directory: %w", err)
		}
		dir = val
	}
	// Resolve config file paths with project directory.
	configFiles, err := resolveConfigFilePath(dir, configFiles)
	if err != nil {
		return nil, err
	}

	proj, err := (&composecmd.ProjectOptions{
		ConfigPaths: configFiles,
		ProjectName: name,
		ProjectDir:  dir,
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
