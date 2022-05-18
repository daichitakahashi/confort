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
	}
	identOptionSkipIfAlreadyExists struct{}
	identOptionImageBuildOptions   struct{}
	buildOption                    struct{ option.Interface }
)

func (buildOption) build() {}

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

// BuildAndRun builds new image with given Dockerfile and context directory.
// After the build is complete, start the container.
//
// When same name image already exists and using WithSkipIfAlreadyExists option,
// BuildAndRun skips to build. In other words, it always builds image without WithSkipIfAlreadyExists.
func (g *Group) BuildAndRun(ctx context.Context, tb testing.TB, name string, b *Build, opts ...BuildOption) map[nat.Port]string {
	tb.Helper()

	if g.namespace != "" {
		name = g.namespace + "-" + name
	}

	g.m.Lock()
	defer g.m.Unlock()

	// find existing container in Group
	info, ok := g.containers[name]
	if ok && info.started {
		return info.endpoints
	}

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
		buildOption := types.ImageBuildOptions{
			Tags:           []string{b.Image},
			SuppressOutput: !b.Output,
			Remove:         true,
			PullParent:     true,
			Dockerfile:     b.Dockerfile,
			BuildArgs:      b.BuildArgs,
			Target:         "",
			SessionID:      "",
			Platform:       b.Platform,
		}
		if modifyBuildOptions != nil {
			modifyBuildOptions(&buildOption)
		}

		tarball, err := createArchive(b.ContextDir, b.Dockerfile)
		if err != nil {
			tb.Fatal(err)
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

	runOpts := make([]RunOption, 0, len(opts))
	var pullOpt identOptionPullOptions
	for _, opt := range opts {
		if opt.Ident() == pullOpt {
			continue // no need to pull
		}
		if runOpt, ok := opt.(RunOption); ok {
			runOpts = append(runOpts, runOpt)
		}
	}

	return g.run(ctx, tb, name, &Container{
		Image:        b.Image,
		Env:          b.Env,
		Cmd:          b.Cmd,
		Entrypoint:   b.Entrypoint,
		ExposedPorts: b.Entrypoint,
		Waiter:       b.Waiter,
	}, info, runOpts...)
}

func createArchive(ctxDir, dockerfilePath string) (io.ReadCloser, error) {
	absContextDir, relDockerfile, err := build.GetContextFromLocalDir(ctxDir, dockerfilePath)
	if err != nil {
		return nil, err
	}

	excludes, err := build.ReadDockerignore(absContextDir)
	if err != nil {
		return nil, err
	}

	// do not ignore Dockerfile and .dockerignore
	excludes = build.TrimBuildFilesFromExcludes(excludes, relDockerfile, false)

	err = build.ValidateContextDirectory(absContextDir, excludes)
	if err != nil {
		return nil, err
	}

	return archive.TarWithOptions(ctxDir, &archive.TarOptions{
		ExcludePatterns: excludes,
		Compression:     archive.Uncompressed,
		NoLchown:        true,
	})
}
