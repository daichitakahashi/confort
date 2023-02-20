package confort

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestContainer_CreateExec(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

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
}
