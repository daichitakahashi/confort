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
	beacon := confort.ConnectBeacon(t, ctx)

	// create container
	cft := confort.New(t, ctx,
		confort.WithNamespace(uuid.NewString(), false),
		confort.WithBeacon(beacon),
	)
	cft.Run(t, ctx, &confort.ContainerParams{
		Name:  "container",
		Image: "alpine:3.16.2",
		Cmd:   []string{"sleep", "infinity"},
	}, confort.WithPullOptions(&types.ImagePullOptions{}, io.Discard))
	t.Log("container is ready")

	time.Sleep(time.Second * 20)
}
