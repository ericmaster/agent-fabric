// Package guard checks shell syntax and read-path metadata, never executing input.
package guard

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
)

const MaxInput = 64 << 10

type Request struct {
	Version int      `json:"version"`
	Op      string   `json:"op"`
	Command string   `json:"command"`
	Paths   []string `json:"paths"`
	Cwd     string   `json:"cwd"`
	Shell   string   `json:"shell"`
	Source  string   `json:"source"`
}

type Response struct {
	Version       int    `json:"version"`
	Decision      string `json:"decision"`
	Reason        string `json:"reason"`
	PolicyVersion int    `json:"policy_version"`
}

func result(decision, reason string) Response { return Response{1, decision, reason, 1} }

func Error(reason string) Response { return result("error", reason) }

func validSource(source string) bool {
	switch source {
	case "", "cli", "sdk", "pi", "opencode":
		return true
	}
	return false
}

// DecodeRequest accepts exactly one bounded object. Errors are fixed enums only.
func DecodeRequest(input []byte) (Request, string) {
	var req Request
	if len(input) > MaxInput {
		return req, "limit_exceeded"
	}
	var fields map[string]json.RawMessage
	d := json.NewDecoder(bytes.NewReader(input))
	if d.Decode(&fields) != nil || fields == nil {
		return req, "invalid_request"
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return req, "invalid_request"
	}
	for key, value := range fields {
		switch key {
		case "version", "op", "command", "paths", "cwd", "shell", "source":
		default:
			return req, "invalid_request"
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return req, "invalid_request"
		}
	}
	if json.Unmarshal(input, &req) != nil {
		return Request{}, "invalid_request"
	}
	if req.Op == "shell" {
		if _, ok := fields["command"]; !ok {
			return req, "invalid_request"
		}
	}
	return req, ""
}

func Check(req Request) Response {
	if req.Version != 1 || !filepath.IsAbs(req.Cwd) || strings.ContainsRune(req.Cwd, 0) || !validSource(req.Source) {
		return Error("invalid_request")
	}
	if len(req.Command)+len(req.Cwd) > MaxInput {
		return Error("limit_exceeded")
	}
	switch req.Op {
	case "shell":
		if req.Shell != "bash" && req.Shell != "posix" {
			return Error("unsupported_shell")
		}
		if len(req.Paths) != 0 || strings.ContainsRune(req.Command, 0) {
			return Error("invalid_request")
		}
		return (&policy{cwd: req.Cwd}).inspect(req.Command, req.Shell, 0, false)
	case "read":
		if len(req.Paths) == 0 || req.Command != "" || req.Shell != "" {
			return Error("invalid_request")
		}
		size := len(req.Cwd)
		for _, path := range req.Paths {
			size += len(path)
			if path == "" || strings.ContainsRune(path, 0) {
				return Error("invalid_request")
			}
		}
		if size > MaxInput {
			return Error("limit_exceeded")
		}
		for _, path := range req.Paths {
			if procPath(req.Cwd, path) {
				return result("deny", "proc_environ_read")
			}
		}
		return result("allow", "no_match")
	}
	return Error("invalid_request")
}
