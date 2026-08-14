package ports

import (
	"context"
	"io"
	"time"
)

type EnvironmentVariable struct {
	Name      string
	Value     string
	Sensitive bool
}

type ProcessRequest struct {
	Executable       string
	Arguments        []string
	Environment      []EnvironmentVariable
	WorkingDirectory string
	StandardInput    io.Reader
	StandardOutput   io.Writer
	StandardError    io.Writer
	Timeout          time.Duration
	Redactions       []string
}

type ProcessResult struct {
	ExitCode        int
	StdErr          string
	StdErrTruncated bool
	Duration        time.Duration
}

type RunningProcess interface {
	Wait() (ProcessResult, error)
	Terminate(context.Context) error
}

type ProcessRunner interface {
	Run(ctx context.Context, request ProcessRequest) (ProcessResult, error)
	Start(ctx context.Context, request ProcessRequest) (RunningProcess, error)
}
