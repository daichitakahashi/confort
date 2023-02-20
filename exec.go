package confort

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/daichitakahashi/confort/internal/logging"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type ContainerExec struct {
	c      *Container
	cmd    []string
	cli    *client.Client
	execID string

	Stdout io.Writer
	Stderr io.Writer
}

func (c *Container) CreateExec(ctx context.Context, cmd []string) (*ContainerExec, error) {
	if _, err := c.cft.cli.ContainerInspect(ctx, c.id); err != nil {
		return nil, err
	}
	return &ContainerExec{
		c:   c,
		cmd: cmd,
		cli: c.cft.cli,
	}, nil
}

func (e *ContainerExec) Start(ctx context.Context) error {
	if e.execID != "" {
		return errors.New("confort: exec: already started")
	}
	logging.Debugf("exec on container %q: %v", e.c.name, e.cmd)
	execConfig := types.ExecConfig{
		Cmd:          e.cmd,
		Detach:       false,
		AttachStdout: e.Stdout != nil,
		AttachStderr: e.Stderr != nil,
	}
	// When both stdout and stderr haven't attached, ContainerExecCreate behaves like a detached mode.
	// So, to wait execution done, make stdout attached.
	if !execConfig.AttachStdout && !execConfig.AttachStderr {
		execConfig.AttachStdout = true
	}
	resp, err := e.cli.ContainerExecCreate(ctx, e.c.id, execConfig)
	if err != nil {
		return err
	}
	e.execID = resp.ID
	return nil
}

type ExitError struct {
	ExitCode int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("confort: exec: exit status %d", e.ExitCode)
}

func (e *ContainerExec) Wait(ctx context.Context) error {
	if e.execID == "" {
		return errors.New("confort: exec: not started")
	}

	hijackedResp, err := e.cli.ContainerExecAttach(ctx, e.execID, types.ExecStartCheck{})
	if err != nil {
		return err
	}
	defer hijackedResp.Close()

	var (
		stdout = io.Discard
		stderr = io.Discard
	)
	if e.Stdout != nil {
		stdout = e.Stdout
	}
	if e.Stderr != nil {
		stderr = e.Stderr
	}
	_, err = stdcopy.StdCopy(stdout, stderr, hijackedResp.Reader)
	if err != nil {
		return err
	}

	info, err := e.cli.ContainerExecInspect(ctx, e.execID)
	if err != nil {
		return err
	}
	if info.ExitCode != 0 {
		return &ExitError{
			ExitCode: info.ExitCode,
		}
	}
	return nil
}

func (e *ContainerExec) Run(ctx context.Context) error {
	err := e.Start(ctx)
	if err != nil {
		return err
	}
	return e.Wait(ctx)
}

func (e *ContainerExec) Output(ctx context.Context) ([]byte, error) {
	if e.Stdout != nil {
		return nil, errors.New("confort: exec: Stdout already set")
	}
	buf := bytes.NewBuffer(nil)
	e.Stdout = buf
	err := e.Run(ctx)
	return buf.Bytes(), err
}

func (e *ContainerExec) CombinedOutput(ctx context.Context) ([]byte, error) {
	if e.Stdout != nil {
		return nil, errors.New("confort: exec: Stdout already set")
	}
	if e.Stderr != nil {
		return nil, errors.New("confort: exec: Stderr already set")
	}
	buf := bytes.NewBuffer(nil)
	e.Stdout = buf
	e.Stderr = buf
	err := e.Run(ctx)
	return buf.Bytes(), err
}
