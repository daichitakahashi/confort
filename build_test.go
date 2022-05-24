package confort

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// test BuildAndRun
func TestGroup_BuildAndRun(t *testing.T) {
	t.Parallel()

	var (
		ctx           = context.Background()
		namespace     = uniqueName.Must(t)
		network       = uniqueName.Must(t)
		containerName = uniqueName.Must(t)
	)

	g, term := NewGroup(ctx, t,
		WithNamespace(namespace),
		WithNetwork(network),
	)
	t.Cleanup(term)

	b := &Build{
		Image:        imageEcho,
		Dockerfile:   "testdata/echo/Dockerfile",
		ContextDir:   "testdata/echo",
		ExposedPorts: []string{"80/tcp"},
		Waiter:       Healthy(),
	}

	// image is already built on TestMain
	e1 := g.BuildAndRun(ctx, t, containerName, b, WithSkipIfAlreadyExists())
	if _, ok := e1["80/tcp"]; !ok {
		t.Fatal("endpoint not found")
	}

	// reuse running container
	e2 := g.BuildAndRun(ctx, t, containerName, b, WithSkipIfAlreadyExists())
	if diff := cmp.Diff(e1, e2); diff != "" {
		t.Fatal(diff)
	}
}
