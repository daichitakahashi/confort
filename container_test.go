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
)

func TestNewGroup(t *testing.T) {
	// test network creation and communication between host and container,
	// and between containers.
	ctx := context.Background()
	image := "communicator:dev"

	g, term := NewGroup(ctx, t,
		WithNamespace("cn"),
		WithNetwork("communicator"),
		WithClientOpts(client.FromEnv),
	)
	defer func() {
		term()
		_, err := g.cli.ImageRemove(ctx, image, types.ImageRemoveOptions{})
		if err != nil {
			t.Logf("failed to remove image %q: %s", image, err)
		}
	}()

	portsOne := g.BuildAndRun(ctx, t, "one", &Build{
		Image:      image,
		Dockerfile: "testdata/communicator/Dockerfile",
		ContextDir: "testdata/communicator",
		Output:     true,
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

	portsTwo := g.BuildAndRun(ctx, t, "two", &Build{
		Image:      image,
		Dockerfile: "testdata/communicator/Dockerfile",
		ContextDir: "testdata/communicator",
		Output:     true,
		Env: map[string]string{
			"CM_TARGET": "one",
		},
		ExposedPorts: []string{"80/tcp"},
		Waiter:       Healthy(),
	}, WithSkipIfAlreadyExists())
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
