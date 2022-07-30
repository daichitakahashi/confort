package integration

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log"
	"os"
	"os/exec"

	"github.com/daichitakahashi/confort/internal/beaconutil"
	"github.com/google/subcommands"
)

func NewCommands(set *flag.FlagSet, name string, op Operation) *subcommands.Commander {
	cmd := subcommands.NewCommander(set, name)
	cmd.Register(subcommands.CommandsCommand(), "help")
	cmd.Register(subcommands.FlagsCommand(), "help")
	cmd.Register(subcommands.HelpCommand(), "help")
	cmd.Register(&StartCommand{
		Operation: op,
	}, "")
	cmd.Register(&StopCommand{
		Operation: op,
	}, "")
	cmd.Register(&TestCommand{
		Operation: op,
	}, "")
	return cmd
}

type StartCommand struct {
	Operation Operation

	// flags
	lockFile string
}

func (s *StartCommand) Name() string {
	return "start"
}

func (s *StartCommand) Synopsis() string {
	return `Start beacon server and output its endpoint to stdout.
If image is not specified, use "ghcr.io/daichitakahashi/confort/beacon:latest".
Set endpoint to environment variable "CFT_BEACON_ADDR", confort.ConnectBeacon detects it and connect server.`
}

func (s *StartCommand) Usage() string {
	return `confort start (-lock-file <filename>)
`
}

func (s *StartCommand) SetFlags(f *flag.FlagSet) {
	f.StringVar(&s.lockFile, "lock-file", beaconutil.LockFile, "user defined lock file name)")
}

func (s *StartCommand) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	// check lock file
	// if the file already exists, "confort start" fails
	_, err := os.Stat(s.lockFile)
	if err == nil {
		log.Printf(`Lock file %q already exists. Please wait until the test finishes or run "confort stop".`, s.lockFile)
		log.Printf(`* If your test has already finished, you can delete %q directly.`, s.lockFile)
		return subcommands.ExitFailure
	} else if !errors.Is(err, fs.ErrNotExist) {
		log.Println(err)
		return subcommands.ExitFailure
	}

	// start server asynchronously
	addr, done, err := s.Operation.StartBeaconServer(ctx)
	if err != nil {
		log.Println("failed to start beacon server:", err)
		return subcommands.ExitFailure
	}

	// write address into lock file
	err = beaconutil.StoreAddressToLockFile(s.lockFile, addr)
	if err != nil {
		log.Printf("failed to create lock file: %q", s.lockFile)
		log.Println(err)
		err = s.Operation.StopBeaconServer(ctx, addr)
		if err != nil {
			log.Println("failed to stop beacon server:", err)
		}
		return subcommands.ExitFailure
	}

	// wait until finished
	<-done
	return subcommands.ExitSuccess
}

var _ subcommands.Command = (*StartCommand)(nil)

type StopCommand struct {
	Operation Operation

	// flags
	lockFile string
}

func (s *StopCommand) Name() string {
	return "stop"
}

func (s *StopCommand) Synopsis() string {
	return `Stop beacon server.
This specifies target container by CFT_BEACON_ADDR environment variable.`
}

func (s *StopCommand) Usage() string {
	return `confort stop (-lock-file <filename>)
`
}

func (s *StopCommand) SetFlags(f *flag.FlagSet) {
	f.StringVar(&s.lockFile, "lock-file", beaconutil.LockFile, "user defined lock file name")
}

func (s *StopCommand) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	// read address from env or lock file
	addr, err := beaconutil.Address(s.lockFile)
	if err != nil {
		log.Printf("failed to read lock file %q", s.lockFile)
		log.Println(err)
		return subcommands.ExitFailure
	}

	err = s.Operation.StopBeaconServer(ctx, addr)
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}

	// delete all docker resources created in test
	err = s.Operation.CleanupResources(ctx, beaconutil.LabelAddr, addr)
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}

	// delete lock file if it exists
	err = beaconutil.DeleteLockFile(s.lockFile)
	if err != nil {
		log.Printf("failed to delete lock file %q", s.lockFile)
		log.Println(err)
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}

var _ subcommands.Command = (*StopCommand)(nil)

type TestCommand struct {
	Operation Operation
	Namespace string
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
	return `confort test (-namespace NS) (-- -p=4 -shuffle=on)
`
}

func (t *TestCommand) SetFlags(set *flag.FlagSet) {
	set.StringVar(&t.Namespace, "namespace", "", "")
}

func (t *TestCommand) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	// start server asynchronously
	endpoint, _, err := t.Operation.StartBeaconServer(ctx)
	if err != nil {
		log.Println("failed to start beacon server", err)
		return subcommands.ExitFailure
	}

	// get args after "--" as test args
	var testArgs []string
	for i, arg := range os.Args {
		if arg == "--" {
			testArgs = os.Args[i+1:]
			break
		}
	}

	// prepare environment variables
	env := append(os.Environ(), beaconutil.AddressEnv+"="+endpoint)
	if t.Namespace != "" {
		env = append(env, beaconutil.NamespaceEnv+"="+t.Namespace)
	}

	// execute test
	err = t.Operation.ExecuteTest(ctx, testArgs, env)
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return subcommands.ExitStatus(ee.ExitCode())
	}
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}

	err = t.Operation.StopBeaconServer(ctx, endpoint)
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}

	// delete all docker resources created in TestCommand
	err = t.Operation.CleanupResources(ctx, beaconutil.LabelAddr, endpoint)
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}

var _ subcommands.Command = (*TestCommand)(nil)
