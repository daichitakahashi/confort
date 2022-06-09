package confort

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/go-cmp/cmp"
)

const (
	imageCommunicator = "github.com/daichitakahashi/confort/testdata/communicator:test"
	imageEcho         = "github.com/daichitakahashi/confort/testdata/echo:test"
)

var (
	// generate unique namespace and name for container
	uniqueName = UniqueString(16)
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	c, cleanup := NewControl()
	defer cleanup()

	cft, term := New(c, ctx, WithNamespace("for-build", false))
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		c.Fatal(err)
	}
	func() {
		defer term()
		c.Logf("building image: %s", imageCommunicator)
		cft.Build(c, ctx, &Build{
			Image:      imageCommunicator,
			Dockerfile: "testdata/communicator/Dockerfile",
			ContextDir: "testdata/communicator",
		}, WithBuildOutput(io.Discard), WithForceBuild())
		c.Cleanup(func() {
			c.Logf("remove image: %s", imageCommunicator)
			_, err := cli.ImageRemove(ctx, imageCommunicator, types.ImageRemoveOptions{})
			if err != nil {
				c.Logf("failed to remove image %q: %s", imageCommunicator, err)
			}
		})
		c.Logf("building image: %s", imageEcho)
		cft.Build(c, ctx, &Build{
			Image:      imageEcho,
			Dockerfile: "testdata/echo/Dockerfile",
			ContextDir: "testdata/echo/",
		}, WithBuildOutput(io.Discard), WithForceBuild())
		c.Cleanup(func() {
			c.Logf("remove image: %s", imageEcho)
			_, err := cli.ImageRemove(ctx, imageEcho, types.ImageRemoveOptions{})
			if err != nil {
				c.Logf("failed to remove image %q: %s", imageEcho, err)
			}
		})
	}()

	m.Run()
}

// test network creation and communication between host and container,
// and between containers.
func TestConfort_Run_Communication(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cft, term := New(t, ctx,
		WithNamespace(t.Name(), false),
	)
	t.Cleanup(term)

	cft.Run(t, ctx, "one", &Container{
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "two",
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       LogContains("communicator is ready", 1),
	})
	portsOne := cft.UseExclusive(t, ctx, "one")
	hostOne, ok := portsOne.Binding("80/tcp")
	if !ok {
		t.Logf("%#v", portsOne)
		t.Fatal("one: bound port not found")
	}

	cft.Run(t, ctx, "two", &Container{
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "one",
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       LogContains("communicator is ready", 1),
	})
	portsTwo := cft.UseExclusive(t, ctx, "two")
	hostTwo, ok := portsTwo.Binding("80/tcp")
	if !ok {
		t.Fatal("two: bound port not found")
	}

	// set one's status
	communicate(t, hostOne, "set", "at home")
	// set two's status
	communicate(t, hostTwo, "set", "at office")

	// exchange status between one and two using docker network
	communicate(t, hostOne, "exchange", "")

	// check exchanged one's status
	if s := communicate(t, hostOne, "get", ""); s != "at office" {
		t.Fatalf("one: expected status is %q, but actual %q", "at office", s)
	}
	// check exchanged
	if s := communicate(t, hostTwo, "get", ""); s != "at home" {
		t.Fatalf("two: expected status is %q, but actual %q", "at home", s)
	}
}

// test container identification with namespace
func TestConfort_Run_ContainerIdentification(t *testing.T) {
	t.Parallel()

	var (
		namespace     = uniqueName.Must(t)
		containerName = uniqueName.Must(t)
		port          = "80/tcp"
	)

	createContainer := func(t *testing.T, namespace string) (string, func()) {
		t.Helper()

		ctx := context.Background()
		cft, term := New(t, ctx, WithNamespace(namespace, true))
		cft.Run(t, ctx, containerName, &Container{
			Image:        imageEcho,
			ExposedPorts: []string{port},
			Waiter:       Healthy(),
		})
		ports := cft.UseShared(t, ctx, containerName)
		endpoint, ok := ports.Binding(nat.Port(port))
		if !ok {
			t.Fatalf("cannot get endpoint of %q: %v", port, ports)
		}
		return endpoint, term
	}

	expectedEndpoint, term := createContainer(t, namespace)
	t.Cleanup(term)

	t.Run(fmt.Sprintf("try to create container %q in same namespace", containerName), func(t *testing.T) {
		t.Parallel()

		actualEndpoint, term := createContainer(t, namespace)
		t.Cleanup(term)

		if expectedEndpoint != actualEndpoint {
			t.Fatalf("unexpected endpoint: want %q, got: %q", expectedEndpoint, actualEndpoint)
		}
	})

	t.Run(fmt.Sprintf("try to create container %q in different namespace", containerName), func(t *testing.T) {
		t.Parallel()

		namespace := uniqueName.Must(t)
		actualEndpoint, term := createContainer(t, namespace)
		t.Cleanup(term)

		if expectedEndpoint == actualEndpoint {
			t.Fatalf("each endpoint must differ because they are in different namespaces: %q, %q",
				expectedEndpoint, actualEndpoint)
		}
	})
}

// check test fails if container name conflicts between different images
func TestConfort_Run_SameNameButAnotherImage(t *testing.T) {
	t.Parallel()

	var (
		ctx           = context.Background()
		namespace     = uniqueName.Must(t)
		containerName = uniqueName.Must(t)
		ctl, term     = NewControl()
	)
	t.Cleanup(term)

	cft1, term1 := New(t, ctx,
		WithNamespace(namespace, true),
	)
	t.Cleanup(term1)

	recovered := func() (v any) {
		defer func() { v = recover() }()
		cft1.Run(ctl, ctx, containerName, &Container{
			Image:        imageEcho,
			ExposedPorts: []string{"80/tcp"},
			Waiter:       Healthy(),
		})
		return
	}()
	if recovered != nil {
		t.Fatalf("unexpected error: %v", recovered)
	}

	cft2, term2 := New(t, ctx,
		WithNamespace(namespace, true),
	)
	t.Cleanup(term2)

	recovered = func() (v any) {
		defer func() { v = recover() }()
		cft2.Run(ctl, ctx, containerName, &Container{ // same name, but different image
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
	if !strings.Contains(fmt.Sprint(recovered), expectedMsg) {
		t.Fatalf("unexpected error: %v", recovered)
	}
}

// test LazyRun
func TestConfort_LazyRun(t *testing.T) {
	t.Parallel()

	var (
		ctx       = context.Background()
		namespace = uniqueName.Must(t)
	)

	cft, term := New(t, ctx,
		WithNamespace(namespace, true),
	)
	t.Cleanup(term)

	t.Run("Use after LazyRun", func(t *testing.T) {
		t.Parallel()

		containerName := uniqueName.Must(t)

		cft.LazyRun(t, ctx, containerName, &Container{
			Image:        imageEcho,
			ExposedPorts: []string{"80/tcp"},
			Waiter:       Healthy(),
		})

		e1 := cft.UseShared(t, ctx, containerName)
		e2 := cft.UseShared(t, ctx, containerName)
		if diff := cmp.Diff(e1, e2); diff != "" {
			t.Fatal(diff)
		}
		endpoint, ok := e1.Binding("80/tcp")
		if !ok {
			t.Fatal("endpoint not found")
		}
		assertEchoWorks(t, endpoint)
	})

	t.Run("Run after LazyRun from another instance", func(t *testing.T) {
		t.Parallel()

		containerName := uniqueName.Must(t)

		c := &Container{
			Image:        imageEcho,
			ExposedPorts: []string{"80/tcp"},
			Waiter:       Healthy(),
		}

		cft.LazyRun(t, ctx, containerName, c)
		e1 := cft.UseShared(t, ctx, containerName)

		cft2, term := New(t, ctx,
			WithNamespace(namespace, true),
		)
		t.Cleanup(term)

		cft2.Run(t, ctx, containerName, c)
		e2 := cft2.UseShared(t, ctx, containerName)
		if diff := cmp.Diff(e1, e2); diff != "" {
			t.Fatal(diff)
		}

		endpoint, ok := e1.Binding("80/tcp")
		if !ok {
			t.Fatal("endpoint not found")
		}
		assertEchoWorks(t, endpoint)
	})

	t.Run("unknown Use", func(t *testing.T) {
		t.Parallel()

		containerName := uniqueName.Must(t)

		cft.LazyRun(t, ctx, containerName, &Container{
			Image:        imageEcho,
			ExposedPorts: []string{"80/tcp"},
			Waiter:       Healthy(),
		})

		cft2, term := New(t, ctx,
			WithNamespace(namespace, true),
		)
		t.Cleanup(term)

		ctl, _ := NewControl()

		recovered := func() (v any) {
			defer func() { v = recover() }()
			cft2.UseShared(ctl, ctx, containerName)
			return
		}()
		if recovered == nil {
			t.Fatal("error expected on use container without LazyRun")
		}
		expectedMsg := containerNotFound(
			fmt.Sprintf("%s-%s", namespace, containerName),
		)
		if !strings.Contains(fmt.Sprint(recovered), expectedMsg) {
			t.Fatalf("unexpected error: %v", recovered)
		}
	})
}

// test if container can join different networks simultaneously
func TestConfort_Run_AttachAliasToAnotherNetwork(t *testing.T) {
	t.Parallel()

	var (
		ctx        = context.Background()
		namespaceA = "namespace"
		namespaceB = "namespace-foo"
	)

	// Network1 ┳ Container "namespace-foo-A"
	//          ┗ Container "namespace-foo-B" ┳　Network2
	//            Container "namespace-foo-C" ┛

	cft1, term1 := New(t, ctx,
		WithNamespace(namespaceA, true),
	)
	t.Cleanup(term1)

	cft1.Run(t, ctx, "foo-A", &Container{
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "foo-B",
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       Healthy(),
	})
	e := cft1.UseShared(t, ctx, "foo-A")
	hostA, ok := e.Binding("80/tcp")
	if !ok {
		t.Fatal("failed to get host/port")
	}

	cft1.Run(t, ctx, "foo-B", &Container{
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "C",
		},
		// Using ephemeral port makes test flaky, why?
		// Without specifying host port, container loses the port binding occasionally.
		ExposedPorts: []string{"8080:80/tcp"},
		Waiter:       Healthy(),
	})
	e = cft1.UseShared(t, ctx, "foo-B")
	hostB, ok := e.Binding("80/tcp")
	if !ok {
		t.Fatal("failed to get host/port")
	}

	cft2, term2 := New(t, ctx,
		WithNamespace(namespaceB, true),
	)
	t.Cleanup(term2)

	cft2.Run(t, ctx, "B", &Container{ // same name container
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "C",
		},
		ExposedPorts: []string{"8080:80/tcp"},
		Waiter:       Healthy(),
	})
	e = cft2.UseShared(t, ctx, "B")
	hostB2, ok := e.Binding("80/tcp")
	if !ok {
		t.Fatal("failed to get host/port")
	}
	if hostB != hostB2 {
		t.Fatalf("expected same host: want %q, got %q", hostB, hostB2)
	}

	cft2.Run(t, ctx, "C", &Container{
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "B", // CHECK THIS WORKS
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       Healthy(),
	})
	e = cft2.UseShared(t, ctx, "C")
	hostC, ok := e.Binding("80/tcp")
	if !ok {
		t.Fatal("failed to get host/port")
	}

	// set initial values
	// Container "namespace-foo-A" => 1
	// Container "namespace-foo-B" => 2
	// Container "namespace-foo-C" => 3
	communicate(t, hostA, "set", "1")
	communicate(t, hostB, "set", "2")
	communicate(t, hostC, "set", "3")

	// exchange values
	// Container "namespace-foo-A" => 1 ┓ 1.exchange
	// Container "namespace-foo-B" => 2 ┛ ┓
	// Container "namespace-foo-C" => 3   ┛ 2.exchange
	communicate(t, hostA, "exchange", "")
	communicate(t, hostC, "exchange", "")

	// check all values
	a := communicate(t, hostA, "get", "")
	b := communicate(t, hostB, "get", "")
	c := communicate(t, hostC, "get", "")
	if !(a == "2" && b == "3" && c == "1") {
		t.Fatalf("unexpected result: a=%q, b=%q, c=%q", a, b, c)
	}
}

func communicate(t *testing.T, host, method, status string) string {
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

func TestWithResourcePolicy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}

	// define assertions

	// container or network is reused
	assertReused := func(t *testing.T, precedingID, id string, recovered any) {
		t.Helper()
		if recovered != nil {
			t.Fatalf("unexpected error: %#v", recovered)
		}
		if precedingID != id {
			t.Fatalf("ids are not match: %q and %q", precedingID, id)
		}
	}
	// create network or container is failed because network/container having same name already exists
	assertFailed := func(t *testing.T, _, _ string, recovered any) {
		t.Helper()
		if recovered == nil {
			t.Fatal("expected failure, but succeeds")
		}
	}
	// after termination, check the network is still alive or not
	assertNetwork := func(removed bool) func(t *testing.T, networkID string) {
		return func(t *testing.T, networkID string) {
			t.Helper()
			list, err := cli.NetworkList(ctx, types.NetworkListOptions{})
			if err != nil {
				t.Fatal(err)
			}
			var found bool
			for _, c := range list {
				if c.ID == networkID {
					found = true
					break
				}
			}
			if found && removed {
				t.Fatalf("expected to be removed but exists: %q", networkID)
			} else if !found && !removed {
				t.Fatalf("network not found: %q", networkID)
			}
		}
	}
	// after termination, check the container is still alive or not
	assertContainer := func(removed bool) func(t *testing.T, containerID string) {
		return func(t *testing.T, containerID string) {
			t.Helper()
			list, err := cli.ContainerList(ctx, types.ContainerListOptions{
				All: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			var found bool
			for _, c := range list {
				if c.ID == containerID {
					found = true
					break
				}
			}
			if found && removed {
				t.Fatalf("expected to be removed but exists: %q", containerID)
			} else if !found && !removed {
				t.Fatalf("container not found: %q", containerID)
			}
		}
	}

	testCases := []struct {
		policy                   ResourcePolicy
		afterNamespaceCreated    func(t *testing.T, foundNetworkID, gotNetworkID string, recovered any)
		afterNamespaceTerminated func(t *testing.T, networkID string)
		afterContainerCreated    func(t *testing.T, foundContainerID, gotContainerID string, recovered any)
		afterContainerTerminated func(t *testing.T, containerID string)
	}{
		{
			policy:                   ResourcePolicyReuse,
			afterNamespaceCreated:    assertReused,
			afterNamespaceTerminated: assertNetwork(false),
			afterContainerCreated:    assertReused,
			afterContainerTerminated: assertContainer(false),
		},
		{
			policy:                   ResourcePolicyTakeOver,
			afterNamespaceCreated:    assertReused,
			afterNamespaceTerminated: assertNetwork(true),
			afterContainerCreated:    assertReused,
			afterContainerTerminated: assertContainer(true),
		},
		{
			policy:                   ResourcePolicyError,
			afterNamespaceCreated:    assertFailed,
			afterNamespaceTerminated: assertNetwork(false),
			afterContainerCreated:    assertFailed,
			afterContainerTerminated: assertContainer(false),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(string(tc.policy), func(t *testing.T) {
			t.Parallel()

			t.Run("duplicated namespace(network)", func(t *testing.T) {
				t.Parallel()

				// create preceding network
				networkName := uniqueName.Must(t)
				precedes, termPrecedes := New(t, ctx, WithNamespace(networkName, true))
				t.Cleanup(termPrecedes)

				// try to re-create
				var cft *Confort
				var term func()
				var terminated bool
				var networkID string
				recovered := func() (r any) {
					defer func() {
						r = recover()
					}()
					c, _ := NewControl()
					cft, term = New(c, ctx,
						WithNamespace(networkName, true),
						WithResourcePolicy(tc.policy),
					)
					if cft != nil && cft.namespace != nil {
						networkID = cft.namespace.Network().ID
					}
					return nil
				}()
				t.Cleanup(func() {
					if !terminated && term != nil {
						term()
					}
				})
				tc.afterNamespaceCreated(t, precedes.namespace.Network().ID, networkID, recovered)
				if term != nil {
					term()
					terminated = true
				}
				tc.afterNamespaceTerminated(t, precedes.namespace.Network().ID)
			})

			t.Run("duplicated container name", func(t *testing.T) {
				t.Parallel()

				// create preceding network and container
				namespacePrefix := uniqueName.Must(t)
				middleName := uniqueName.Must(t)
				containerNameSuffix := uniqueName.Must(t)
				precedes, termPrecedes := New(t, ctx,
					WithNamespace(fmt.Sprintf("%s-%s", namespacePrefix, middleName), true),
				)
				t.Cleanup(termPrecedes)

				precedingContainerID, err := precedes.namespace.CreateContainer(ctx, containerNameSuffix, &container.Config{
					Image: imageEcho,
				}, &container.HostConfig{}, &network.NetworkingConfig{}, true, nil, io.Discard)
				if err != nil {
					t.Fatal(err)
				}

				// try to re-create
				var cft *Confort
				var term func()
				var terminated bool
				var containerID string
				recovered := func() (r any) {
					defer func() {
						r = recover()
					}()
					c, _ := NewControl()
					cft, term = New(c, ctx,
						WithNamespace(namespacePrefix, true),
						WithResourcePolicy(tc.policy),
					)
					containerID, err = cft.namespace.CreateContainer(ctx, fmt.Sprintf("%s-%s", middleName, containerNameSuffix), &container.Config{
						Image: imageEcho,
					}, &container.HostConfig{}, &network.NetworkingConfig{}, true, nil, io.Discard)
					if err != nil {
						c.Fatal(err)
					}
					return nil
				}()
				t.Cleanup(func() {
					if !terminated {
						term()
					}
				})
				tc.afterContainerCreated(t, precedingContainerID, containerID, recovered)
				term()
				terminated = true
				tc.afterContainerTerminated(t, precedingContainerID)
			})
		})
	}
}

func TestWithPullOptions(t *testing.T) {
	t.Parallel()

	const pullImage = "ghcr.io/daichitakahashi/confort/testdata/echo:test"

	ctx := context.Background()
	namespace := uniqueName.Must(t)
	containerName := uniqueName.Must(t)

	cft, term := New(t, ctx,
		WithNamespace(namespace, true),
	)
	t.Cleanup(func() {
		if t.Failed() {
			term()
		}
	})

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}

	// remove target image if already exists
	_ = removeImageIfExists(t, cli, pullImage)

	// pull and run
	out := &bytes.Buffer{}
	cft.Run(t, ctx, containerName, &Container{
		Image:        pullImage,
		ExposedPorts: []string{"80/tcp"},
		Waiter:       Healthy(),
	}, WithPullOptions(&types.ImagePullOptions{}, out))

	t.Log(out.String())
	if out.Len() == 0 {
		t.Fatal("pull image not performed")
	}

	// check if container works
	ports := cft.UseShared(t, ctx, containerName)
	endpoint, ok := ports.Binding("80/tcp")
	if !ok {
		t.Fatal("endpoint not found")
	}
	assertEchoWorks(t, endpoint)

	// remove container
	term()

	// remove pulled image
	removed := removeImageIfExists(t, cli, pullImage)
	if !removed {
		t.Fatalf("cannot remove pulled image %q", pullImage)
	}
}

func removeImageIfExists(t *testing.T, cli *client.Client, image string) (removed bool) {
	t.Helper()
	ctx := context.Background()

	images, err := cli.ImageList(ctx, types.ImageListOptions{
		All: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
find:
	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == image {
				found = true
				break find
			}
		}
	}
	if found {
		_, err = cli.ImageRemove(ctx, image, types.ImageRemoveOptions{
			Force: false,
		})
		if err != nil {
			t.Fatal(err)
		}
		return true
	}
	return false
}

func assertEchoWorks(t *testing.T, endpoint string) {
	t.Helper()

	resp, err := http.Post("http://"+endpoint, "text/plain", strings.NewReader("ping"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(response) != "ping" {
		t.Fatalf("unexpected echo response: %q", response)
	}
}

func TestConfort_Run_UnsupportedStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	namespace := uniqueName.Must(t)
	containerName := namespace + "-" + "foo"

	cft, term := New(t, ctx,
		WithNamespace(namespace, true),
	)
	t.Cleanup(term)

	// start container
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	created, err := cli.ContainerCreate(ctx, &container.Config{
		Image: imageEcho,
	}, &container.HostConfig{}, &network.NetworkingConfig{}, nil, containerName)
	if err != nil {
		t.Fatal(err)
	}
	err = cli.ContainerStart(ctx, created.ID, types.ContainerStartOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cli.ContainerRemove(ctx, created.ID, types.ContainerRemoveOptions{
			RemoveVolumes: true,
			Force:         true,
		})
	})

	tryRun := func() (r any) {
		defer func() {
			r = recover()
		}()
		c, _ := NewControl()
		cft.Run(c, ctx, "foo", &Container{
			Image: imageEcho,
		})
		return nil
	}

	// unsupported container status "pause"
	err = cli.ContainerPause(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	recovered := tryRun()
	if recovered == nil {
		t.Fatal("unexpected success")
	}

	// unsupported container status "exited"
	err = cli.ContainerStop(ctx, created.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	recovered = tryRun()
	if recovered == nil {
		t.Fatal("unexpected success")
	}
}
