package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dbvault/dbvault/internal/ports"
)

const defaultStderrLimit = 1024 * 1024

type Runner struct{ StderrLimit int }

func New() *Runner { return &Runner{StderrLimit: defaultStderrLimit} }

func (r *Runner) Run(ctx context.Context, req ports.ProcessRequest) (ports.ProcessResult, error) {
	started := time.Now()
	p, err := r.Start(ctx, req)
	if err != nil {
		return ports.ProcessResult{}, err
	}
	res, err := p.Wait()
	res.Duration = time.Since(started)
	return res, err
}

func (r *Runner) Start(ctx context.Context, req ports.ProcessRequest) (ports.RunningProcess, error) {
	if req.Executable == "" {
		return nil, errors.New("process executable is required")
	}
	if strings.Contains(req.Executable, "\x00") {
		return nil, errors.New("invalid executable")
	}
	ctx2 := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		ctx2, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	cmd := osexec.CommandContext(ctx2, req.Executable, req.Arguments...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Dir = req.WorkingDirectory
	cmd.Stdin = req.StandardInput
	cmd.Stdout = req.StandardOutput
	stderrLimit := r.StderrLimit
	if stderrLimit <= 0 {
		stderrLimit = defaultStderrLimit
	}
	limited := &limitedBuffer{limit: stderrLimit}
	if req.StandardError != nil {
		cmd.Stderr = io.MultiWriter(limited, req.StandardError)
	} else {
		cmd.Stderr = limited
	}
	cmd.Env = os.Environ()
	for _, env := range req.Environment {
		cmd.Env = append(cmd.Env, env.Name+"="+env.Value)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	return &running{cmd: cmd, cancel: cancel, stderr: limited, redactions: req.Redactions}, nil
}

type running struct {
	cmd        *osexec.Cmd
	cancel     func()
	stderr     *limitedBuffer
	redactions []string
	once       sync.Once
}

func (r *running) Wait() (ports.ProcessResult, error) {
	defer r.cancel()
	err := r.cmd.Wait()
	code := 0
	if err != nil {
		var exit *osexec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		} else {
			code = -1
		}
	}
	stderr := redact(r.stderr.String(), r.redactions)
	return ports.ProcessResult{ExitCode: code, StdErr: stderr, StdErrTruncated: r.stderr.Truncated()}, err
}

func (r *running) Terminate(ctx context.Context) error {
	var err error
	r.once.Do(func() {
		if r.cmd.Process == nil {
			return
		}
		pgid, e := syscall.Getpgid(r.cmd.Process.Pid)
		if e == nil {
			err = syscall.Kill(-pgid, syscall.SIGTERM)
		} else {
			err = r.cmd.Process.Signal(syscall.SIGTERM)
		}
		select {
		case <-ctx.Done():
			if r.cmd.Process != nil {
				_ = r.cmd.Process.Kill()
			}
		case <-time.After(2 * time.Second):
		}
	})
	return err
}

type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.truncated = true
		_, _ = b.buf.Write(p[:remaining])
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}
func (b *limitedBuffer) String() string  { return b.buf.String() }
func (b *limitedBuffer) Truncated() bool { return b.truncated }

func redact(s string, values []string) string {
	for _, v := range values {
		if v != "" {
			s = strings.ReplaceAll(s, v, "[REDACTED]")
		}
	}
	return s
}

func SafeArgs(args []string, redactions []string) string {
	joined := strings.Join(args, " ")
	return fmt.Sprintf("%s", redact(joined, redactions))
}
