package confort

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/daichitakahashi/confort/compose"
	"github.com/daichitakahashi/confort/internal/beacon"
	"github.com/docker/docker/client"
	"gotest.tools/v3/golden"
)

// test against unified & modified configuration
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

	type testCase struct {
		configFile string
		opts       compose.LoadOptions
		goldenFile string
	}
	runTest := func(t *testing.T, tc testCase) {
		c, err := be.Load(ctx, tc.configFile, tc.opts)
		if err != nil {
			t.Fatal(err)
		}
		tc.goldenFile = strings.TrimPrefix(
			filepath.Clean(tc.goldenFile), "testdata/")
		golden.Assert(t, string(c.(*composer).modifiedConfig), tc.goldenFile)
	}

	testCases := map[string]testCase{
		"normal": {
			configFile: "./testdata/compose/simple/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/simple",
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			goldenFile: "./testdata/compose/simple/modified.yaml",
		},
		"override-name": {
			configFile: "./testdata/compose/simple/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/simple",
				ProjectName:             "new-name",
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			goldenFile: "./testdata/compose/simple/modified-override-name.yaml",
		},
		"override": {
			configFile: "./testdata/compose/override/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/override",
				OverrideConfigFiles:     []string{"./testdata/compose/override/compose.override.yaml"},
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			goldenFile: "./testdata/compose/override/modified.yaml",
		},
		"profile-none": {
			configFile: "./testdata/compose/profile/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/profile",
				Profiles:                nil,
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			goldenFile: "./testdata/compose/profile/modified.yaml",
		},
		"profile-webapp": {
			configFile: "./testdata/compose/profile/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/profile",
				Profiles:                []string{"webapp"},
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			goldenFile: "./testdata/compose/profile/modified-webapp.yaml",
		},
		"profile-all": {
			configFile: "./testdata/compose/profile/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/profile",
				Profiles:                []string{"webapp", "proxy"},
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			goldenFile: "./testdata/compose/profile/modified-all.yaml",
		},
		"env-default": {
			configFile: "./testdata/compose/env/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/env",
				EnvFile:                 "",
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			goldenFile: "./testdata/compose/env/modified.yaml",
		},
		"env-prod": {
			configFile: "./testdata/compose/env/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/env",
				EnvFile:                 "./testdata/compose/env/.env-prod",
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			goldenFile: "./testdata/compose/env/modified-prod.yaml",
		},
		"project-dir": {
			configFile: "./testdata/compose/simple/compose.yaml",
			opts: compose.LoadOptions{
				ProjectDir:              "./testdata/compose/project-dir",
				ResourceIdentifierLabel: beacon.LabelIdentifier,
				ResourceIdentifier:      beacon.Identifier("VALUE"),
			},
			goldenFile: "./testdata/compose/project-dir/modified.yaml",
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
