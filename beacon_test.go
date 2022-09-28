package confort_test

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/internal/beacon/server"
	"github.com/daichitakahashi/confort/internal/beacon/util"
	"github.com/daichitakahashi/confort/unique"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"google.golang.org/grpc"
)

func TestConnectBeacon(t *testing.T) {
	ctx := context.Background()

	// start beacon server
	srv := grpc.NewServer()
	server.Register(srv, func() error {
		return nil
	})
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = srv.Serve(ln)
		_ = ln.Close()
	}()
	t.Cleanup(func() {
		srv.Stop()
	})

	// write lock file
	lockFile := filepath.Join(t.TempDir(), "lock")
	err = util.StoreAddressToLockFile(lockFile, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(util.LockFileEnv, lockFile)

	beacon := confort.ConnectBeacon(t, ctx)
	if !beacon.Enabled() {
		t.Fatal("failed to connect beacon server")
	}

	t.Run("confort", func(t *testing.T) {
		t.Parallel()

		var term func()
		namespace := strings.ReplaceAll(t.Name(), "/", "_")
		cft := confort.New(t, ctx,
			confort.WithBeacon(beacon),
			confort.WithNamespace(namespace, true),
			confort.WithTerminateFunc(&term),
		)
		var done bool
		t.Cleanup(func() {
			if !done {
				term()
				return
			}
		})
		cft.Run(t, ctx, &confort.ContainerParams{
			Name:  "tester",
			Image: "github.com/daichitakahashi/confort/testdata/echo:test",
		})
		term()
		done = true

		// when beacon server is enabled, network and container is not deleted after termination
		cli, err := client.NewClientWithOpts(client.FromEnv)
		if err != nil {
			t.Fatal(err)
		}

		containerName := namespace + "-tester"
		containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
			All: true,
			Filters: filters.NewArgs(
				filters.Arg("name", containerName),
			),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(containers) == 0 {
			t.Fatalf("container %q not found", containerName)
		}
		for _, container := range containers {
			err = cli.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{
				RemoveVolumes: true,
				Force:         true,
			})
			if err != nil {
				t.Fatal(err)
			}
		}

		networks, err := cli.NetworkList(ctx, types.NetworkListOptions{
			Filters: filters.NewArgs(
				filters.Arg("name", namespace),
			),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(networks) == 0 {
			t.Fatalf("network %q not found", namespace)
		}
		for _, network := range networks {
			err = cli.NetworkRemove(ctx, network.ID)
			if err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("unique", func(t *testing.T) {
		t.Parallel()

		uniq := unique.New(func() (bool, error) {
			return true, nil
		}, unique.WithBeacon(beacon, t.Name()))

		_, err := uniq.New()
		if err != nil {
			t.Fatal(err)
		}
		_, err = uniq.New()
		if err == nil {
			t.Fatal("unexpected success")
		}
	})
}
