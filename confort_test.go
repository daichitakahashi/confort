package confort

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"
	"testing"
	"time"

	"github.com/daichitakahashi/confort/internal/beaconutil"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

const (
	imageCommunicator = "github.com/daichitakahashi/confort/testdata/communicator:test"
	imageEcho         = "github.com/daichitakahashi/confort/testdata/echo:test"
	imageLs           = "github.com/daichitakahashi/confort/testdata/ls:"
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
		c.Cleanup(func() {
			_, err := cli.ImagesPrune(ctx, filters.NewArgs(
				filters.Arg("dangling", "true"),
			))
			if err != nil {
				c.Logf("prune dangling images failed: %s", err)
			}
		})
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
	hostOne := portsOne.HostPort("80/tcp")
	if hostOne == "" {
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
	hostTwo := portsTwo.HostPort("80/tcp")
	if hostTwo == "" {
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
		endpoint := ports.HostPort(nat.Port(port))
		if endpoint == "" {
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
		endpoint := e1.HostPort("80/tcp")
		if endpoint == "" {
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

		endpoint := e1.HostPort("80/tcp")
		if endpoint == "" {
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
	hostA := e.HostPort("80/tcp")
	if hostA == "" {
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
	hostB := e.HostPort("80/tcp")
	if hostB == "" {
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
	hostB2 := e.HostPort("80/tcp")
	if hostB2 == "" {
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
	hostC := e.HostPort("80/tcp")
	if hostC == "" {
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

func TestWithClientOptions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	c, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}

	// wrap transport
	logOut := bytes.NewBuffer(nil)
	httpCli := c.HTTPClient()
	transport := httpCli.Transport
	httpCli.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		resp, err := transport.RoundTrip(r)
		if err != nil {
			return nil, err
		}
		dump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			return nil, err
		}
		logOut.Write(dump)
		logOut.WriteByte('\n')
		return resp, err
	})

	_, term := New(t, ctx,
		WithClientOptions(client.FromEnv, client.WithHTTPClient(httpCli)),
		WithNamespace(uuid.NewString(), true),
	)
	term()

	if logOut.Len() == 0 {
		t.Fatal("no log output")
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestWithNamespace(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		desc              string
		envNamespace      string
		optNamespace      string
		force             bool
		expectedNamespace string
	}{
		{
			desc:              "without env or enforcement",
			envNamespace:      "",
			optNamespace:      "opt-namespace",
			expectedNamespace: "opt-namespace-",
		}, {
			desc:              "with env and no enforcement",
			envNamespace:      "env-namespace",
			optNamespace:      "opt-namespace",
			expectedNamespace: "env-namespace-",
		}, {
			desc:              "without env and with enforcement",
			envNamespace:      "",
			optNamespace:      "opt-namespace",
			force:             true,
			expectedNamespace: "opt-namespace-",
		}, {
			desc:              "with env and enforcement",
			envNamespace:      "env-namespace",
			optNamespace:      "opt-namespace",
			force:             true,
			expectedNamespace: "opt-namespace-",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			if tc.envNamespace != "" {
				t.Setenv(beaconutil.NamespaceEnv, tc.envNamespace)
			}
			cft, term := New(t, ctx, WithNamespace(tc.optNamespace, tc.force))
			t.Cleanup(term)

			actual := cft.Namespace()
			if tc.expectedNamespace != actual {
				t.Fatalf("expected namespace %q, got %q", tc.expectedNamespace, actual)
			}
		})
	}
}

func TestWithNamespace_empty(t *testing.T) {
	t.Parallel()

	c, cleanup := NewControl()
	t.Cleanup(cleanup)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected to fail, but succeeded")
		}
	}()
	_, term := New(c, context.Background(),
		WithNamespace("", true),
	)
	t.Cleanup(term)
}

func TestWithDefaultTimeout(t *testing.T) {
	t.Parallel()

	// test timeout for Docker API request
	httpClient := func(fn func(deadline time.Time, ok bool)) *http.Client {
		cli, err := client.NewClientWithOpts(client.FromEnv)
		if err != nil {
			t.Fatal(err)
		}
		httpCli := cli.HTTPClient()
		transport := httpCli.Transport

		var tested bool
		httpCli.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
			ctx := r.Context()
			// test once
			if !tested {
				deadline, ok := ctx.Deadline()
				fn(deadline, ok)
				tested = true
			}
			return transport.RoundTrip(r)
		})
		return httpCli
	}

	testCases := []struct {
		desc    string
		timeout time.Duration
		newCtx  func() (context.Context, context.CancelFunc)
		test    func(t *testing.T, deadline time.Time, ok bool)
	}{
		{
			desc:    "default default timeout(1 min.)",
			timeout: -1, // without WithDefaultTimeout
			test: func(t *testing.T, deadline time.Time, ok bool) {
				if !ok {
					t.Fatal("deadline expected")
				}
				d := time.Until(deadline)
				if d < time.Second*59 || time.Minute < d {
					t.Fatalf("expected timeout is more than 59 sec. and less than 1 min., actual %s", d)
				}
			},
		}, {
			desc:    "no timeout",
			timeout: 0, // WithDefaultTimeout(0)
			test: func(t *testing.T, deadline time.Time, ok bool) {
				if ok {
					t.Fatal("no deadline expected")
				}
			},
		}, {
			desc:    "with default timeout(5 sec.)",
			timeout: time.Second * 5, // WithDefaultTimeout(time.Second*5)
			test: func(t *testing.T, deadline time.Time, ok bool) {
				if !ok {
					t.Fatal("deadline expected")
				}
				d := time.Until(deadline)
				if d < time.Second*4 || time.Second*5 < d {
					t.Fatalf("expected timeout is more than 4 min., and less than 5 min., actual %s", d)
				}
			},
		}, {
			desc:    "with default timeout(5 sec.) and timeout for New(3 sec.)",
			timeout: time.Second * 5, // WithDefaultTimeout(time.Second*5)
			newCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), time.Second*3)
			},
			test: func(t *testing.T, deadline time.Time, ok bool) {
				if !ok {
					t.Fatal("deadline expected")
				}
				d := time.Until(deadline)
				if d < time.Second*2 || time.Second*3 < d {
					t.Fatalf("expected timeout is more than 2 sec., and less than 3 sec., actual %s", d)
				}
			},
		}, {
			desc:    "with default timeout(5 sec.) and timeout for New(7 sec.)",
			timeout: time.Second * 5, // WithDefaultTimeout(time.Second*5)
			newCtx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), time.Second*7)
			},
			test: func(t *testing.T, deadline time.Time, ok bool) {
				if !ok {
					t.Fatal("deadline expected")
				}
				d := time.Until(deadline)
				if d < time.Second*6 || time.Second*7 < d {
					t.Fatalf("expected timeout is more than 6 sec., and less than 7 sec., actual %s", d)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			httpCli := httpClient(func(deadline time.Time, ok bool) {
				tc.test(t, deadline, ok)
			})
			opts := []NewOption{
				WithNamespace(uuid.NewString(), true),
				WithClientOptions(client.FromEnv, client.WithHTTPClient(httpCli)),
			}
			if tc.timeout >= 0 {
				opts = append(opts, WithDefaultTimeout(tc.timeout))
			}

			ctx := context.Background()
			if tc.newCtx != nil {
				var cancel context.CancelFunc
				ctx, cancel = tc.newCtx()
				t.Cleanup(cancel)
			}
			_, term := New(t, ctx, opts...)
			t.Cleanup(term)
		})
	}
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

				precedingContainerID, err := precedes.namespace.CreateContainer(ctx, precedes.namespace.Namespace()+containerNameSuffix, &container.Config{
					Image: imageEcho,
				}, &container.HostConfig{}, &network.NetworkingConfig{}, true, nil, nil, io.Discard)
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
					containerID, err = cft.namespace.CreateContainer(ctx, fmt.Sprintf("%s%s-%s", cft.namespace.Namespace(), middleName, containerNameSuffix), &container.Config{
						Image: imageEcho,
					}, &container.HostConfig{}, &network.NetworkingConfig{}, true, nil, nil, io.Discard)
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

func TestWithResourcePolicy_invalid(t *testing.T) {

	t.Run("invalid policy from env", func(t *testing.T) {
		t.Setenv(beaconutil.ResourcePolicyEnv, "invalid")

		c, cleanup := NewControl()
		t.Cleanup(cleanup)

		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected to fail, but succeeded")
			}
		}()
		_, term := New(c, context.Background(),
			WithNamespace(uuid.NewString(), true),
		)
		t.Cleanup(term)
	})

	t.Run("invalid policy from WithResourcePolicy", func(t *testing.T) {
		c, cleanup := NewControl()
		t.Cleanup(cleanup)

		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected to fail, but succeeded")
			}
		}()
		_, term := New(c, context.Background(),
			WithNamespace(uuid.NewString(), true),
			WithResourcePolicy("invalid"),
		)
		t.Cleanup(term)
	})
}

func TestWithImageBuildOptions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cft, term := New(t, ctx,
		WithNamespace(t.Name(), true),
	)
	t.Cleanup(term)

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}

	tag := "label"
	build := &Build{
		Image:      imageLs + tag,
		Dockerfile: "testdata/ls/Dockerfile",
		ContextDir: "testdata/ls",
		BuildArgs: map[string]*string{
			"ID": &tag,
		},
	}

	var (
		label      = "daichitakahashi.confort.test"
		labelValue = t.Name()
	)

	// build labeled image
	cft.Build(t, ctx, build,
		WithBuildOutput(io.Discard),
		WithImageBuildOptions(func(option *types.ImageBuildOptions) {
			if option.Labels == nil {
				option.Labels = map[string]string{}
			}
			option.Labels[label] = labelValue
		}),
	)
	t.Cleanup(func() {
		removeImageIfExists(t, cli, build.Image)
	})

	// check the labeled image exists
	list, err := cli.ImageList(ctx, types.ImageListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s=%s", label, labelValue)),
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatalf(`there is no image labeled "%s=%s"`, label, labelValue)
	}
}

func TestWithForceBuild_WithBuildOutput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cft, term := New(t, ctx,
		WithNamespace(t.Name(), true),
	)
	t.Cleanup(term)

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}

	tag := "force"
	build := &Build{
		Image:      imageLs + tag,
		Dockerfile: "testdata/ls/Dockerfile",
		ContextDir: "testdata/ls",
		BuildArgs: map[string]*string{
			"ID": &tag,
		},
	}

	// build once
	cft.Build(t, ctx, build,
		WithForceBuild(),
		WithBuildOutput(io.Discard),
	)
	t.Cleanup(func() {
		removeImageIfExists(t, cli, build.Image)
	})

	buf := bytes.NewBuffer(nil)

	// force build
	cft.Build(t, ctx, build,
		WithForceBuild(),
		WithBuildOutput(buf),
	)
	if buf.Len() == 0 {
		t.Fatal("expected build log to be written to buf, but got no output")
	}
	buf.Reset()

	// build if the image not exists
	cft.Build(t, ctx, build,
		WithBuildOutput(buf),
	)
	if buf.Len() > 0 {
		t.Error("expected build to be skipped, but build log is written")
		t.Log(buf.String())
	}
}

func TestWithContainerConfig(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}

	cft, term := New(t, ctx,
		WithNamespace(t.Name(), true),
	)
	t.Cleanup(term)

	var (
		label      = "daichitakahashi.confort.test"
		labelValue = t.Name()
	)

	cft.Run(t, ctx, "echo", &Container{
		Image: imageEcho,
	}, WithContainerConfig(func(config *container.Config) {
		if config.Labels == nil {
			config.Labels = map[string]string{}
		}
		config.Labels[label] = labelValue
	}))

	// check the labeled container exists
	list, err := cli.ContainerList(ctx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s=%s", label, labelValue)),
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatalf(`there is no container labeled "%s=%s"`, label, labelValue)
	}
}

func TestWithHostConfig(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cft, term := New(t, ctx,
		WithNamespace(t.Name(), true),
	)
	t.Cleanup(term)

	cft.Run(t, ctx, "communicator", &Container{
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "reflect",
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       Healthy(),
	}, WithHostConfig(func(config *container.HostConfig) {
		// configure container to communicate with itself using extra_hosts
		config.ExtraHosts = append(config.ExtraHosts, "reflect:127.0.0.1")
	}))

	ports := cft.UseExclusive(t, ctx, "communicator")
	host := ports.HostPort("80/tcp")
	if host == "" {
		t.Fatal("two: bound port not found")
	}

	// set status
	communicate(t, host, "set", "reflecting")
	// exchange status with container-self
	communicate(t, host, "exchange", "")
	// get exchanged status
	status := communicate(t, host, "get", "")

	if status != "reflecting" {
		t.Fatalf(`expected status "reflecting", but got %q`, status)
	}
}

func TestWithNetworkingConfig(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var (
		name  = "com"
		alias = "com_alias"
	)

	// create a communicator with two aliases
	cft1, term1 := New(t, ctx, WithNamespace(t.Name(), true))
	t.Cleanup(term1)
	cft1.Run(t, ctx, name, &Container{
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": alias,
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       Healthy(),
	}, WithNetworkingConfig(func(config *network.NetworkingConfig) {
		for _, cfg := range config.EndpointsConfig {
			// add alias
			cfg.Aliases = append(cfg.Aliases, alias)
		}
	}))
	host := cft1.UseExclusive(t, ctx, name).HostPort("80/tcp")
	if host == "" {
		t.Fatalf("%s: bound port not found", name)
	}

	// set status
	communicate(t, host, "set", "hello")
	// exchange status with container-self
	communicate(t, host, "exchange", "")
	// get exchanged status
	status := communicate(t, host, "get", "")

	if status != "hello" {
		t.Fatalf(`expected status "hello", but got %q`, status)
	}
}

func TestWithConfigConsistency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	namespace := t.Name()
	cft, term := New(t, ctx, WithNamespace(namespace, true))
	t.Cleanup(term)

	cft.Run(t, ctx, "echo", &Container{
		Image:        imageEcho,
		ExposedPorts: []string{"80/tcp", "8080/tcp"},
		Waiter:       Healthy(),
	})

	testCases := []struct {
		desc                     string
		ports                    []string
		configConsistencyEnabled bool
		failed                   bool
	}{
		{
			desc:                     "less ports with consistency check",
			ports:                    []string{"80/tcp"},
			configConsistencyEnabled: true,
			failed:                   false,
		}, {
			desc:                     "extra ports with consistency check",
			ports:                    []string{"80/tcp", "8443/tcp"},
			configConsistencyEnabled: true,
			failed:                   true,
		}, {
			desc:                     "less ports without consistency check",
			ports:                    []string{"80/tcp"},
			configConsistencyEnabled: false,
			failed:                   false,
		}, {
			desc:                     "extra ports without consistency check",
			ports:                    []string{"80/tcp", "8443/tcp"},
			configConsistencyEnabled: false,
			failed:                   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			cft, term := New(t, ctx, WithNamespace(namespace, true))
			t.Cleanup(term)

			var opts []RunOption
			if !tc.configConsistencyEnabled {
				opts = append(opts, WithConfigConsistency(false))
			}

			recovered := func() (r any) {
				defer func() {
					r = recover()
				}()
				c, term := NewControl()
				defer term()

				cft.Run(c, ctx, "echo", &Container{
					Image:        imageEcho,
					ExposedPorts: tc.ports,
					Waiter:       Healthy(),
				}, opts...)
				return nil
			}()
			if tc.failed && recovered == nil {
				t.Fatal("expected fail because of inconsistency, but not failed")
			} else if !tc.failed && recovered != nil {
				t.Fatal("expected not to fail, but failed")
			}
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
	endpoint := ports.HostPort("80/tcp")
	if endpoint == "" {
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

func TestWithReleaseFunc(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cft, term := New(t, ctx, WithNamespace(t.Name(), true))
	t.Cleanup(term)

	cft.Run(t, ctx, "echo", &Container{
		Image:        imageEcho,
		ExposedPorts: []string{"80/tcp"},
		Waiter:       Healthy(),
	})

	// test that the container is not released until the release is called.
	var release func()
	func() {
		c, cleanup := NewControl()
		defer cleanup()
		cft.UseExclusive(c, ctx, "echo", WithReleaseFunc(&release))
	}()

	use := func() (r any) {
		defer func() {
			r = recover()
		}()
		c, cleanup := NewControl()
		defer cleanup()
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		cft.UseExclusive(c, ctx, "echo")
		return nil
	}
	if use() == nil {
		release()
		t.Fatal("timeout expected")
	}

	release()
	if v := use(); v != nil {
		t.Fatalf("unexpected failure: %v", v)
	}
}

func TestWithInitFunc(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cft, term := New(t, ctx, WithNamespace(t.Name(), true))
	t.Cleanup(term)

	cft.Run(t, ctx, "echo", &Container{
		Image:        imageEcho,
		ExposedPorts: []string{"80/tcp"},
		Waiter:       Healthy(),
	})

	var try, done int
	use := func() (r any) {
		defer func() {
			r = recover()
		}()
		c, cleanup := NewControl()
		defer cleanup()
		cft.UseShared(c, ctx, "echo", WithInitFunc(func(ctx context.Context, ports Ports) error {
			if try++; try < 3 {
				return errors.New("dummy error")
			}
			if len(ports["80/tcp"]) == 0 {
				return errors.New("port not found")
			}
			done++
			return nil
		}))
		return nil
	}

	for i := 0; i < 5; i++ {
		v := use()
		if i < 2 {
			if v == nil {
				t.Fatal("expected error on init")
			}
			continue
		}
		if v != nil {
			t.Fatalf("unexpected failure: %v", v)
		}
	}
	if done != 1 {
		t.Fatalf("expected call of init: 1, actual: %d", done)
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
