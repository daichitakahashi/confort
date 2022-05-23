package confort

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const (
	imageCommunicator = "github.com/daichitakahashi/confort/testdata/communicator:test"
	imageEcho         = "github.com/daichitakahashi/confort/testdata/echo:test"
)

var (
	// generate unique namespace and name for container and network
	uniqueName = UniqueString("name", 16)
)

func TestMain(m *testing.M) {
	var (
		ctx        = context.Background()
		c, cleanup = New()
	)
	defer cleanup()

	g, term := NewGroup(ctx, c, WithNamespace(uniqueName.Must(c)))
	func() {
		defer term()
		c.Logf("building image: %s", imageCommunicator)
		g.Build(ctx, c, &Build{
			Image:      imageCommunicator,
			Dockerfile: "testdata/communicator/Dockerfile",
			ContextDir: "testdata/communicator",
		})
		c.Cleanup(func() {
			c.Logf("remove image: %s", imageCommunicator)
			_, err := g.cli.ImageRemove(ctx, imageCommunicator, types.ImageRemoveOptions{})
			if err != nil {
				c.Logf("failed to remove image %q: %s", imageCommunicator, err)
			}
		})
		c.Logf("building image: %s", imageEcho)
		g.Build(ctx, c, &Build{
			Image:      imageEcho,
			Dockerfile: "testdata/echo/Dockerfile",
			ContextDir: "testdata/echo/",
		})
		c.Cleanup(func() {
			c.Logf("remove image: %s", imageEcho)
			_, err := g.cli.ImageRemove(ctx, imageEcho, types.ImageRemoveOptions{})
			if err != nil {
				c.Logf("failed to remove image %q: %s", imageEcho, err)
			}
		})
	}()

	m.Run()
}

// test network creation and communication between host and container,
// and between containers.
func TestNewGroup(t *testing.T) {
	t.Parallel()

	var (
		ctx     = context.Background()
		network = uniqueName.Must(t)
	)

	g, term := NewGroup(ctx, t,
		WithNamespace(t.Name()),
		WithNetwork(network),
		WithClientOpts(client.FromEnv),
	)
	defer term()

	portsOne := g.Run(ctx, t, "one", &Container{
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "two",
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       Healthy(),
	})
	hostOne, ok := portsOne["80/tcp"]
	if !ok {
		t.Logf("%#v", portsOne)
		t.Fatal("one: bound port not found")
	}

	portsTwo := g.Run(ctx, t, "two", &Container{
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "one",
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       Healthy(),
	})
	hostTwo, ok := portsTwo["80/tcp"]
	if !ok {
		t.Fatal("two: bound port not found")
	}

	request := func(t *testing.T, host, method, status string) string {
		t.Helper()

		resp, err := http.Post(
			fmt.Sprintf("http://%s/%s", host, method),
			"text/plain",
			strings.NewReader(status),
		)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		stat, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("got error response: %d: %s", resp.StatusCode, stat)
		}
		return string(stat)
	}

	// set one's status
	request(t, hostOne, "set", "at home")
	// set two's status
	request(t, hostTwo, "set", "at office")

	// exchange status between one and two using docker network
	request(t, hostOne, "exchange", "")

	// check exchanged one's status
	if s := request(t, hostOne, "get", ""); s != "at office" {
		t.Fatalf("one: expected status is %q, but actual %q", "at office", s)
	}
	// check exchanged
	if s := request(t, hostTwo, "get", ""); s != "at home" {
		t.Fatalf("two: expected status is %q, but actual %q", "at home", s)
	}
}

// test container identification with namespace
func TestNewGroup_Namespace(t *testing.T) {
	t.Parallel()

	var (
		ctx           = context.Background()
		namespace     = uniqueName.Must(t)
		network       = uniqueName.Must(t)
		containerName = uniqueName.Must(t)
		port          = "80/tcp"
	)

	createContainer := func(t *testing.T, namespace, network string) (string, TerminateFunc) {
		t.Helper()

		g, term := NewGroup(ctx, t,
			WithNamespace(namespace),
			WithNetwork(network),
		)
		endpoints := g.Run(ctx, t, containerName, &Container{
			Image:        imageEcho,
			ExposedPorts: []string{port},
			Waiter:       Healthy(),
		})
		endpoint, ok := endpoints[nat.Port(port)]
		if !ok {
			t.Fatalf("cannot get endpoint of %q: %v", port, endpoints)
		}
		return endpoint, term
	}

	expectedEndpoint, term := createContainer(t, namespace, network)
	t.Cleanup(term)

	t.Run(fmt.Sprintf("try to create container %q in same namespace", containerName), func(t *testing.T) {
		t.Parallel()

		network := uniqueName.Must(t)
		actualEndpoint, term := createContainer(t, namespace, network)
		t.Cleanup(term)

		if expectedEndpoint != actualEndpoint {
			t.Fatalf("unexpected endpoint: want %q, got: %q", expectedEndpoint, actualEndpoint)
		}
	})

	t.Run(fmt.Sprintf("try to create container %q in different namespace", containerName), func(t *testing.T) {
		t.Parallel()

		namespace := uniqueName.Must(t)
		network := uniqueName.Must(t)
		actualEndpoint, term := createContainer(t, namespace, network)
		t.Cleanup(term)

		if expectedEndpoint == actualEndpoint {
			t.Fatalf("each endpoint must differ because they are in different namespaces: %q, %q",
				expectedEndpoint, actualEndpoint)
		}
	})
}

// check test fails if container name conflicts between different images
func TestGroup_Run_SameNameButAnotherImage(t *testing.T) {
	t.Parallel()

	var (
		ctx           = context.Background()
		namespace     = uniqueName.Must(t)
		containerName = uniqueName.Must(t)
		/*ctl           = &testutil.Controller{
			TB: t,
			FatalFunc: func(args ...any) {
				t.Helper()
				panic(fmt.Sprint(args...))
			},
		}*/
		ctl, term = New()
	)
	t.Cleanup(term)

	g1, term1 := NewGroup(ctx, t,
		WithNamespace(namespace),
		WithNetwork(uniqueName.Must(t)),
	)
	t.Cleanup(term1)

	recovered := func() (v any) {
		defer func() { v = recover() }()
		g1.Run(ctx, ctl, containerName, &Container{
			Image:        imageEcho,
			ExposedPorts: []string{"80/tcp"},
			Waiter:       Healthy(),
		})
		return
	}()
	if recovered != nil {
		t.Fatalf("unexpected error: %v", recovered)
	}

	test := func(t *testing.T, g *Group) {
		t.Helper()
		recovered := func() (v any) {
			defer func() { v = recover() }()
			g.Run(ctx, ctl, containerName, &Container{ // same name, but different image
				Image: imageCommunicator,
			})
			return
		}()
		if recovered == nil {
			t.Fatal("error expected on run containers that has same name and different image")
		}
		expectedMsg := containerNameConflict(
			fmt.Sprintf("%s-%s", namespace, containerName),
			imageCommunicator,
			imageEcho,
		)
		if recovered != expectedMsg {
			t.Fatalf("unexpected error: %v", recovered)
		}
	}

	t.Run("in the same group", func(t *testing.T) {
		t.Parallel()

		test(t, g1)
	})

	t.Run("across groups", func(t *testing.T) {
		t.Parallel()

		g2, term2 := NewGroup(ctx, t,
			WithNamespace(namespace),
			WithNetwork(uniqueName.Must(t)),
		)
		t.Cleanup(term2)

		test(t, g2)
	})
}
