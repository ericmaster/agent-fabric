//go:build !unix

package retrospective

import (
	"context"
	"os/exec"
)

func runMaintenanceProcess(ctx context.Context, cmd *exec.Cmd) error { return ErrCapability }
