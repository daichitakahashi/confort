package confort

import (
	"context"
	"testing"

	"github.com/daichitakahashi/confort/compose"
	"github.com/docker/docker/client"
)

func TestComposeBackend_Load(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	cli.NegotiateAPIVersion(ctx)
	be := &composeBackend{
		cli: cli,
	}

	be.Load(ctx, "", compose.LoadOptions{
		ProjectDir:          "./testdata/compose",
		ProjectName:         "",
		OverrideConfigFiles: nil,
		Profiles:            nil,
		EnvFile:             "", // env of process
		Policy:              compose.ResourcePolicy{},
	})
}

// TODO: テストケース
//  * 起動済みの
//  * プロファイルで絞って、除外されたサービスの起動に失敗すること
