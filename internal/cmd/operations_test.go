package cmd

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	health "google.golang.org/grpc/health/grpc_health_v1"
)

func TestOperation_StartBeaconServer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	op := NewOperation()

	endpoint, done, err := op.StartBeaconServer(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// check if beacon server is started
	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(
		insecure.NewCredentials(),
	))
	if err != nil {
		t.Fatal(err)
	}
	hc := health.NewHealthClient(conn)
	resp, err := hc.Check(ctx, &health.HealthCheckRequest{
		Service: "beacon",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetStatus() != health.HealthCheckResponse_SERVING {
		t.Fatalf("unexpected state: %s", resp.GetStatus())
	}

	err = op.StopBeaconServer(ctx, endpoint)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	// ok
	case <-time.After(time.Second):
		t.Fatal("beacon server haven't shut down correctly")
	}
}

func TestOperation_StopBeaconServer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	op := NewOperation()

	err := op.StopBeaconServer(ctx, "0.0.0.0:0")
	if err == nil {
		t.Fatal("error expected but succeeded")
	}
}

func TestOperation_CleanupResources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	op := NewOperation()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}

	/*
			image := uuid.NewString() + ":latest"

			// create image
			cmd := exec.Command("docker", "build", "-t", image, "-")
			cmd.Stdin = strings.NewReader(`FROM alpine:3.15.4
		LABEL confort="hoge"
		`)
			err := cmd.Run()
			if err != nil {
				t.Fatal(err)
			}
	*/

	// create container
	cmd := exec.Command("docker", "run", "-itd", "--label", "confort=hoge", "ghcr.io/daichitakahashi/confort/testdata/echo:test", "/bin/sh")
	err = cmd.Run()
	if err != nil {
		t.Fatal(err)
	}

	// create network
	cmd = exec.Command("docker", "network", "create", "--label", "confort=hoge", uuid.NewString())
	err = cmd.Run()
	if err != nil {
		t.Fatal(err)
	}

	// do cleanup
	err = op.CleanupResources(ctx, "confort", "hoge")
	if err != nil {
		t.Fatal(err)
	}

	// check
	f := filters.NewArgs(
		filters.Arg("label", "confort=hoge"),
	)

	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
		All:     true,
		Filters: f,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(containers) > 0 {
		t.Error("container is not removed")
	}

	/*
		images, err := op.cli.ImageList(ctx, types.ImageListOptions{
			All:     true,
			Filters: f,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(images) > 0 {
			t.Error("image is not removed")
		}
	*/

	networks, err := cli.NetworkList(ctx, types.NetworkListOptions{
		Filters: f,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(networks) > 0 {
		t.Error("network is not removed")
	}
}

func TestExecuteProcess(t *testing.T) {
	t.Parallel()

	expect := os.Getenv("BEACON_INTEGRATION_EXECUTE_TEST")
	switch expect {
	case "success":
		t.Log(runtime.Version())
	case "fail":
		t.Error("intended error")
	default:
		t.Skip()
	}
}

func TestOperation_ExecuteTest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	op := NewOperation()
	args := []string{"-run", "TestExecuteProcess", "-v"}
	env := os.Environ()

	t.Run("success", func(t *testing.T) {
		err := op.ExecuteTest(ctx, args, append(env, "BEACON_INTEGRATION_EXECUTE_TEST=success"))
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("fail", func(t *testing.T) {
		err := op.ExecuteTest(ctx, args, append(env, "BEACON_INTEGRATION_EXECUTE_TEST=fail"))
		if err == nil {
			t.Fatal("error expected but succeeded")
		}
	})
}
