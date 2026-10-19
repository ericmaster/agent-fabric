// Spec: docs/specs/task-retrospective.md
//go:build !unix

package retrospective

import "os/exec"

// spawnDrainWorker is the non-unix companion of the detached worker launch.
// Spec: docs/specs/task-retrospective.md via RunDrain
func spawnDrainWorker(cliBin string) error {
	cmd := exec.Command(cliBin, "retrospective", "drain")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _, _ = cmd.Process.Wait() }()
	return nil
}
