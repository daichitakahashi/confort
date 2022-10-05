package interrupt

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
)

func TestTest_Interrupted(t *testing.T) {
	ctx := context.Background()

	namespace := uuid.NewString()
	cmd := exec.Command("go", "run", "../../cmd/confort", "test", "-go=mod", "-namespace", namespace,
		"--", "-v", "-tags=interrupt", "./tests")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	// Set new process group id.
	// After the container is created, we send SIGINT to "confort test".
	// Because the command is executed via go command(not a built binary), we
	// have to use a new process group to send the signal.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err := cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	cli.NegotiateAPIVersion(ctx)
	containerExists := func(t *testing.T) bool {
		t.Helper()
		name := "/" + namespace + "-container"
		list, err := cli.ContainerList(ctx, types.ContainerListOptions{
			Filters: filters.NewArgs(
				filters.Arg("name", name),
			),
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range list {
			for _, n := range c.Names {
				if n == name {
					return true
				}
			}
		}
		return false
	}

	var found bool
	for i := 0; i < 20; i++ {
		if containerExists(t) {
			found = true
			break
		}
		time.Sleep(time.Second)
	}
	if !found {
		t.Fatal("container not exists")
	}
	t.Log("container exists")

	// Send SIGINT to created process group.
	err = syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for test command has finished.
	err = cmd.Wait()
	t.Log("confort test finished: ", err)

	// Check whether the container is removed.
	if containerExists(t) {
		t.Fatal("container is not removed")
	}
}
