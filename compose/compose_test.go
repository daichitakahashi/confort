package compose

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func TestPrepareProject(t *testing.T) {
	ctx := context.Background()

	// change working directory to testdata
	wd := chdir(t, "testdata")

	// get module directory
	modDir, err := resolveGoModDir(ctx)
	if err != nil {
		t.Fatal(err)
	}
	modDirName := filepath.Base(modDir)

	testCases := []struct {
		desc                string
		projectDir          []string
		projectName         string
		configFiles         []string
		errorExpected       bool
		expectedWorkingDir  string
		expectedProjectName string
		expectedServiceName string
	}{
		{
			desc:                "load default config file",
			expectedWorkingDir:  wd,
			expectedProjectName: "testdata",
			expectedServiceName: "default",
		}, {
			desc:                "load custom config file",
			configFiles:         []string{"custom-compose.yaml"},
			expectedWorkingDir:  wd,
			expectedProjectName: "testdata",
			expectedServiceName: "custom",
		}, {
			desc:                "load another config file from another directory",
			configFiles:         []string{"./another/compose.yaml"},
			expectedWorkingDir:  filepath.Join(wd, "another"),
			expectedProjectName: "another",
			expectedServiceName: "another",
		}, {
			desc:                "ignore empty file name",
			configFiles:         []string{""},
			expectedWorkingDir:  wd,
			expectedProjectName: "testdata",
			expectedServiceName: "default",
		}, {
			desc:                "ignore '-' as config file source",
			configFiles:         []string{"-"},
			expectedWorkingDir:  wd,
			expectedProjectName: "testdata",
			expectedServiceName: "default",
		}, {
			desc:                "specify project name",
			projectName:         "my-project",
			expectedWorkingDir:  wd,
			expectedProjectName: "my-project",
			expectedServiceName: "default",
		}, {
			desc:                "specify project directory(absolute path)",
			projectDir:          []string{wd, "another"},
			expectedWorkingDir:  filepath.Join(wd, "another"),
			expectedProjectName: "another",
			expectedServiceName: "another",
		}, {
			desc:                "specify project directory(relative path)",
			projectDir:          []string{"./another"},
			expectedWorkingDir:  filepath.Join(wd, "another"),
			expectedProjectName: "another",
			expectedServiceName: "another",
		}, {
			desc:                "specify project directory and name",
			projectDir:          []string{wd, "another"},
			projectName:         "new-project",
			expectedWorkingDir:  filepath.Join(wd, "another"),
			expectedProjectName: "new-project",
			expectedServiceName: "another",
		}, {
			desc:                "specify project directory, name and custom config",
			projectDir:          []string{wd, "another"},
			configFiles:         []string{"../custom-compose.yaml"},
			expectedWorkingDir:  filepath.Join(wd, "another"),
			expectedProjectName: "another",
			expectedServiceName: "custom",
		}, {
			desc:          "specify root directory as project directory",
			projectDir:    []string{string(filepath.Separator)},
			configFiles:   []string{filepath.Join(wd, "compose.yaml")},
			errorExpected: true,
		}, {
			desc:                "specify module directory as project directory",
			projectDir:          []string{ModDir},
			configFiles:         []string{"compose/testdata/compose.yaml"},
			expectedWorkingDir:  modDir,
			expectedProjectName: modDirName,
			expectedServiceName: "default",
		}, {
			desc:                "specify project directory by relative path from module directory",
			projectDir:          []string{ModDir, "compose/testdata/another"},
			expectedWorkingDir:  filepath.Join(wd, "another"),
			expectedProjectName: "another",
			expectedServiceName: "another",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			p, err := prepareProject(ctx, tc.projectDir, tc.projectName, tc.configFiles)
			if tc.errorExpected {
				if err == nil {
					t.Fatal("must be error")
				}
				return
			} else if err != nil {
				t.Fatal(err)
			}
			// check working directory
			assert.Equal(t, p.WorkingDir, tc.expectedWorkingDir)

			// check project configuration
			assert.Equal(t, p.Name, tc.expectedProjectName)

			// check service name(file content)
			_, err = p.GetService(tc.expectedServiceName)
			assert.NilError(t, err)
		})
	}
}

func chdir(t *testing.T, dir string) (newWorkingDir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	err = os.Chdir(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err = os.Chdir(wd)
		if err != nil {
			t.Fatal(err)
		}
	})
	newWorkingDir, err = os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return newWorkingDir
}

func TestComposeUp(t *testing.T) {
	ctx := context.Background()
	compose, err := New(ctx, []string{"compose/testdata/compose.yaml"}, WithProjectDir(ModDir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = compose.Close()
	}()

	_, err = compose.Up(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
}
