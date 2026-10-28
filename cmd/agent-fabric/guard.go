package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ericmaster/agent-fabric/internal/guard"
)

func runGuard(args []string, input io.Reader, output, diagnostic io.Writer) int {
	response := guard.Error("invalid_request")
	logFile := guard.DefaultLogFile()
	fs := flag.NewFlagSet("guard check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&logFile, "log-file", logFile, "trusted absolute audit destination")
	if len(args) > 0 && args[0] == "check" && fs.Parse(args[1:]) == nil && len(fs.Args()) == 0 {
		body, err := io.ReadAll(io.LimitReader(input, guard.MaxInput+1))
		if err == nil {
			req, reason := guard.DecodeRequest(body)
			if reason != "" {
				response = guard.Error(reason)
			} else {
				response = guard.Check(req)
				if guard.AppendDenial(logFile, req.Source, response) != nil {
					fmt.Fprintln(diagnostic, "guard_audit_unavailable")
				}
			}
		}
	}
	if json.NewEncoder(output).Encode(response) != nil {
		return 2
	}
	switch response.Decision {
	case "allow":
		return 0
	case "deny":
		return 1
	}
	return 2
}

func guardMain(args []string) { os.Exit(runGuard(args, os.Stdin, os.Stdout, os.Stderr)) }
