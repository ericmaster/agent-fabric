//go:build unix

package retrospective

import (
	"context"
	"os/exec"
	"syscall"
)

// Kill the entire owned command group on deadline, including children retaining
// output pipes. Ordinary maintenance never leaves a background daemon behind.
func runMaintenanceProcess(ctx context.Context, cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		case <-done:
		}
	}()
	err := cmd.Wait()
	close(done)
	<-stopped
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	return err
}
