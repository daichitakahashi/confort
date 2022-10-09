//go:build interrupt

package tests

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/daichitakahashi/confort"
	"github.com/docker/docker/api/types"
	"github.com/google/uuid"
)

func TestInterrupt(t *testing.T) {
	ctx := context.Background()

	t.Log("connecting beacon server")

	// create container
	cft, err := confort.New(ctx,
		confort.WithNamespace(uuid.NewString(), false),
		confort.WithBeacon(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft.Close()
	})
	_, err = cft.Run(ctx, &confort.ContainerParams{
		Name:  "container",
		Image: "alpine:3.16.2",
		Cmd:   []string{"sleep", "infinity"},
	}, confort.WithPullOptions(&types.ImagePullOptions{}, io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	t.Log("container is ready")

	time.Sleep(time.Second * 20)
}
