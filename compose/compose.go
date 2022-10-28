package compose

import (
	"context"

	"github.com/compose-spec/compose-go/cli"
	"github.com/docker/cli/cli/command"
	composecmd "github.com/docker/compose/v2/cmd/compose"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	"github.com/docker/docker/client"
)

type Compose struct{}

func New(ctx context.Context, configFiles []string) (*Compose, error) {
	dockerCli, err := command.NewDockerCli(
		command.DockerCliOption(command.WithInitializeClient(func(dockerCli *command.DockerCli) (client.APIClient, error) {
			apiClient, err := client.NewClientWithOpts(client.FromEnv)
			if err != nil {
				return nil, err
			}
			apiClient.NegotiateAPIVersion(context.Background())
			return apiClient, nil
		})),
	)
	if err != nil {
		return nil, err
	}
	service := api.NewServiceProxy().
		WithService(compose.NewComposeService(dockerCli))

	opts := composecmd.ProjectOptions{
		ConfigPaths: configFiles,
		// ProjectDir:  "",
		// WorkDir: "",
	}
	project, err := opts.ToProject(nil, cli.WithDefaultConfigPath)
	if err != nil {
		return nil, err
	}
	for _, service := range project.Services {
		service.Labels = service.Labels.
			Add("CUSTOM_ENV1", "VALUE1").
			Add("CUSTOM_ENV2", "VALUE2")
	}

	err = service.Up(ctx, project, api.UpOptions{})
	if err != nil {
		return nil, err
	}

	return &Compose{}, nil
}
