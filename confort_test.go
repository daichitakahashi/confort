package confort_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/internal/beacon"
	"github.com/daichitakahashi/confort/internal/beacon/server"
	"github.com/daichitakahashi/confort/unique"
	"github.com/daichitakahashi/confort/wait"
	"github.com/daichitakahashi/testingc"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const (
	imageCommunicator = "github.com/daichitakahashi/confort/testdata/communicator:test"
	imageEcho         = "github.com/daichitakahashi/confort/testdata/echo:test"
	imageLs           = "github.com/daichitakahashi/confort/testdata/ls:"
)

var (
	// generate unique namespace and name for container
	uniqueName, _ = unique.String(context.Background(), 16)
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	err := os.Setenv(beacon.LogLevelEnv, "0")
	if err != nil {
		log.Panic(err)
	}

	cft, err := confort.New(ctx,
		confort.WithNamespace("for-build", false),
	)
	if err != nil {
		log.Panic(err)
	}
	defer func() {
		_ = cft.Close()
	}()
	cli := cft.APIClient()

	defer func() {
		_, err := cli.ImagesPrune(ctx, filters.NewArgs(
			filters.Arg("dangling", "true"),
		))
		if err != nil {
			log.Printf("prune dangling images failed: %s", err)
		}
	}()
	log.Printf("building image: %s", imageCommunicator)
	err = cft.Build(ctx, &confort.BuildParams{
		Image:      imageCommunicator,
		Dockerfile: "testdata/communicator/Dockerfile",
		ContextDir: "testdata/communicator",
	}, confort.WithBuildOutput(io.Discard), confort.WithForceBuild())
	if err != nil {
		log.Panic(err)
	}
	defer func() {
		log.Printf("remove image: %s", imageCommunicator)
		_, err := cli.ImageRemove(ctx, imageCommunicator, types.ImageRemoveOptions{})
		if err != nil {
			log.Printf("failed to remove image %q: %s", imageCommunicator, err)
		}
	}()
	log.Printf("building image: %s", imageEcho)
	err = cft.Build(ctx, &confort.BuildParams{
		Image:      imageEcho,
		Dockerfile: "testdata/echo/Dockerfile",
		ContextDir: "testdata/echo/",
	}, confort.WithBuildOutput(io.Discard), confort.WithForceBuild())
	if err != nil {
		log.Println(err)
	}
	defer func() {
		log.Printf("remove image: %s", imageEcho)
		_, err := cli.ImageRemove(ctx, imageEcho, types.ImageRemoveOptions{})
		if err != nil {
			log.Printf("failed to remove image %q: %s", imageEcho, err)
		}
	}()

	m.Run()
}

// test network creation and communication between host and container,
// and between containers.
func TestConfort_Run_Communication(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cft, err := confort.New(ctx,
		confort.WithNamespace(t.Name(), false),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft.Close()
	})

	comOne, err := cft.Run(ctx, &confort.ContainerParams{
		Name:  "one",
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "two",
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       wait.LogContains("communicator is ready", 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	portsOne, releaseOne, err := comOne.UseExclusive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseOne)
	hostOne := portsOne.HostPort("80/tcp")
	if hostOne == "" {
		t.Logf("%#v", portsOne)
		t.Fatal("one: bound port not found")
	}

	comTwo, err := cft.Run(ctx, &confort.ContainerParams{
		Name:  "two",
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "one",
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       wait.LogContains("communicator is ready", 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	portsTwo, releaseTwo, err := comTwo.UseExclusive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseTwo)
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

	createContainer := func(t *testing.T, namespace string) string {
		t.Helper()

		ctx := context.Background()
		cft, err := confort.New(ctx, confort.WithNamespace(namespace, true))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = cft.Close()
		})
		echo, err := cft.Run(ctx, &confort.ContainerParams{
			Name:         containerName,
			Image:        imageEcho,
			ExposedPorts: []string{port},
			Waiter:       wait.CommandSucceeds([]string{"wget", "-q", "--spider", "http://localhost"}),
		})
		if err != nil {
			t.Fatal(err)
		}
		ports, release, err := echo.UseShared(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release)
		endpoint := ports.HostPort(nat.Port(port))
		if endpoint == "" {
			t.Fatalf("cannot get endpoint of %q: %v", port, ports)
		}
		return endpoint
	}

	expectedEndpoint := createContainer(t, namespace)

	t.Run(fmt.Sprintf("try to create container %q in same namespace", containerName), func(t *testing.T) {
		t.Parallel()

		actualEndpoint := createContainer(t, namespace)

		if expectedEndpoint != actualEndpoint {
			t.Fatalf("unexpected endpoint: want %q, got: %q", expectedEndpoint, actualEndpoint)
		}
	})

	t.Run(fmt.Sprintf("try to create container %q in different namespace", containerName), func(t *testing.T) {
		t.Parallel()

		namespace := uniqueName.Must(t)
		actualEndpoint := createContainer(t, namespace)

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
	)

	cft1, err := confort.New(ctx,
		confort.WithNamespace(namespace, true),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft1.Close()
	})

	echo, err := cft1.Run(ctx, &confort.ContainerParams{
		Name:         containerName,
		Image:        imageEcho,
		ExposedPorts: []string{"80/tcp"},
		Waiter:       wait.Healthy(),
	})
	if err != nil {
		t.Fatal(err)
	}
	fullName := echo.Name()

	cft2, err := confort.New(ctx,
		confort.WithNamespace(namespace, true),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft2.Close()
	})

	_, err = cft2.Run(ctx, &confort.ContainerParams{ // same name, but different image
		Name:  containerName,
		Image: imageCommunicator,
	})
	if err == nil {
		t.Fatal("error expected on run containers that has same name and different image")
	}
	if !strings.Contains(err.Error(), fullName) {
		t.Fatalf("unexpected error: %s", err)
	}
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

	cft1, err := confort.New(ctx,
		confort.WithNamespace(namespaceA, true),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft1.Close()
	})

	com1, err := cft1.Run(ctx, &confort.ContainerParams{
		Name:  "foo-A",
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "foo-B",
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       wait.Healthy(),
	})
	if err != nil {
		t.Fatal(err)
	}
	e, release1, err := com1.UseShared(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release1)
	hostA := e.HostPort("80/tcp")
	if hostA == "" {
		t.Fatal("failed to get host/port")
	}

	com2, err := cft1.Run(ctx, &confort.ContainerParams{
		Name:  "foo-B",
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "C",
		},
		// Using ephemeral port makes test flaky, why?
		// Without specifying host port, container loses the port binding occasionally.
		ExposedPorts: []string{"8080:80/tcp"},
		Waiter:       wait.Healthy(),
	})
	if err != nil {
		t.Fatal(err)
	}
	e, release2, err := com2.UseShared(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release2)
	hostB := e.HostPort("80/tcp")
	if hostB == "" {
		t.Fatal("failed to get host/port")
	}

	cft2, err := confort.New(ctx,
		confort.WithNamespace(namespaceB, true),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft2.Close()
	})

	com3, err := cft2.Run(ctx, &confort.ContainerParams{ // same name container
		Name:  "B",
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "C",
		},
		ExposedPorts: []string{"8080:80/tcp"},
		Waiter:       wait.Healthy(),
	})
	if err != nil {
		t.Fatal(err)
	}
	e, release3, err := com3.UseShared(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release3)
	hostB2 := e.HostPort("80/tcp")
	if hostB2 == "" {
		t.Fatal("failed to get host/port")
	}
	if hostB != hostB2 {
		t.Fatalf("expected same host: want %q, got %q", hostB, hostB2)
	}

	com4, err := cft2.Run(ctx, &confort.ContainerParams{
		Name:  "C",
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": com3.Alias(), // "B" CHECK THIS WORKS
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       wait.Healthy(),
	})
	if err != nil {
		t.Fatal(err)
	}
	e, release4, err := com4.UseShared(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release4)
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
	c.NegotiateAPIVersion(ctx)

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

	cft, err := confort.New(ctx,
		confort.WithClientOptions(client.FromEnv, client.WithHTTPClient(httpCli)),
		confort.WithNamespace(uuid.NewString(), true),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft.Close()
	})

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
				t.Setenv(beacon.NamespaceEnv, tc.envNamespace)
			}
			cft, err := confort.New(ctx, confort.WithNamespace(tc.optNamespace, tc.force))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = cft.Close()
			})

			actual := cft.Namespace()
			if tc.expectedNamespace != actual {
				t.Fatalf("expected namespace %q, got %q", tc.expectedNamespace, actual)
			}
		})
	}
}

func TestWithNamespace_empty(t *testing.T) {
	t.Parallel()

	cft, err := confort.New(context.Background(),
		confort.WithNamespace("", true),
	)
	if err == nil {
		_ = cft.Close()
		t.Fatal("expected to fail, but succeeded")
	}
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
			timeout: -1, // without confort.WithDefaultTimeout
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
			timeout: 0, // confort.WithDefaultTimeout(0)
			test: func(t *testing.T, deadline time.Time, ok bool) {
				if ok {
					t.Fatal("no deadline expected")
				}
			},
		}, {
			desc:    "with default timeout(5 sec.)",
			timeout: time.Second * 5, // confort.WithDefaultTimeout(time.Second*5)
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
			desc:    "with default timeout(5 sec.) and timeout for confort.New(3 sec.)",
			timeout: time.Second * 5, // confort.WithDefaultTimeout(time.Second*5)
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
			desc:    "with default timeout(5 sec.) and timeout for confort.New(7 sec.)",
			timeout: time.Second * 5, // confort.WithDefaultTimeout(time.Second*5)
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
			opts := []confort.NewOption{
				confort.WithNamespace(uuid.NewString(), true),
				confort.WithClientOptions(client.FromEnv, client.WithHTTPClient(httpCli)),
			}
			if tc.timeout >= 0 {
				opts = append(opts, confort.WithDefaultTimeout(tc.timeout))
			}

			ctx := context.Background()
			if tc.newCtx != nil {
				var cancel context.CancelFunc
				ctx, cancel = tc.newCtx()
				t.Cleanup(cancel)
			}
			cft, err := confort.New(ctx, opts...)
			if err != nil {
				t.Fatal(err)
			}
			_ = cft.Close()
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
	cli.NegotiateAPIVersion(ctx)

	// define assertions

	// container or network is reused
	assertReused := func(t *testing.T, precedingID, id string, result *testingc.TestResult) {
		t.Helper()
		if result.Failed() {
			t.Fatalf("unexpected error: %s", result.Logs())
		}
		if precedingID != id {
			t.Fatalf("ids are not match: %q and %q", precedingID, id)
		}
	}
	// create network or container is failed because network/container having same name already exists
	assertFailed := func(t *testing.T, _, _ string, result *testingc.TestResult) {
		t.Helper()
		if !result.Failed() {
			t.Fatal("expected failure, but succeeds")
		}
	}
	// after termination, check the network is still alive or not
	assertNetwork := func(removed bool) func(t *testing.T, networkID string) {
		return func(t *testing.T, networkID string) {
			t.Helper()

			_, err := cli.NetworkInspect(ctx, networkID, types.NetworkInspectOptions{})
			found := err == nil

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

			_, err := cli.ContainerInspect(ctx, containerID)
			found := err == nil

			if found && removed {
				t.Fatalf("expected to be removed but exists: %q", containerID)
			} else if !found && !removed {
				t.Fatalf("container not found: %q", containerID)
			}
		}
	}

	testCases := []struct {
		policy                   confort.ResourcePolicy
		afterNamespaceCreated    func(t *testing.T, foundNetworkID, gotNetworkID string, result *testingc.TestResult)
		afterNamespaceTerminated func(t *testing.T, networkID string)
		afterContainerCreated    func(t *testing.T, foundContainerID, gotContainerID string, result *testingc.TestResult)
		afterContainerTerminated func(t *testing.T, containerID string)
	}{
		{
			policy:                   confort.ResourcePolicyReuse,
			afterNamespaceCreated:    assertReused,
			afterNamespaceTerminated: assertNetwork(false),
			afterContainerCreated:    assertReused,
			afterContainerTerminated: assertContainer(false),
		},
		{
			policy:                   confort.ResourcePolicyReusable,
			afterNamespaceCreated:    assertReused,
			afterNamespaceTerminated: assertNetwork(false),
			afterContainerCreated:    assertReused,
			afterContainerTerminated: assertContainer(false),
		},
		{
			policy:                   confort.ResourcePolicyTakeOver,
			afterNamespaceCreated:    assertReused,
			afterNamespaceTerminated: assertNetwork(true),
			afterContainerCreated:    assertReused,
			afterContainerTerminated: assertContainer(true),
		},
		{
			policy:                   confort.ResourcePolicyError,
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
				precedes, err := confort.New(ctx, confort.WithNamespace(networkName, true))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					_ = precedes.Close()
				})

				// try to re-create
				var cft *confort.Confort
				var terminated bool
				var networkID string
				result := testingc.Test(func(t *testingc.T) {
					cft, err = confort.New(ctx,
						confort.WithNamespace(networkName, true),
						confort.WithResourcePolicy(tc.policy),
					)
					if err != nil {
						t.Fatal(err)
					}
					if cft != nil && cft.Network() != nil {
						networkID = cft.Network().ID
					}
				})
				t.Cleanup(func() {
					if !terminated && cft != nil {
						_ = cft.Close()
					}
				})
				tc.afterNamespaceCreated(t, precedes.Network().ID, networkID, result)
				if cft != nil {
					_ = cft.Close()
					terminated = true
				}
				tc.afterNamespaceTerminated(t, precedes.Network().ID)
			})

			t.Run("duplicated container name", func(t *testing.T) {
				t.Parallel()

				// create preceding network and container
				namespacePrefix := uniqueName.Must(t)
				middleName := uniqueName.Must(t)
				containerNameSuffix := uniqueName.Must(t)
				precedes, err := confort.New(ctx,
					confort.WithNamespace(fmt.Sprintf("%s-%s", namespacePrefix, middleName), true),
				)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					_ = precedes.Close()
				})

				echo, err := precedes.Run(ctx, &confort.ContainerParams{
					Name:  containerNameSuffix,
					Image: imageEcho,
				})
				if err != nil {
					t.Fatal(err)
				}
				precedingContainerID := echo.ID()

				// try to re-create
				var cft *confort.Confort
				var terminated bool
				var containerID string
				result := testingc.Test(func(t *testingc.T) {
					cft, err = confort.New(ctx,
						confort.WithNamespace(namespacePrefix, true),
						confort.WithResourcePolicy(tc.policy),
					)
					if err != nil {
						t.Fatal(err)
					}
					echo, err := cft.Run(ctx, &confort.ContainerParams{
						Name:  fmt.Sprintf("%s-%s", middleName, containerNameSuffix),
						Image: imageEcho,
					})
					if err != nil {
						t.Fatal(err)
					}
					containerID = echo.ID()
				})
				t.Cleanup(func() {
					if !terminated {
						_ = cft.Close()
					}
				})
				tc.afterContainerCreated(t, precedingContainerID, containerID, result)
				_ = cft.Close()
				terminated = true
				tc.afterContainerTerminated(t, precedingContainerID)

				// remove network if it exists
				_ = cli.NetworkRemove(ctx, cft.Network().ID)
			})
		})
	}
}

func TestWithResourcePolicy_reusable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cft, err := confort.New(ctx,
		confort.WithNamespace(t.Name(), true),
		confort.WithResourcePolicy(confort.ResourcePolicyReusable),
	)
	if err != nil {
		t.Fatal(err)
	}
	cli := cft.APIClient()

	networkID := cft.Network().ID
	echo, err := cft.Run(ctx, &confort.ContainerParams{
		Name:  "echo",
		Image: imageEcho,
	})
	if err != nil {
		t.Fatal(err)
	}
	containerID := echo.ID()
	_ = cft.Close()
	t.Cleanup(func() {
		err = cli.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{
			Force: true,
		})
		if err != nil {
			t.Log(err)
		}
		err := cli.NetworkRemove(ctx, networkID)
		if err != nil {
			t.Log(err)
		}
	})

	// check created resources are alive
	_, err = cli.NetworkInspect(ctx, networkID, types.NetworkInspectOptions{})
	if err != nil {
		t.Errorf("network %s not found: %s", networkID, err)
	}

	_, err = cli.ContainerInspect(ctx, containerID)
	if err != nil {
		t.Errorf("container %s not found: %s", containerID, err)
	}
}

func TestWithResourcePolicy_invalid(t *testing.T) {

	t.Run("invalid policy from env", func(t *testing.T) {
		t.Setenv(beacon.ResourcePolicyEnv, "invalid")

		cft, err := confort.New(context.Background(),
			confort.WithNamespace(uuid.NewString(), true),
		)
		if err == nil {
			_ = cft.Close()
			t.Fatal("expected to fail, but succeeded")
		}
	})

	t.Run("invalid policy from WithResourcePolicy", func(t *testing.T) {
		cft, err := confort.New(context.Background(),
			confort.WithNamespace(uuid.NewString(), true),
			confort.WithResourcePolicy("invalid"),
		)
		if err == nil {
			_ = cft.Close()
			t.Fatal("expected to fail, but succeeded")
		}
	})
}

func TestWithBeacon(t *testing.T) {
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
	}()
	t.Cleanup(func() {
		srv.Stop()
		_ = ln.Close()
	})

	// write lock file
	lockFile := filepath.Join(t.TempDir(), "lock")
	err = beacon.StoreAddressToLockFile(lockFile, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(beacon.LockFileEnv, lockFile)

	t.Run("confort", func(t *testing.T) {
		t.Parallel()

		namespace := strings.ReplaceAll(t.Name(), "/", "_")
		cft, err := confort.New(ctx,
			confort.WithBeacon(),
			confort.WithNamespace(namespace, true),
		)
		if err != nil {
			t.Fatal(err)
		}
		var done bool
		t.Cleanup(func() {
			if !done {
				_ = cft.Close()
				return
			}
		})
		_, err = cft.Run(ctx, &confort.ContainerParams{
			Name:  "tester",
			Image: "github.com/daichitakahashi/confort/testdata/echo:test",
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = cft.Close()
		done = true

		// when beacon server is enabled, network and container is not deleted after termination
		cli := cft.APIClient()

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
		for _, c := range containers {
			err = cli.ContainerRemove(ctx, c.ID, types.ContainerRemoveOptions{
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
		for _, n := range networks {
			err = cli.NetworkRemove(ctx, n.ID)
			if err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("unique", func(t *testing.T) {
		t.Parallel()

		uniq, err := unique.New(ctx, func() (bool, error) {
			return true, nil
		}, unique.WithBeacon(t.Name()))
		if err != nil {
			t.Fatal(err)
		}

		_, err = uniq.New()
		if err != nil {
			t.Fatal(err)
		}
		_, err = uniq.New()
		if err == nil {
			t.Fatal("unexpected success")
		}
	})
}

func TestWithImageBuildOptions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cft, err := confort.New(ctx,
		confort.WithNamespace(t.Name(), true),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft.Close()
	})

	cli := cft.APIClient()

	tag := "label"
	build := &confort.BuildParams{
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
	err = cft.Build(ctx, build,
		confort.WithBuildOutput(io.Discard),
		confort.WithImageBuildOptions(func(option *types.ImageBuildOptions) {
			if option.Labels == nil {
				option.Labels = map[string]string{}
			}
			option.Labels[label] = labelValue
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
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

	cft, err := confort.New(ctx,
		confort.WithNamespace(t.Name(), true),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft.Close()
	})

	cli := cft.APIClient()

	tag := "force"
	build := &confort.BuildParams{
		Image:      imageLs + tag,
		Dockerfile: "testdata/ls/Dockerfile",
		ContextDir: "testdata/ls",
		BuildArgs: map[string]*string{
			"ID": &tag,
		},
	}

	// build once
	err = cft.Build(ctx, build,
		confort.WithForceBuild(),
		confort.WithBuildOutput(io.Discard),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		removeImageIfExists(t, cli, build.Image)
	})

	buf := bytes.NewBuffer(nil)

	// force build
	err = cft.Build(ctx, build,
		confort.WithForceBuild(),
		confort.WithBuildOutput(buf),
	)
	if err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected build log to be written to buf, but got no output")
	}
	buf.Reset()

	// build if the image not exists
	err = cft.Build(ctx, build,
		confort.WithBuildOutput(buf),
	)
	if err != nil {
		t.Fatal(err)
	}
	if buf.Len() > 0 {
		t.Error("expected build to be skipped, but build log is written")
		t.Log(buf.String())
	}
}

func TestWithContainerConfig(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cft, err := confort.New(ctx,
		confort.WithNamespace(t.Name(), true),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft.Close()
	})

	cli := cft.APIClient()

	var (
		label      = "daichitakahashi.confort.test"
		labelValue = t.Name()
	)

	_, err = cft.Run(ctx, &confort.ContainerParams{
		Name:  "echo",
		Image: imageEcho,
	}, confort.WithContainerConfig(func(config *container.Config) {
		if config.Labels == nil {
			config.Labels = map[string]string{}
		}
		config.Labels[label] = labelValue
	}))
	if err != nil {
		t.Fatal(err)
	}

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

	cft, err := confort.New(ctx,
		confort.WithNamespace(t.Name(), true),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft.Close()
	})

	communicator, err := cft.Run(ctx, &confort.ContainerParams{
		Name:  "communicator",
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "reflect",
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       wait.Healthy(),
	}, confort.WithHostConfig(func(config *container.HostConfig) {
		// configure container to communicate with itself using extra_hosts
		config.ExtraHosts = append(config.ExtraHosts, "reflect:127.0.0.1")
	}))
	if err != nil {
		t.Fatal(err)
	}

	ports, release, err := communicator.UseExclusive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
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
	cft1, err := confort.New(ctx, confort.WithNamespace(t.Name(), true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft1.Close()
	})
	communicator, err := cft1.Run(ctx, &confort.ContainerParams{
		Name:  name,
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": alias,
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       wait.Healthy(),
	}, confort.WithNetworkingConfig(func(config *network.NetworkingConfig) {
		for _, cfg := range config.EndpointsConfig {
			// add alias
			cfg.Aliases = append(cfg.Aliases, alias)
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	ports, release, err := communicator.UseExclusive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	host := ports.HostPort("80/tcp")
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
	cft, err := confort.New(ctx, confort.WithNamespace(namespace, true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft.Close()
	})

	ports := []string{"80/tcp", "8080/tcp"}
	env := map[string]string{
		"ENV1": "VALUE",
		"ENV2": "VALUE",
	}
	_, err = cft.Run(ctx, &confort.ContainerParams{
		Name:         "echo",
		Image:        imageEcho,
		ExposedPorts: ports,
		Env:          env,
		Waiter:       wait.Healthy(),
	})
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		desc   string
		ports  []string
		env    map[string]string
		failed bool
	}{
		{
			desc:   "less ports",
			ports:  []string{"80/tcp"},
			env:    env,
			failed: false,
		}, {
			desc:   "extra ports",
			ports:  []string{"80/tcp", "8443/tcp"},
			env:    env,
			failed: true,
		}, {
			desc:  "less env",
			ports: ports,
			env: map[string]string{
				"ENV1": "VALUE",
			},
			failed: false,
		}, {
			desc:  "extra env",
			ports: ports,
			env: map[string]string{
				"ENV1":     "VALUE",
				"ENV2":     "VALUE",
				"MORE_ENV": "VALUE",
			},
			failed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			cft, err := confort.New(ctx, confort.WithNamespace(namespace, true))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = cft.Close()
			})

			_, err = cft.Run(ctx, &confort.ContainerParams{
				Name:         "echo",
				Image:        imageEcho,
				ExposedPorts: tc.ports,
				Env:          tc.env,
				Waiter:       wait.Healthy(),
			}, confort.WithConfigConsistency(true))
			if tc.failed && err == nil {
				t.Fatal("expected fail because of inconsistency, but not failed")
			} else if !tc.failed && err != nil {
				t.Fatalf("expected not to fail, but failed: %s", err)
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
	var terminated bool

	cft, err := confort.New(ctx,
		confort.WithNamespace(namespace, true),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !terminated {
			_ = cft.Close()
		}
	})

	cli := cft.APIClient()

	// remove target image if already exists
	_ = removeImageIfExists(t, cli, pullImage)

	// pull and run
	out := &bytes.Buffer{}
	c, err := cft.Run(ctx, &confort.ContainerParams{
		Name:         containerName,
		Image:        pullImage,
		ExposedPorts: []string{"80/tcp"},
		Waiter:       wait.Healthy(),
	}, confort.WithPullOptions(&types.ImagePullOptions{}, out))
	if err != nil {
		t.Fatal(err)
	}

	t.Log(out.String())
	if out.Len() == 0 {
		t.Fatal("pull image not performed")
	}

	// check if container works
	ports, release, err := c.UseShared(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	endpoint := ports.HostPort("80/tcp")
	if endpoint == "" {
		t.Fatal("endpoint not found")
	}
	assertEchoWorks(t, endpoint)

	// remove container
	_ = cft.Close()
	terminated = true

	// remove pulled image
	removed := removeImageIfExists(t, cli, pullImage)
	if !removed {
		t.Fatalf("cannot remove pulled image %q", pullImage)
	}
}

func TestWithInitFunc(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cft, err := confort.New(ctx, confort.WithNamespace(t.Name(), true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft.Close()
	})

	echo, err := cft.Run(ctx, &confort.ContainerParams{
		Name:         "echo",
		Image:        imageEcho,
		ExposedPorts: []string{"80/tcp"},
		Waiter:       wait.Healthy(),
	})
	if err != nil {
		t.Fatal(err)
	}

	var try, done int
	use := func() error {
		_, release, err := echo.UseShared(ctx, confort.WithInitFunc(func(ctx context.Context, ports confort.Ports) error {
			if try++; try < 3 {
				return errors.New("dummy error")
			}
			if len(ports["80/tcp"]) == 0 {
				return errors.New("port not found")
			}
			done++
			return nil
		}))
		if err == nil {
			t.Cleanup(release)
		}
		return err
	}

	for i := 0; i < 5; i++ {
		err := use()
		if i < 2 {
			if err == nil {
				t.Fatal("expected error on init")
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected failure: %s", err)
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

	cft, err := confort.New(ctx,
		confort.WithNamespace(namespace, true),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft.Close()
	})

	cli := cft.APIClient()

	// start container
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

	tryRun := func() error {
		_, err := cft.Run(ctx, &confort.ContainerParams{
			Name:  "foo",
			Image: imageEcho,
		})
		return err
	}

	// unsupported container status "pause"
	err = cli.ContainerPause(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = tryRun()
	if err == nil {
		t.Fatal("unexpected success")
	}

	// unsupported container status "exited"
	err = cli.ContainerStop(ctx, created.ID, container.StopOptions{})
	if err != nil {
		t.Fatal(err)
	}
	err = tryRun()
	if err == nil {
		t.Fatal("unexpected success")
	}
}

func TestAcquire(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cft, err := confort.New(ctx,
		confort.WithNamespace(t.Name(), true),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft.Close()
	})

	exposed := nat.Port("80/tcp")
	one, err := cft.Run(ctx, &confort.ContainerParams{
		Name:  "one",
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "two",
		},
		ExposedPorts: []string{string(exposed)},
		Waiter:       wait.LogContains("communicator is ready", 1),
	})
	if err != nil {
		t.Fatal(err)
	}

	two, err := cft.Run(ctx, &confort.ContainerParams{
		Name:  "two",
		Image: imageCommunicator,
		Env: map[string]string{
			"CM_TARGET": "one",
		},
		ExposedPorts: []string{string(exposed)},
		Waiter:       wait.LogContains("communicator is ready", 1),
	})
	if err != nil {
		t.Fatal(err)
	}

	initFunc := func(t *testing.T, status string) confort.InitFunc {
		return func(ctx context.Context, ports confort.Ports) error {
			communicate(t, ports.HostPort(exposed), "set", status)
			return nil
		}
	}
	test := func(ports map[confort.AcquisitionTarget]confort.Ports) error {
		oneHost := ports[one].HostPort(exposed)
		twoHost := ports[two].HostPort(exposed)
		communicate(t, oneHost, "exchange", "")
		oneStatus := communicate(t, oneHost, "get", "")
		twoStatus := communicate(t, twoHost, "get", "")
		if oneStatus != "two" || twoStatus != "one" {
			return fmt.Errorf("unexpected status: one=%s, two=%s", oneStatus, twoStatus)
		}
		communicate(t, oneHost, "exchange", "")
		return nil
	}

	doOneTwo := func(ctx context.Context) error {
		ports, release, err := confort.Acquire().
			UseShared(one, confort.WithInitFunc(initFunc(t, "one"))).
			UseExclusive(two, confort.WithInitFunc(initFunc(t, "two"))).
			Do(ctx)
		if err != nil {
			return err
		}
		defer release()
		return test(ports)
	}
	doTwoOne := func(ctx context.Context) error {
		ports, release, err := confort.Acquire().
			UseShared(two, confort.WithInitFunc(initFunc(t, "two"))).
			UseExclusive(one, confort.WithInitFunc(initFunc(t, "one"))).
			Do(ctx)
		if err != nil {
			return err
		}
		defer release()
		return test(ports)
	}

	cond := sync.NewCond(new(sync.Mutex))
	var counter int
	rendezvous := func() {
		cond.L.Lock()
		counter++
		cond.Broadcast()
		for counter < 2 {
			cond.Wait()
		}
		cond.L.Unlock()
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		rendezvous()
		for i := 0; i < 50; i++ {
			err := doOneTwo(ctx)
			if err != nil {
				return err
			}
		}
		return nil
	})
	eg.Go(func() error {
		rendezvous()
		for i := 0; i < 50; i++ {
			err := doTwoOne(ctx)
			if err != nil {
				return err
			}
		}
		return nil
	})
	err = eg.Wait()
	if err != nil {
		t.Fatal(err)
	}
}
