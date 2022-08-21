package confort_test

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/beaconserver"
	"github.com/daichitakahashi/confort/internal/beaconutil"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"google.golang.org/grpc"
)

func TestConnectBeacon(t *testing.T) {
	ctx := context.Background()

	// start beacon server
	srv := grpc.NewServer()
	beaconserver.Register(srv, func() error {
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
	err = beaconutil.StoreAddressToLockFile(lockFile, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(beaconutil.LockFileEnv, lockFile)

	beacon := confort.ConnectBeacon(t, ctx)
	if !beacon.Enabled() {
		t.Fatal("failed to connect beacon server")
	}

	t.Run("confort", func(t *testing.T) {
		t.Parallel()

		namespace := strings.ReplaceAll(t.Name(), "/", "_")
		cft, shutdown := confort.New(t, ctx,
			confort.WithBeacon(beacon),
			confort.WithNamespace(namespace, true),
		)
		var done bool
		t.Cleanup(func() {
			if !done {
				shutdown()
				return
			}
		})
		cft.Run(t, ctx, "tester", &confort.Container{
			Image: "github.com/daichitakahashi/confort/testdata/echo:test",
		})
		shutdown()
		done = true

		// when beacon server is enabled, container is not deleted after shutdown
		containerName := namespace + "-tester"
		cli, err := client.NewClientWithOpts(client.FromEnv)
		if err != nil {
			t.Fatal(err)
		}
		f := filters.NewArgs()
		f.Add("name", containerName)
		list, err := cli.ContainerList(ctx, types.ContainerListOptions{
			All:     true,
			Filters: f,
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range list {
			err = cli.ContainerRemove(ctx, c.ID, types.ContainerRemoveOptions{
				RemoveVolumes: true,
				Force:         true,
			})
			if err != nil {
				t.Fatal(err)
			}
			return
		}
		t.Fatalf("container %q not found", containerName)
	})

	t.Run("unique", func(t *testing.T) {
		t.Parallel()

		unique := confort.NewUnique(func() (bool, error) {
			return true, nil
		}, confort.WithGlobalUniqueness(beacon, t.Name()))

		if !unique.Global() {
			t.Fatal("integration with beacon is nor enabled properly")
		}

		_, err := unique.New()
		if err != nil {
			t.Fatal(err)
		}
		_, err = unique.New()
		if err == nil {
			t.Fatal("unexpected success")
		}
	})
}
