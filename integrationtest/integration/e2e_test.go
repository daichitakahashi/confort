package integration

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/daichitakahashi/confort/internal/beaconutil"
	"github.com/daichitakahashi/confort/internal/cmd"
	"github.com/google/subcommands"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func TestStartAndStop(t *testing.T) {
	ctx := context.Background()
	lockFile := reserveLockFile(t)

	op := cmd.NewOperation()
	start := cmd.StartCommand{
		Operation: op,
	}
	f := flag.NewFlagSet("start", flag.ContinueOnError)
	start.SetFlags(f)
	err := f.Parse([]string{"-lock-file", lockFile})
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan subcommands.ExitStatus)
	go func() {
		stopped <- start.Execute(ctx, f)
	}()

	addr := waitLockFile(t, lockFile)

	// execute tests

	var eg errgroup.Group
	env := append(
		os.Environ(),
		fmt.Sprintf("%s=%s", beaconutil.AddressEnv, addr),
		fmt.Sprintf("%s=%s", beaconutil.NamespaceEnv, uuid.NewString()),
	)
	// use "go" command which executes this test
	goCmd := goCommand()
	for i := 0; i < 4; i++ {
		<-time.After(200 * time.Millisecond)
		eg.Go(func() error {
			buf := bytes.NewBuffer(nil)
			testCmd := exec.Command(goCmd, "test", "-shuffle=on", "-count=20", "-v", "../tests")
			testCmd.Env = env
			testCmd.Stdout = buf
			err := testCmd.Run()
			t.Log(buf.String())
			return err
		})
	}
	err = eg.Wait()
	if err != nil {
		t.Error(err)
	}

	//

	stop := cmd.StopCommand{
		Operation: op,
	}
	f = flag.NewFlagSet("stop", flag.ContinueOnError)
	stop.SetFlags(f)
	err = f.Parse([]string{"-lock-file", lockFile})
	if err != nil {
		t.Fatal(err)
	}
	code := stop.Execute(ctx, f)
	if code != 0 {
		t.Fatalf("unexpected exit code of stop: %d", code)
	}

	code = <-stopped
	if code != 0 {
		t.Fatalf("unexpected exit code of stop: %d", code)
	}
}

func reserveLockFile(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "e2e")
	if err != nil {
		t.Fatal(err)
	}
	name := f.Name()
	defer func() {
		_ = f.Close()
		_ = os.Remove(name)
	}()
	return name
}

func waitLockFile(t *testing.T, lockFile string) string {
	for i := 0; i < 10; i++ {
		<-time.After(200 * time.Millisecond)
		data, err := os.ReadFile(lockFile)
		if err != nil {
			continue
		}
		return string(data)
	}
	t.Fatal("waitLockFile: failed to load", lockFile)
	return ""
}

func goCommand() string {
	goRoot := os.Getenv("GOROOT")
	if goRoot != "" {
		return filepath.Join(goRoot, "bin", "go")
	}
	candidate := "go" + runtime.Version() // go1.18.3
	p, err := exec.LookPath(candidate)
	if err == nil {
		return p
	}
	return "go"
}
