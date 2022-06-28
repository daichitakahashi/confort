package integration

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	health "google.golang.org/grpc/health/grpc_health_v1"
)

// const image = "ghcr.io/daichitakahashi/confort/beacon:latest"
const image = "beacon:dev"

func TestMain(m *testing.M) {
	m.Run()

	ctx := context.Background()
	cli := initClient()
	images, err := cli.ImageList(ctx, types.ImageListOptions{
		All: true,
	})
	if err != nil {
		panic(err)
	}

	var imageID string
find:
	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == image {
				// imageID = img.ID // do not remove currently
				break find
			}
		}
	}
	if imageID == "" {
		return
	}
	_, err = cli.ImageRemove(ctx, imageID, types.ImageRemoveOptions{
		Force: true,
	})
	if err != nil {
		panic(err)
	}
}

func initClient() *client.Client {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}
	return cli
}

func TestOperation_StartBeaconServer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	op := &operation{
		cli: initClient(),
	}

	endpoint, err := op.StartBeaconServer(ctx, image)
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
}

func TestOperation_StopBeaconServer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	op := &operation{
		cli: initClient(),
	}

	err := op.StopBeaconServer(ctx, "0.0.0.0:0")
	if err == nil {
		t.Fatal("error expected but succeeded")
	}
}
