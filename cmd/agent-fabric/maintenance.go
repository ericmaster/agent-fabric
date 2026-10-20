// Spec: docs/specs/task-retrospective.md — Autonomous maintenance.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"

	"github.com/ericmaster/agent-fabric/internal/retrospective"
)

// runMaintenance resumes the same durable job; it cannot mint operator policy,
// approve a diagnostic or reset a budget. The ordinary drain calls the same API.
func runMaintenance(args []string) error {
	fs := flag.NewFlagSet("maintenance", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var config, incident string
	fs.StringVar(&config, "config", "", "retrospective config with explicit maintenance_policy")
	fs.StringVar(&incident, "incident", "", "independently reviewed pipeline incident")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || incident == "" {
		return errors.New("maintenance requires --incident and no positional arguments")
	}
	cfg, err := loadObservationConfig(config)
	if err != nil {
		return err
	}
	job, runErr := retrospective.RunMaintenance(context.Background(), retrospective.MaintenanceOptions{Config: cfg, IncidentID: incident})
	if job != nil {
		if err := json.NewEncoder(os.Stdout).Encode(job); err != nil {
			return err
		}
	}
	return runErr
}
