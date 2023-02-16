package confort

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/daichitakahashi/confort/compose"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/goccy/go-yaml"
	"go.uber.org/multierr"
)

type (
	// compose.ComposeBackend implementation using Docker Compose CLI.
	composeBackend struct {
		cli *client.Client
	}
	composer struct {
		cli             *client.Client
		dockerCompose   func(ctx context.Context, args ...string) *exec.Cmd
		modifiedConfig  []byte
		proj            projectConfig
		resourcePolicy  compose.ResourcePolicy
		scalingPolicies map[string]compose.ScalingPolicy

		m                  sync.Mutex
		services           map[string]*upService // services launched
		resourceLabel      string
		resourceLabelValue string
	}

	// configuration models
	projectConfig struct {
		Name     string                    `yaml:"name"`
		Services map[string]projectService `yaml:"services"`
	}
	projectService struct {
		Environment map[string]string `yaml:"environment"`
		Deploy      serviceDeploy     `yaml:"deploy"`
	}
	serviceDeploy struct {
		Replicas int `yaml:"replicas"`
	}

	upService struct {
		remove  bool
		service *compose.Service
	}
)

func (b *composeBackend) Load(ctx context.Context, configFile string, opts compose.LoadOptions) (compose.Composer, error) {
	args := []string{
		"compose",
	}
	if opts.ProjectDir != "" {
		args = append(args, "--project-directory", opts.ProjectDir)
	}
	if opts.ProjectName != "" {
		args = append(args, "--project-name", opts.ProjectName)
	}
	args = append(args, "--file", configFile)
	for _, f := range opts.OverrideConfigFiles {
		args = append(args, "--file", f)
	}
	for _, p := range opts.Profiles {
		args = append(args, "--profile", p)
	}
	if opts.EnvFile != "" {
		args = append(args, "--env-file", opts.EnvFile)
	}
	args = append(args, "config")

	// Load canonical config.
	var (
		stdout bytes.Buffer
		stderr strings.Builder
	)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if errors.As(err, new(*exec.ExitError)) {
			return nil, errors.New(stderr.String())
		}
		return nil, err
	}
	canonicalConfig := stdout.Bytes()

	v := map[string]any{}
	err = yaml.Unmarshal(canonicalConfig, &v)
	if err != nil {
		return nil, err
	}
	injectResourceLabel(v, "services", opts.ResourceIdentifierLabel, opts.ResourceIdentifier)
	injectResourceLabel(v, "volumes", opts.ResourceIdentifierLabel, opts.ResourceIdentifier)
	injectResourceLabel(v, "networks", opts.ResourceIdentifierLabel, opts.ResourceIdentifier)
	modifiedConfig, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}

	dockerCompose := func(ctx context.Context, commandArgs ...string) *exec.Cmd {
		args := []string{"compose", "--file", "-"}
		if opts.ProjectDir != "" {
			args = append(args, "--project-directory", opts.ProjectDir)
		}
		cmd := exec.CommandContext(ctx, "docker", append(args, commandArgs...)...)
		cmd.Stdin = bytes.NewReader(modifiedConfig)
		return cmd
	}

	var proj projectConfig
	err = yaml.Unmarshal(modifiedConfig, &proj)
	if err != nil {
		return nil, err
	}

	scalingPolicies := opts.ScalingPolicies
	if scalingPolicies == nil {
		scalingPolicies = map[string]compose.ScalingPolicy{}
	}

	return &composer{
		cli:                b.cli,
		dockerCompose:      dockerCompose,
		modifiedConfig:     modifiedConfig,
		proj:               proj,
		resourcePolicy:     opts.ResourcePolicy,
		scalingPolicies:    scalingPolicies,
		services:           map[string]*upService{},
		resourceLabel:      opts.ResourceIdentifierLabel,
		resourceLabelValue: opts.ResourceIdentifier,
	}, nil
}

func injectResourceLabel(v map[string]any, topLevel, labelName, labelValue string) {
	resources, ok := v[topLevel].(map[string]any)
	if !ok {
		return
	}
	for _, r := range resources {
		resource, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if external, _ := resource["external"].(bool); external {
			// skip external resource
			continue
		}
		labels, ok := resource["labels"].(map[string]string)
		if !ok {
			labels = map[string]string{}
			resource["labels"] = labels
		}
		labels[labelName] = labelValue
	}
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
	filter := types.ContainerListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("com.docker.compose.project=%s", c.proj.Name)),
			filters.Arg("label", fmt.Sprintf("com.docker.compose.service=%s", service)),
		),
	}
	list, err := c.cli.ContainerList(ctx, filter)
	if err != nil {
		return nil, err
	}

	initiate := len(list) == 0
	c.m.Lock()
	_, using := c.services[service]
	c.m.Unlock()

	if !initiate {
		if !using && !c.resourcePolicy.AllowReuse {
			return nil, fmt.Errorf("service conainter %q already exist", service)
		}
		// Check consistence of container num, if required.
		if c.scalingPolicies[service] == compose.ScalingPolicyConsistent && len(list) != requiredContainerN {
			return nil, errors.New("containers already exist, but its number is inconsistent with the request")
		}
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

		list, err = c.cli.ContainerList(ctx, filter)
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

	// Save service info
	c.m.Lock()
	if using {
		c.services[service].service.ContainerIDs = containerIDs // Update IDs of scaled container
	} else {
		c.services[service] = &upService{
			remove: c.resourcePolicy.Remove && (initiate || c.resourcePolicy.Takeover), // Remove or not
			service: &compose.Service{
				Name:         service,
				ContainerIDs: containerIDs,
				Env:          s.Environment,
			},
		}
	}
	c.m.Unlock()

	return &compose.Service{
		Name:         service,
		ContainerIDs: containerIDs,
		Env:          s.Environment,
	}, nil
}

func (c *composer) RemoveCreated(ctx context.Context, opts compose.RemoveOptions) error {
	// Remove containers.
	args := []string{
		"rm", "--force", "--stop",
	}
	if opts.RemoveAnonymousVolumes {
		args = append(args, "--volumes")
	}
	c.m.Lock()
	for name, service := range c.services { // Select created services
		if service.remove {
			args = append(args, name)
		}
	}
	c.m.Unlock()
	err := multierr.Append(
		nil,
		c.dockerCompose(ctx, args...).Run(),
	)

	// Remove volumes.
	f := filters.NewArgs(
		filters.Arg("label", fmt.Sprintf("com.docker.compose.project=%s", c.proj.Name)),
		filters.Arg("label", fmt.Sprintf("%s=%s", c.resourceLabel, c.resourceLabelValue)),
	)
	volumes, e := c.cli.VolumeList(ctx, f)
	err = multierr.Append(err, e)
	for _, v := range volumes.Volumes {
		err = multierr.Append(
			err,
			c.cli.VolumeRemove(ctx, v.Name, false),
		)
	}

	// Remove networks
	networks, e := c.cli.NetworkList(ctx, types.NetworkListOptions{
		Filters: f,
	})
	err = multierr.Append(err, e)
	for _, n := range networks {
		err = multierr.Append(
			err,
			c.cli.NetworkRemove(ctx, n.ID),
		)
	}
	return err
}

var _ compose.Composer = (*composer)(nil)
