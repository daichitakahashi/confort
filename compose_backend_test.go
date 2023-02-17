package confort

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daichitakahashi/confort/compose"
	"github.com/daichitakahashi/confort/internal/beacon"
	"github.com/docker/docker/client"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/golden"
)

func newBackend(t *testing.T) *composeBackend {
	t.Helper()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	cli.NegotiateAPIVersion(context.Background())
	return &composeBackend{
		cli: cli,
	}
}

// test against unified & modified configuration
func TestComposeBackend_Load(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	be := newBackend(t)

	type testCase struct {
		configFile  string
		opts        compose.LoadOptions
		projectName string
		goldenFile  string
	}
	runTest := func(t *testing.T, tc testCase) {
		c, err := be.Load(ctx, tc.configFile, tc.opts)
		if err != nil {
			t.Fatal(err)
		}
		cc := c.(*composer)

		// test project name
		assert.Equal(t, tc.projectName, cc.ProjectName())

		// test unified & modified config
		appliedConfig, err := cc.dockerCompose(ctx, "config").Output()
		if err != nil {
			t.Fatal(err)
		}
		tc.goldenFile = strings.TrimPrefix(
			filepath.Clean(tc.goldenFile), "testdata/")
		golden.Assert(t, string(appliedConfig), tc.goldenFile)
	}

	testCases := map[string]testCase{
		"normal": {
			configFile: "./testdata/compose/simple/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/simple",
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			projectName: "simple",
			goldenFile:  "./testdata/compose/simple/modified.yaml",
		},
		"override-name": {
			configFile: "./testdata/compose/simple/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/simple",
				ProjectName:             "new-name",
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			projectName: "new-name",
			goldenFile:  "./testdata/compose/simple/modified-override-name.yaml",
		},
		"override": {
			configFile: "./testdata/compose/override/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/override",
				OverrideConfigFiles:     []string{"./testdata/compose/override/compose.override.yaml"},
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			projectName: "override",
			goldenFile:  "./testdata/compose/override/modified.yaml",
		},
		"profile-none": {
			configFile: "./testdata/compose/profile/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/profile",
				Profiles:                nil,
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			projectName: "profile",
			goldenFile:  "./testdata/compose/profile/modified.yaml",
		},
		"profile-webapp": {
			configFile: "./testdata/compose/profile/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/profile",
				Profiles:                []string{"webapp"},
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			projectName: "profile",
			goldenFile:  "./testdata/compose/profile/modified-webapp.yaml",
		},
		"profile-all": {
			configFile: "./testdata/compose/profile/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/profile",
				Profiles:                []string{"webapp", "proxy"},
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			projectName: "profile",
			goldenFile:  "./testdata/compose/profile/modified-all.yaml",
		},
		"env-default": {
			configFile: "./testdata/compose/env/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/env",
				EnvFile:                 "",
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			projectName: "env",
			goldenFile:  "./testdata/compose/env/modified.yaml",
		},
		"env-prod": {
			configFile: "./testdata/compose/env/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/env",
				EnvFile:                 "./testdata/compose/env/.env-prod",
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			projectName: "env",
			goldenFile:  "./testdata/compose/env/modified-prod.yaml",
		},
		"project-dir": {
			configFile: "./testdata/compose/simple/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/project-dir",
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			projectName: "project-dir",
			goldenFile:  "./testdata/compose/project-dir/modified.yaml",
		},
	}
	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runTest(t, tc)
		})
	}
}

func TestComposer_Up(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	be := newBackend(t)
	_, _ = ctx, be

	// docker composeのテストはしない
	// 意図通りに指示できているかどうかをテストする

	// ラベルを使用して、サービスを検索し、必要なコンテナが出来上がっているかどうかを確認する
	// =>これでラベルのテストはOK

	// 観点
	// スケール
	// 再利用
	// 再利用（別プロセス）

	// compose up
	// compose up
	// ...
	// compose down

	// service not found

	// replicas with smaller scale

	// replicas with larger scale

	// requiring consistent but inconsistent scale

	// up and reuse

	// up and another up

	// up and another up(scaling)
}
