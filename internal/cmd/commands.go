package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"

	"github.com/daichitakahashi/confort/internal/beaconutil"
	"github.com/google/subcommands"
)

func NewCommands(set *flag.FlagSet, op Operation) *subcommands.Commander {
	cmd := subcommands.NewCommander(set, set.Name())
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
	return `Start beacon server and output its endpoint to lock file.
Use "confort stop" command to stop beacon server.

By using "-lock-file" option, you can use a user-defined file name as a lock file.
Default file name is ".confort.lock".

If lock file already exists, this command fails.`
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
	return `Stop beacon server run by "confort start" command.
The target server address will be read from lock file(".confort.lock"), and the lock file will be removed.
If "confort start" has accompanied by "-lock-file" option, this command requires the same.`
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

	// flags
	namespace string
	policy    string
}

func (t *TestCommand) Name() string {
	return "test"
}

func (t *TestCommand) Synopsis() string {
	return `Start beacon server and execute tests.
After tests are finished, beacon server will be stopped automatically.
If you want to use options of "go test", put them after "--".`
}

func (t *TestCommand) Usage() string {
	return `confort test (-namespace <namespace> -policy <resource policy>) (-- -p=4 -shuffle=on)
`
}

func (t *TestCommand) SetFlags(f *flag.FlagSet) {
	f.StringVar(&t.namespace, "namespace", "", "namespace")
	f.StringVar(&t.policy, "policy", beaconutil.ResourcePolicyReuse, `resource policy("error", "reuse" or "takeover")`)
}

func (t *TestCommand) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	// start server asynchronously
	addr, _, err := t.Operation.StartBeaconServer(ctx)
	if err != nil {
		log.Println("failed to start beacon server:", err)
		return subcommands.ExitFailure
	}
	defer func() {
		err = t.Operation.StopBeaconServer(ctx, addr)
		if err != nil {
			log.Println("(CAUTION) error occurred in stopping beacon server:", err)
		}
	}()

	// prepare environment variables
	env := os.Environ()
	env = append(env, fmt.Sprintf("%s=%s", beaconutil.AddressEnv, addr))
	if t.namespace != "" {
		env = append(env, fmt.Sprintf("%s=%s", beaconutil.NamespaceEnv, t.namespace))
	}
	if t.policy != "" {
		if !beaconutil.ValidResourcePolicy(t.policy) {
			log.Printf("invalid resource policy %q", t.policy)
			return subcommands.ExitFailure
		}
		env = append(env, fmt.Sprintf("%s=%s", beaconutil.ResourcePolicyEnv, t.policy))
	}

	// execute test
	err = t.Operation.ExecuteTest(ctx, f.Args(), env)
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return subcommands.ExitStatus(ee.ExitCode())
	}
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}

	// delete all docker resources created in TestCommand
	err = t.Operation.CleanupResources(ctx, beaconutil.LabelAddr, addr)
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}

var _ subcommands.Command = (*TestCommand)(nil)
