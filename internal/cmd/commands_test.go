package cmd

import (
	"bytes"
	"context"
	"flag"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daichitakahashi/confort/internal/beacon/util"
	"github.com/daichitakahashi/gocmd"
)

func assert(t *testing.T, typ, want, got string) {
	t.Helper()
	if want != got {
		t.Errorf("unexpected %s: want %s, got %s", typ, want, got)
	}
}

func assertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Errorf("error expected but succeeded")
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestNewCommands_Help(t *testing.T) {
	t.Parallel()

	arguments := [][]string{
		{"help"},
		{"help", "start"},
		{"help", "stop"},
		{"help", "test"},
	}

	for _, args := range arguments {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			f := flag.NewFlagSet("confort", flag.ContinueOnError)
			cmd := NewCommands(f, nil)
			err := f.Parse(args)
			if err != nil {
				t.Fatal(err)
			}
			cmd.Execute(context.Background())
		})
	}
}

func TestTestCommand_determineGoCommand(t *testing.T) {
	t.Parallel()

	curVer, err := gocmd.CurrentVersion()
	if err != nil {
		t.Fatal(err)
	}
	modVer, err := gocmd.ModuleGoVersion()
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		desc   string
		goVer  string
		goMode gocmd.Mode
		test   func(t *testing.T, cmd, ver string, err error)
	}{
		{
			desc: "no option",
			test: func(t *testing.T, cmd, ver string, err error) {
				assert(t, "command path", "go", cmd)
				assert(t, "version", curVer, ver)
				assertNoError(t, err)
			},
		}, {
			desc:  "-go=mod",
			goVer: "mod",
			test: func(t *testing.T, cmd, ver string, err error) {
				if cmd == "go" {
					out, err := exec.Command(cmd, "env", "GOVERSION").Output()
					if err != nil {
						t.Fatal(err)
					}
					goVer := string(bytes.TrimSpace(out))
					assert(t, "version", modVer, gocmd.MajorVersion(goVer))
				} else {
					assert(t, "command path", modVer, gocmd.MajorVersion(filepath.Base(cmd)))
				}
				assert(t, "version", modVer, gocmd.MajorVersion(ver))
				assertNoError(t, err)
			},
		}, {
			desc:   "-go=go*.*.99 -go-mode=exact",
			goVer:  modVer + ".99",
			goMode: gocmd.ModeExact,
			test: func(t *testing.T, _, _ string, err error) {
				assertError(t, err)
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			mode := gocmd.ModeFallback
			if tc.goMode != 0 {
				mode = tc.goMode
			}
			cmd := TestCommand{
				goVer:  tc.goVer,
				goMode: goMode(mode),
			}

			path, ver, err := cmd.determineGoCommand()
			tc.test(t, path, ver, err)
		})
	}
}

func TestResourcePolicy(t *testing.T) {
	t.Parallel()

	policies := []string{
		util.ResourcePolicyError,
		util.ResourcePolicyReuse,
		util.ResourcePolicyTakeOver,
	}

	for _, p := range policies {
		var r resourcePolicy
		err := r.Set(p)
		assertNoError(t, err)
		assert(t, "String", p, r.String())
	}

	var r resourcePolicy
	assertError(t, r.Set("unknown"))
}

func TestGoMode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		str  string
		mode gocmd.Mode
	}{
		{
			str:  "exact",
			mode: gocmd.ModeExact,
		}, {
			str:  "latest",
			mode: gocmd.ModeLatest,
		}, {
			str:  "fallback",
			mode: gocmd.ModeFallback,
		}, {
			str:  "",
			mode: gocmd.ModeFallback,
		}, {
			str: "unknown",
		},
	}

	for _, tc := range testCases {
		t.Run("input="+tc.str, func(t *testing.T) {
			var m goMode
			if tc.mode == 0 {
				assertError(t, m.Set(tc.str))
				assert(t, "String", "", m.String())
				return
			}
			assertNoError(t, m.Set(tc.str))
			if gocmd.Mode(m) != tc.mode {
				t.Fatalf("unexpected mode: want %d, got %d", tc.mode, m)
			}
			var m2 goMode
			assertNoError(t, m2.Set(m.String()))
			if m != m2 {
				t.Fatalf("unexpected re-set mode: want %d, got %d", m, m2)
			}
		})
	}
}
