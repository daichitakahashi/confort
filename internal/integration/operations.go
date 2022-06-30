package integration

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/lestrrat-go/backoff/v2"
)

type Operation interface {
	StartBeaconServer(ctx context.Context, image string) (string, error)
	StopBeaconServer(ctx context.Context, endpoint string) error
	CleanupResources(ctx context.Context) error
	ExecuteTest(ctx context.Context, args []string, environments []string) error
}

type operation struct {
	cli *client.Client
}

func NewOperation() (Operation, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &operation{
		cli: cli,
	}, nil
}

func (o *operation) StartBeaconServer(ctx context.Context, image string) (string, error) {
	// TODO: pull image if not exists

	created, err := o.cli.ContainerCreate(ctx, &container.Config{
		Image: image,
		ExposedPorts: nat.PortSet{
			"8080/tcp": struct{}{},
		},
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			"8080/tcp": []nat.PortBinding{
				{},
			},
		},
		AutoRemove: true,
	}, &network.NetworkingConfig{}, nil, "")
	if err != nil {
		return "", err
	}

	err = o.cli.ContainerStart(ctx, created.ID, types.ContainerStartOptions{})
	if err != nil {
		return "", err
	}

	b := backoff.Constant(
		backoff.WithInterval(200*time.Millisecond),
		backoff.WithMaxRetries(150),
	).Start(ctx)
	for backoff.Continue(b) {
		i, err := o.cli.ContainerInspect(ctx, created.ID)
		if err != nil {
			return "", err
		}

		if i.State.Health.Status != "healthy" {
			continue
		}

		port, ok := i.NetworkSettings.Ports["8080/tcp"]
		if !ok {
			continue
		}
		if len(port) == 0 {
			// endpoint not bound yet
			continue
		}
		return fmt.Sprintf("%s:%s", port[0].HostIP, port[0].HostPort), nil
	}
	return "", errors.New("cannot obtain beacon endpoint")
}

func (o *operation) StopBeaconServer(ctx context.Context, endpoint string) error {
	_, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return err
	}

	containers, err := o.cli.ContainerList(ctx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("expose", "8080"),
		),
	})
	if err != nil {
		return err
	}

	var target string
find:
	for _, c := range containers {
		for _, p := range c.Ports {
			pp := strconv.FormatUint(uint64(p.PublicPort), 10)
			if pp == port {
				target = c.ID
				break find
			}
		}
	}
	if target == "" {
		return errors.New("container not found")
	}

	return o.cli.ContainerRemove(ctx, target, types.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})
}

func (o *operation) CleanupResources(ctx context.Context) error {
	// TODO: implement
	return nil
}

func (o *operation) ExecuteTest(ctx context.Context, args, environments []string) error {
	goCmd := "go"
	goRoot := os.Getenv("GOROOT")
	if goRoot != "" {
		goCmd = filepath.Join(goRoot, "bin/go")
	}

	cmd := exec.CommandContext(ctx, goCmd, append([]string{"test"}, args...)...)
	cmd.Env = environments
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var _ Operation = (*operation)(nil)