package integration

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/daichitakahashi/confort/internal/dockerutil"
	"github.com/google/subcommands"
)

func NewCommands(set *flag.FlagSet, name string, op Operation, stdout, stderr io.Writer) *subcommands.Commander {
	cmd := subcommands.NewCommander(set, name)
	cmd.Register(subcommands.CommandsCommand(), "help")
	cmd.Register(subcommands.FlagsCommand(), "help")
	cmd.Register(subcommands.HelpCommand(), "help")
	cmd.Register(&StartCommand{
		Operation: op,
		Stdout:    stdout,
		Stderr:    stderr,
	}, "")
	cmd.Register(&StopCommand{
		Operation: op,
		Stderr:    stderr,
	}, "")
	cmd.Register(&TestCommand{
		Operation: op,
	}, "")
	return cmd
}

type StartCommand struct {
	Operation Operation
	Image     string
	ForcePull bool
	Stdout    io.Writer
	Stderr    io.Writer
}

func (s *StartCommand) Name() string {
	return "start"
}

func (s *StartCommand) Synopsis() string {
	return `Start beacon server and output its endpoint to stdout.
If image is not specified, use "ghcr.io/daichitakahashi/confort/beacon:latest".
Set endpoint to environment variable "CFT_BEACON_ENDPOINT", confort.ConnectBeacon detects it and connect server.`
}

func (s *StartCommand) Usage() string {
	return "confort start (-image IMAGE_BEACON)"
}

func (s *StartCommand) SetFlags(set *flag.FlagSet) {
	set.StringVar(&s.Image, "image", "ghcr.io/daichitakahashi/confort/beacon:latest", "")
	set.BoolVar(&s.ForcePull, "forcePull", false, "")
}

func (s *StartCommand) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	endpoint, err := s.Operation.StartBeaconServer(ctx, s.Image, s.ForcePull)
	if err != nil {
		_, _ = fmt.Fprintln(s.Stderr, err.Error())
		return subcommands.ExitFailure
	}
	_, _ = fmt.Fprintln(s.Stdout, endpoint)
	return subcommands.ExitSuccess
}

var _ subcommands.Command = (*StartCommand)(nil)

type StopCommand struct {
	Operation Operation
	Stderr    io.Writer
}

func (s *StopCommand) Name() string {
	return "stop"
}

func (s *StopCommand) Synopsis() string {
	return `Stop beacon server.
This specifies target container by CFT_BEACON_ENDPOINT environment variable.`
}

func (s *StopCommand) Usage() string {
	return "confort stop"
}

func (s *StopCommand) SetFlags(_ *flag.FlagSet) {}

func (s *StopCommand) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	endpoint := os.Getenv("CFT_BEACON_ENDPOINT")
	if endpoint == "" {
		_, _ = fmt.Fprintln(s.Stderr, "CFT_BEACON_ENDPOINT not set")
		return subcommands.ExitFailure
	}

	err := s.Operation.StopBeaconServer(ctx, endpoint)
	if err != nil {
		_, _ = fmt.Fprintln(s.Stderr, err.Error())
		return subcommands.ExitFailure
	}

	// delete all docker resources created in TestCommand
	err = s.Operation.CleanupResources(ctx, dockerutil.LabelEndpoint, endpoint)
	if err != nil {
		_, _ = fmt.Fprintln(s.Stderr, err.Error())
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

var _ subcommands.Command = (*StopCommand)(nil)

type TestCommand struct {
	Operation Operation
	Image     string
	ForcePull bool
	Namespace string
	Stderr    io.Writer
}

func (t *TestCommand) Name() string {
	return "test"
}

func (t *TestCommand) Synopsis() string {
	return `Start beacon server and execute tests.
After test finished, stop beacon server automatically.
If you want to use "go test" option, specify them after "--".`
}

func (t *TestCommand) Usage() string {
	return "confort test (-image IMAGE_BEACON) (-- -p=4 -shuffle=on)"
}

func (t *TestCommand) SetFlags(set *flag.FlagSet) {
	flag.Parse()
	set.StringVar(&t.Image, "image", "ghcr.io/daichitakahashi/confort/beacon:latest", "")
	set.BoolVar(&t.ForcePull, "forcePull", false, "")
	set.StringVar(&t.Namespace, "namespace", "", "")
}

func (t *TestCommand) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	endpoint, err := t.Operation.StartBeaconServer(ctx, t.Image, t.ForcePull)
	if err != nil {
		_, _ = fmt.Fprintln(t.Stderr, err.Error())
		return subcommands.ExitFailure
	}

	var sepIdx int
	for i, arg := range os.Args {
		sepIdx = i
		if arg == "--" {
			break
		}
	}

	env := append(os.Environ(), "CFT_BEACON_ENDPOINT="+endpoint)
	if t.Namespace != "" {
		env = append(env, "CFT_NAMESPACE="+t.Namespace)
	}

	err = t.Operation.ExecuteTest(ctx, os.Args[:sepIdx+1], env)
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return subcommands.ExitStatus(ee.ExitCode())
	}
	if err != nil {
		_, _ = fmt.Fprintln(t.Stderr, err.Error())
		return subcommands.ExitFailure
	}

	err = t.Operation.StopBeaconServer(ctx, endpoint)
	if err != nil {
		_, _ = fmt.Fprintln(t.Stderr, err.Error())
		return subcommands.ExitFailure
	}

	// delete all docker resources created in TestCommand
	err = t.Operation.CleanupResources(ctx, dockerutil.LabelEndpoint, endpoint)
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}

var _ subcommands.Command = (*TestCommand)(nil)
