package confort

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

func createExecEnv(t *testing.T, ctx context.Context) *Container {
	t.Helper()

	cft, err := New(ctx,
		WithNamespace(t.Name(), true),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cft.Close()
	})

	c, err := cft.Run(ctx, &ContainerParams{
		Name:       "exec-env",
		Image:      "alpine:3.16.2",
		Entrypoint: []string{"sleep", "infinity"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestContainer_CreateExec(t *testing.T) {
	t.Parallel()
	var (
		ctx = context.Background()
		c   = createExecEnv(t, ctx)
	)

	// install findutils
	ce, err := c.CreateExec(ctx, []string{"apk", "add", "findutils"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ce.Run(ctx); err != nil {
		t.Fatalf("failed to install findutils: %s", err)
	}

	t.Run("get stdout and stderr", func(t *testing.T) {
		t.Parallel()
		cmd := []string{"/bin/sh", "-c", `ls /usr | xargs -t -L 1 -P 1 echo "argument:"`}

		t.Run("RUN", func(t *testing.T) {
			t.Parallel()

			ce, err := c.CreateExec(ctx, cmd)
			if err != nil {
				t.Fatal(err)
			}
			var (
				stderr = bytes.NewBuffer(nil)
				stdout = bytes.NewBuffer(nil)
			)
			ce.Stderr = stderr
			ce.Stdout = stdout
			if err := ce.Run(ctx); err != nil {
				t.Log(stderr.String())
				t.Fatalf("failed to run command: %s", err)
			}

			const (
				expectedStderr = `echo argument: bin
echo argument: lib
echo argument: local
echo argument: sbin
echo argument: share
`
				expectedStdout = `argument: bin
argument: lib
argument: local
argument: sbin
argument: share
`
			)
			if diff := cmp.Diff(expectedStderr, stderr.String()); diff != "" {
				t.Error(diff)
			}
			if diff := cmp.Diff(expectedStdout, stdout.String()); diff != "" {
				t.Error(diff)
			}
		})
		t.Run("Output", func(t *testing.T) {
			t.Parallel()

			ce, err := c.CreateExec(ctx, cmd)
			if err != nil {
				t.Fatal(err)
			}
			output, err := ce.Output(ctx)
			if err != nil {
				t.Fatalf("failed to run command: %s", err)
			}

			const expectedStdout = `argument: bin
argument: lib
argument: local
argument: sbin
argument: share
`
			if diff := cmp.Diff(expectedStdout, string(output)); diff != "" {
				t.Error(diff)
			}
		})
		t.Run("CombinedOutput", func(t *testing.T) {
			t.Parallel()

			ce, err := c.CreateExec(ctx, cmd)
			if err != nil {
				t.Fatal(err)
			}
			output, err := ce.CombinedOutput(ctx)
			if err != nil {
				t.Fatalf("failed to run command: %s", err)
			}
			outputLines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")

			// Compare map that has a line of message as the key.
			// Because the order of combined output is not preserved in non-TTY output.
			// The stdout is line buffered, but the stderr is not buffered.
			expected := map[string]struct{}{
				"echo argument: bin":   {},
				"argument: bin":        {},
				"echo argument: lib":   {},
				"argument: lib":        {},
				"echo argument: local": {},
				"argument: local":      {},
				"echo argument: sbin":  {},
				"argument: sbin":       {},
				"echo argument: share": {},
				"argument: share":      {},
			}
			actual := map[string]struct{}{}
			for _, line := range outputLines {
				actual[line] = struct{}{}
			}
			if diff := cmp.Diff(expected, actual); diff != "" {
				t.Error(diff)
			}
		})
	})

	t.Run("exit code", func(t *testing.T) {
		t.Parallel()

		ce, err := c.CreateExec(ctx, []string{"/bin/sh", "-c", "exit 128"})
		if err != nil {
			t.Fatal(err)
		}
		if err := ce.Run(ctx); err != nil {
			if code := err.(*ExitError).ExitCode; code != 128 {
				t.Fatalf("got unexpected exit code: want 128, got %d", code)
			}
			t.Log(err.Error())
		} else {
			t.Fatal("unexpected success")
		}
	})

	t.Run("error cases", func(t *testing.T) {
		t.Run("already started", func(t *testing.T) {
			t.Parallel()

			ce, err := c.CreateExec(ctx, []string{"pwd"})
			if err != nil {
				t.Fatal(err)
			}
			if err := ce.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if err := ce.Run(ctx); err == nil { // already started
				t.Fatal("the second execution must be failed")
			}
		})

		t.Run("not started", func(t *testing.T) {
			t.Parallel()

			ce, err := c.CreateExec(ctx, []string{"pwd"})
			if err != nil {
				t.Fatal(err)
			}
			if err := ce.Wait(ctx); err == nil { // not started
				t.Fatal("before the call of Start, Wait must be failed")
			}
		})

		t.Run("stdout already set", func(t *testing.T) {
			t.Parallel()

			ce, err := c.CreateExec(ctx, []string{"pwd"})
			if err != nil {
				t.Fatal(err)
			}

			ce.Stdout = bytes.NewBuffer(nil)
			if _, err := ce.Output(ctx); err == nil {
				t.Fatal("the Output must be failed when Stdout is already set")
			}
			if _, err := ce.CombinedOutput(ctx); err == nil {
				t.Fatal("the CombinedOutput must be failed when Stdout is already set")
			}

			ce.Stderr = bytes.NewBuffer(nil)
			if _, err := ce.CombinedOutput(ctx); err == nil {
				t.Fatal("the CombinedOutput must be failed when Stderr is already set")
			}
		})
	})
}

func TestWithExecWorkingDir(t *testing.T) {
	t.Parallel()
	var (
		ctx = context.Background()
		c   = createExecEnv(t, ctx)
	)
	const workingDir = "/usr/local"

	// Print working directory and compare.
	ce, err := c.CreateExec(ctx, []string{"pwd"},
		WithExecWorkingDir(workingDir),
	)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ce.Output(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pwd := strings.TrimSpace(string(out))
	if pwd != workingDir {
		t.Fatalf("unexpected working directory: want %q, got %q", workingDir, pwd)
	}
}

func TestWithExecEnv(t *testing.T) {
	t.Parallel()
	var (
		ctx = context.Background()
		c   = createExecEnv(t, ctx)

		key1   = "EXAMPLE_ENV_1"
		value1 = fmt.Sprintf("%s %s", uuid.NewString(), uuid.NewString())
		key2   = "EXAMPLE_ENV_2"
		value2 = fmt.Sprintf("%s\n%s", uuid.NewString(), uuid.NewString())
	)

	// Print value and compare.
	ce, err := c.CreateExec(ctx, []string{"/bin/sh", "-c", fmt.Sprintf(`printf "${%s}"`, key1)},
		WithExecEnv(map[string]string{
			key1: value1,
			key2: value2,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	out1, err := ce.Output(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if actual := string(out1); actual != value1 {
		t.Fatalf("got unexpected value(key=%q): want %q, got %q", key1, value1, actual)
	}

	ce, err = c.CreateExec(ctx, []string{"/bin/sh", "-c", fmt.Sprintf(`printf "${%s}"`, key2)},
		WithExecEnv(map[string]string{
			key1: value1,
			key2: value2,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	out2, err := ce.Output(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if actual := string(out2); actual != value2 {
		t.Fatalf("got unexpected value(key=%q): want %q, got %q", key2, value2, actual)
	}
}
