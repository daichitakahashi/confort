package confort

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"sync"

	"github.com/daichitakahashi/confort/compose"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

type (
	// Backend implementation using Docker Compose CLI.
	composeBackend struct {
		cli *client.Client
	}
	composer struct {
		cli           *client.Client
		dockerCompose func(ctx context.Context, args ...string) *exec.Cmd
		proj          projectConfig

		m        sync.Mutex
		services map[string]bool // services launched
	}

	projectConfig struct {
		Name     string                    `json:"name"`
		Services map[string]projectService `json:"services"`
	}
	projectService struct {
		Environment map[string]string `json:"environment"`
		Deploy      serviceDeploy     `json:"deploy"`
	}
	serviceDeploy struct {
		Replicas int `json:"replicas"`
	}
)

func (b *composeBackend) Load(ctx context.Context, projectDir string, configFiles, profiles []string, envFile string) (compose.Composer, error) {
	args := []string{
		"compose",
	}
	if projectDir != "" {
		args = append(args, []string{"--project-dir", projectDir}...)
	}
	for _, f := range configFiles {
		args = append(args, []string{"--file", f}...)
	}
	for _, p := range profiles {
		args = append(args, []string{"--profile", p}...)
	}
	if envFile != "" {
		args = append(args, []string{"--env-file", envFile}...)
	}
	dockerCompose := func(ctx context.Context, commandArgs ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "docker", append(args, commandArgs...)...)
	}

	// Load unified config as json.
	raw, err := dockerCompose(ctx, "convert", "--format", "json").Output()
	if err != nil {
		return nil, err
	}
	var config projectConfig
	err = json.Unmarshal(raw, &config)
	if err != nil {
		return nil, err
	}

	return &composer{
		cli:           b.cli,
		dockerCompose: dockerCompose,
		proj:          config,
	}, nil
}

var _ compose.Backend = (*composeBackend)(nil)

func (c *composer) ProjectName() string {
	return c.proj.Name
}

func (c *composer) Up(ctx context.Context, service string, opts compose.UpOptions) (*compose.Service, error) {
	s, ok := c.proj.Services[service]
	if !ok {
		return nil, fmt.Errorf("service %q not found", service)
	}

	// Get required container num.
	// The --scale option precedes.
	requiredContainerN := s.Deploy.Replicas
	if 0 < opts.Scale {
		requiredContainerN = opts.Scale
	}
	if requiredContainerN == 0 {
		requiredContainerN = 1
	}

	// Get existing containers of the service
	list, err := c.cli.ContainerList(ctx, types.ContainerListOptions{
		Filters: filters.NewArgs(
			filters.Arg("com.docker.compose.project", c.proj.Name),
			filters.Arg("com.docker.compose.service", service),
		),
	})
	if err != nil {
		return nil, err
	}

	// Check consistence of container num, if required.
	if opts.ScalingStrategy == compose.ScalingStrategyConsistent &&
		0 < len(list) && len(list) != requiredContainerN {
		return nil, errors.New("containers already exist, but its number is inconsistent with the request")
	}

	if len(list) < requiredContainerN { // More containers are required.
		// TODO: consider options
		args := []string{
			"up", service, "--detach", "--wait",
		}
		if 0 < opts.Scale {
			args = append(args, "--scale", fmt.Sprintf("%s=%d", service, opts.Scale))
			if requiredContainerN < opts.Scale {
				requiredContainerN = opts.Scale
			}
		}
		_, err = c.dockerCompose(ctx, args...).Output() // TODO: log output to stdout?
		if err != nil {
			return nil, err
		}

		// Service is initiated by this op.
		if len(list) == 0 {
			c.m.Lock()
			c.services[service] = true
			c.m.Unlock()
		}

		list, err = c.cli.ContainerList(ctx, types.ContainerListOptions{
			All: true,
			Filters: filters.NewArgs(
				filters.Arg("com.docker.compose.project", c.proj.Name),
				filters.Arg("com.docker.compose.service", service),
			),
		})
		if err != nil {
			return nil, err
		}
	}

	// Sort containers according to container number.
	sort.Slice(list, func(i, j int) bool {
		return list[i].Names[0] < list[j].Names[0]
	})
	containerIDs := make([]string, 0, len(list))
	for _, c := range list {
		containerIDs = append(containerIDs, c.ID)
	}

	return &compose.Service{
		Name:         service,
		ContainerIDs: containerIDs,
		Env:          s.Environment,
	}, nil
}

func (c *composer) RemoveCreated(ctx context.Context, opts compose.RemoveOptions) error {
	args := []string{
		"rm", "--force", "--stop",
	}
	if opts.RemoveAnonymousVolumes {
		args = append(args, "--volumes")
	}
	c.m.Lock()
	for service := range c.services { // Select created services
		args = append(args, service)
	}
	c.m.Unlock()
	return c.dockerCompose(ctx, args...).Run()
}

func (c *composer) Down(ctx context.Context, opts compose.DownOptions) error {
	args := []string{
		"down",
	}
	if opts.RemoveOrphans {
		args = append(args, "--remove--orphans")
	}
	if opts.RemoveVolumes {
		args = append(args, "--volumes")
	}
	return c.dockerCompose(ctx, args...).Run()
}

var _ compose.Composer = (*composer)(nil)
