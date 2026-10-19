// Spec: docs/specs/task-retrospective.md
//go:build unix

package retrospective

import (
	"os/exec"
	"syscall"
)

// spawnDrainWorker launches the detached drain worker in its own session so
// the ingest process returns immediately and the worker outlives it. The
// worker exits at once when another worker holds the single-host lock.
// Spec: docs/specs/task-retrospective.md via RunDrain
func spawnDrainWorker(cliBin string) error {
	cmd := exec.Command(cliBin, "retrospective", "drain")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	// The child runs in its own session (Setsid) and the caller drops the
	// handle immediately: the daemon-less worker outlives this process and
	// is reaped by init/systemd (PID 1) when it exits.
	go func() { _, _ = cmd.Process.Wait() }()
	return nil
}
