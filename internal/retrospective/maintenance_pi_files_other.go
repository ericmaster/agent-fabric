//go:build !linux

// Spec: docs/specs/task-retrospective.md — Standing maintenance authority.
package retrospective

func writePiMaintenanceAnchored(root string, image piMaintenanceImage, hook func(string)) error {
	return ErrCapability // Never substitute pathname check/rename for Linux guards.
}
