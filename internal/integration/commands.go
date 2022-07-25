package integration

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/exec"

	"github.com/daichitakahashi/confort/internal/dockerutil"
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
	return "confort start"
}

func (s *StartCommand) SetFlags(*flag.FlagSet) {}

func (s *StartCommand) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	endpoint, done, err := s.Operation.StartBeaconServer(ctx)
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}
	// TODO: ファイルにエンドポイントを書き込む
	_ = endpoint

	<-done
	return subcommands.ExitSuccess
}

var _ subcommands.Command = (*StartCommand)(nil)

type StopCommand struct {
	Operation Operation
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
		log.Println("CFT_BEACON_ENDPOINT not set")
		return subcommands.ExitFailure
	}

	// TODO: 環境変数がなければファイルに保存したエンドポイントをチェックする

	err := s.Operation.StopBeaconServer(ctx, endpoint)
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}

	// delete all docker resources created in TestCommand
	err = s.Operation.CleanupResources(ctx, dockerutil.LabelEndpoint, endpoint)
	if err != nil {
		log.Println(err)
		return subcommands.ExitFailure
	}

	// TODO: エンドポイントを保存したファイルがあれば削除する

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
	return "confort test (-namespace NS) (-- -p=4 -shuffle=on)"
}

func (t *TestCommand) SetFlags(set *flag.FlagSet) {
	set.StringVar(&t.Namespace, "namespace", "", "")
}

func (t *TestCommand) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	endpoint, _, err := t.Operation.StartBeaconServer(ctx)
	if err != nil {
		log.Println(err)
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
		log.Println(err)
		return subcommands.ExitFailure
	}

	err = t.Operation.StopBeaconServer(ctx, endpoint)
	if err != nil {
		log.Println(err)
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
