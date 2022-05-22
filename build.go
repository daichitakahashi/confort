package confort

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/docker/cli/cli/command/image/build"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/go-connections/nat"
	"github.com/lestrrat-go/option"
)

type Build struct {
	Image        string
	Dockerfile   string
	ContextDir   string
	BuildArgs    map[string]*string
	Output       bool
	Platform     string
	Env          map[string]string
	Cmd          []string
	Entrypoint   []string
	ExposedPorts []string
	Waiter       *Waiter
}

type (
	BuildOption interface {
		option.Interface
		build()
		buildAndRun()
	}
	BuildAndRunOption interface {
		option.Interface
		buildAndRun()
	}
	identOptionSkipIfAlreadyExists struct{}
	identOptionImageBuildOptions   struct{}
	buildOption                    struct{ option.Interface }
)

func (buildOption) build()       {}
func (buildOption) buildAndRun() {}

func WithSkipIfAlreadyExists() BuildOption {
	return buildOption{
		Interface: option.New(identOptionSkipIfAlreadyExists{}, true),
	}
}

func WithImageBuildOptions(f func(option *types.ImageBuildOptions)) BuildOption {
	return buildOption{
		Interface: option.New(identOptionImageBuildOptions{}, f),
	}
}

// Build creates new image with given Dockerfile and context directory.
//
// When same name image already exists and using WithSkipIfAlreadyExists option,
// Build skips to build. In other words, it always builds image without WithSkipIfAlreadyExists.
func (g *Group) Build(ctx context.Context, tb testing.TB, b *Build, opts ...BuildOption) {
	tb.Helper()

	var skip bool
	var modifyBuildOptions func(option *types.ImageBuildOptions)

	for _, opt := range opts {
		switch opt.Ident() {
		case identOptionSkipIfAlreadyExists{}:
			skip = opt.Value().(bool)
		case identOptionImageBuildOptions{}:
			modifyBuildOptions = opt.Value().(func(option *types.ImageBuildOptions))
		}
	}

	var noBuild bool
	if skip {
		// check if the same image already exists
		summaries, err := g.cli.ImageList(ctx, types.ImageListOptions{
			All: true,
		})
		if err != nil {
			tb.Fatal(err)
		}
	LOOP:
		for _, s := range summaries {
			for _, t := range s.RepoTags {
				if t == b.Image {
					tb.Logf("image %q already exists", b.Image)
					noBuild = true
					break LOOP
				}
			}
		}
	}

	if !noBuild {
		tarball, relDockerfile, err := createArchive(b.ContextDir, b.Dockerfile)
		if err != nil {
			tb.Fatal(err)
		}

		buildOption := types.ImageBuildOptions{
			Tags:           []string{b.Image},
			SuppressOutput: !b.Output,
			Remove:         true,
			PullParent:     true,
			Dockerfile:     relDockerfile,
			BuildArgs:      b.BuildArgs,
			Target:         "",
			SessionID:      "",
			Platform:       b.Platform,
		}
		if modifyBuildOptions != nil {
			modifyBuildOptions(&buildOption)
		}

		resp, err := g.cli.ImageBuild(ctx, tarball, buildOption)
		if err != nil {
			tb.Fatal(err)
		}
		err = func() error {
			defer func() {
				_, _ = io.ReadAll(resp.Body)
				_ = resp.Body.Close()
			}()
			if !b.Output {
				return nil
			}
			return jsonmessage.DisplayJSONMessagesStream(resp.Body, os.Stdout, 0, false, nil)
		}()
		if err != nil {
			tb.Fatal(err)
		}
	}
}

// BuildAndRun builds new image with given Dockerfile and context directory.
// After the build is complete, start the container.
//
// When same name image already exists and using WithSkipIfAlreadyExists option,
// BuildAndRun skips to build. In other words, it always builds image without WithSkipIfAlreadyExists.
func (g *Group) BuildAndRun(ctx context.Context, tb testing.TB, name string, b *Build, opts ...BuildAndRunOption) map[nat.Port]string {
	tb.Helper()

	if g.namespace != "" {
		name = g.namespace + "-" + name
	}

	g.m.Lock()
	defer g.m.Unlock()

	// find existing container in Group
	info, ok := g.containers[name]
	if ok {
		if info.c.Image != b.Image {
			tb.Fatal(containerNameConflict(name, b.Image, info.c.Image))
		} else if info.started {
			return info.endpoints
		}
	}

	buildOpts := make([]BuildOption, 0, len(opts))
	for _, opt := range opts {
		buildOpt, ok := opt.(BuildOption)
		if !ok {
			continue
		}
		buildOpts = append(buildOpts, buildOpt)
	}
	g.Build(ctx, tb, b, buildOpts...)

	runOpts := make([]RunOption, 0, len(opts))
	var pullOpt identOptionPullOptions
	for _, opt := range opts {
		runOpt, ok := opt.(RunOption)
		if !ok {
			continue
		} else if runOpt.Ident() == pullOpt {
			continue // no need to pull
		}
		runOpts = append(runOpts, runOpt)
	}

	return g.run(ctx, tb, name, &Container{
		Image:        b.Image,
		Env:          b.Env,
		Cmd:          b.Cmd,
		Entrypoint:   b.Entrypoint,
		ExposedPorts: b.ExposedPorts,
		Waiter:       b.Waiter,
	}, info, runOpts...)
}

func createArchive(ctxDir, dockerfilePath string) (io.ReadCloser, string, error) {
	absContextDir, relDockerfile, err := build.GetContextFromLocalDir(ctxDir, dockerfilePath)
	if err != nil {
		return nil, "", err
	}

	excludes, err := build.ReadDockerignore(absContextDir)
	if err != nil {
		return nil, "", err
	}

	// We have to include docker-ignored Dockerfile and .dockerignore for build.
	// When `ADD` or `COPY` executes, daemon excludes these docker-ignored files.
	excludes = build.TrimBuildFilesFromExcludes(excludes, relDockerfile, false)

	err = build.ValidateContextDirectory(absContextDir, excludes)
	if err != nil {
		return nil, "", err
	}

	tarball, err := archive.TarWithOptions(absContextDir, &archive.TarOptions{
		ExcludePatterns: excludes,
		Compression:     archive.Uncompressed,
		NoLchown:        true,
	})
	if err != nil {
		return nil, "", err
	}
	return tarball, relDockerfile, nil
}
