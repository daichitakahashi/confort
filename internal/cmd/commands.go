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
	"github.com/daichitakahashi/gocmd"
	"github.com/google/subcommands"
)

func NewCommands(set *flag.FlagSet, op Operation) *subcommands.Commander {
	cmd := subcommands.NewCommander(set, set.Name())
	cmd.Register(cmd.CommandsCommand(), "help")
	cmd.Register(cmd.FlagsCommand(), "help")
	cmd.Register(cmd.HelpCommand(), "help")
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
	return `Start beacon server for test.`
}

func (s *StartCommand) Usage() string {
	return `$ confort start (-lock-file <filename>)

Start beacon server and output its endpoint to lock file.
Use "confort stop" command to stop beacon server.

By using "-lock-file" option, you can use a user-defined file name as a lock file.
Default file name is ".confort.lock".
To tell the endpoint to test code, you have to set file name as environment variable CFT_LOCKFILE.
If the variable is already set, this command regards CFT_LOCKFILE as the default file name.
See the document of confort.ConnectBeacon.

If lock file already exists, this command fails.

`
}

func (s *StartCommand) SetFlags(f *flag.FlagSet) {
	s.lockFile = os.Getenv(beaconutil.LockFileEnv) // it regards CFT_LOCKFILE as default value
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
	return `Stop beacon server.`
}

func (s *StopCommand) Usage() string {
	return `$ confort stop (-lock-file <filename>)

Stop the beacon server started by "confort start" command.
The target server address will be read from lock file(".confort.lock"), and the lock file will be removed.
If "confort start" has accompanied by "-lock-file" option, this command requires the same.

`
}

func (s *StopCommand) SetFlags(f *flag.FlagSet) {
	f.StringVar(&s.lockFile, "lock-file", beaconutil.LockFile, "user defined lock file name")
}

func (s *StopCommand) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	// read address from env or lock file
	addr, err := beaconutil.Address(ctx, s.lockFile)
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
	err = s.Operation.CleanupResources(ctx, beaconutil.LabelIdentifier, beaconutil.Identifier(addr))
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
	policy    resourcePolicy
	goVer     string
	goMode    goMode
}

func (t *TestCommand) Name() string {
	return "test"
}

func (t *TestCommand) Synopsis() string {
	return `Start beacon server and execute test.`
}

func (t *TestCommand) Usage() string {
	return `$ confort test (-namespace <namespace> -policy <resource policy> -go <go version> -go-mode <mode>) (-- -p=4 -shuffle=on)

Start the beacon server and execute tests.
After the tests are finished, the beacon server will be stopped automatically.
If you want to use options of "go test", put them after "--".

`
}

func (t *TestCommand) SetFlags(f *flag.FlagSet) {
	f.StringVar(&t.namespace, "namespace", os.Getenv(beaconutil.NamespaceEnv), "namespace")
	t.policy = beaconutil.ResourcePolicyReuse
	f.Var(&t.policy, "policy", `resource policy
  * With "error", the existing same resource(network and container) makes test failed
  * With "reuse", tests reuse resources if already exist. It is default.
  * "reusable" is similar to "reuse", but created resources with this policy will not be removed after the tests finished
  * "takeover" is also similar to "reuse", but reused resources with this policy will be removed after the tests`)
	f.StringVar(&t.goVer, "go", "", `specify go version. "-go=mod" enables to use go version written in your go.mod`)
	t.goMode = goMode(gocmd.ModeFallback)
	f.Var(&t.goMode, "go-mode", `use with -go option
  * "exact" finds go command that has the exact same version as given in "-go"
  * "latest" finds go command that has the same major version as given in "-go"
  * "fallback" behaves like "latest", but if no command was found, fallbacks to "go" command`)
}

func (t *TestCommand) Execute(ctx context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	workingDir, err := os.Getwd()
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}

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

	goCmd, ver, err := t.determineGoCommand()
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}
	fmt.Println("use go version:", ver)

	identifier := beaconutil.Identifier(workingDir + ":" + t.namespace)

	// prepare environment variables
	env := os.Environ()
	env = append(env, fmt.Sprintf("%s=%s", beaconutil.AddressEnv, addr))
	if t.namespace != "" {
		env = append(env, fmt.Sprintf("%s=%s", beaconutil.NamespaceEnv, t.namespace))
	}
	env = append(env, fmt.Sprintf("%s=%s", beaconutil.ResourcePolicyEnv, t.policy))
	env = append(env, fmt.Sprintf("%s=%s", beaconutil.IdentifierEnv, identifier))

	// execute test
	var status subcommands.ExitStatus
	err = t.Operation.ExecuteTest(ctx, goCmd, f.Args(), env)
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		status = subcommands.ExitStatus(ee.ExitCode())
	} else if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	} else {
		status = subcommands.ExitSuccess
	}

	if t.policy != beaconutil.ResourcePolicyReusable {
		// delete all docker resources created in TestCommand
		err = t.Operation.CleanupResources(ctx, beaconutil.LabelIdentifier, identifier)
		if err != nil {
			log.Println(err)
			return subcommands.ExitFailure
		}
	}

	return status
}

func (t *TestCommand) determineGoCommand() (string, string, error) {
	switch t.goVer {
	case "":
		goVer, err := gocmd.CurrentVersion()
		return "go", goVer, err
	case "mod":
		modVer, err := gocmd.ModuleGoVersion()
		if err != nil {
			return "", "", fmt.Errorf("failed to read go.mod: %w", err)
		}
		return gocmd.Determine(modVer, gocmd.Mode(t.goMode))
	default:
		return gocmd.Determine(t.goVer, gocmd.Mode(t.goMode))
	}
}

var _ subcommands.Command = (*TestCommand)(nil)

type resourcePolicy string

func (r *resourcePolicy) String() string {
	return string(*r)
}

func (r *resourcePolicy) Set(v string) error {
	if !beaconutil.ValidResourcePolicy(v) {
		return fmt.Errorf("invalid resource policy: %s", v)
	}
	*r = resourcePolicy(v)
	return nil
}

var _ flag.Value = (*resourcePolicy)(nil)

type goMode gocmd.Mode

func (g *goMode) String() string {
	switch gocmd.Mode(*g) {
	case gocmd.ModeExact:
		return "exact"
	case gocmd.ModeLatest:
		return "latest"
	case gocmd.ModeFallback:
		return "fallback"
	}
	return ""
}

func (g *goMode) Set(v string) error {
	switch v {
	case "exact":
		*g = goMode(gocmd.ModeExact)
	case "latest":
		*g = goMode(gocmd.ModeLatest)
	case "fallback", "":
		*g = goMode(gocmd.ModeFallback)
	default:
		return fmt.Errorf("invalid value: %s", v)
	}
	return nil
}

var _ flag.Value = (*goMode)(nil)
