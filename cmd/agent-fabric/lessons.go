// Spec: docs/specs/task-retrospective.md — Reusable lessons
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"

	"github.com/ericmaster/agent-fabric/internal/retrospective"
)

// runRetrospectiveLessons uses only explicit operator paths; no ambient config.
func runRetrospectiveLessons(args []string) error {
	if len(args) == 0 {
		return errors.New("lessons requires apply, read or status")
	}
	sub := args[0]
	if sub != "apply" && sub != "read" && sub != "status" {
		return errors.New("unknown lessons command")
	}
	fs := flag.NewFlagSet("retrospective lessons "+sub, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var state, trustPath, input, project, scope, version, consumer string
	var limit int
	fs.StringVar(&state, "state-dir", "", "explicit private retrospective store")
	fs.StringVar(&trustPath, "trust", "", "operator-enrolled reviewer/producer public keys")
	if sub == "apply" {
		fs.StringVar(&input, "input", "", "signed lesson envelope")
	}
	if sub == "read" {
		fs.StringVar(&project, "project", "", "exact execution root")
		fs.StringVar(&scope, "scope", "", "exact task scope")
		fs.StringVar(&version, "version", "", "exact compatibility identity")
		fs.StringVar(&consumer, "consumer", "", "executor or corrector")
		fs.IntVar(&limit, "limit", 10, "maximum lessons (1..20)")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 || !filepath.IsAbs(state) || filepath.Clean(state) != state {
		return errors.New("lessons requires clean absolute --state-dir and no positional arguments")
	}
	if err := retrospective.ValidateLessonDirectory(state); err != nil {
		return err
	}
	body, err := retrospective.ReadLessonFile(trustPath, 64<<10)
	if err != nil {
		return err
	}
	var trust retrospective.LessonTrust
	if err := retrospective.DecodeLessonJSON(body, &trust); err != nil {
		return err
	}
	if err := retrospective.ValidateLessonTrust(&trust); err != nil {
		return err
	}
	store := &retrospective.Store{Dir: state}
	var result any
	switch sub {
	case "apply":
		body, err = retrospective.ReadLessonFile(input, 64<<10)
		if err != nil {
			return err
		}
		var envelope retrospective.LessonEnvelope
		if err := retrospective.DecodeLessonJSON(body, &envelope); err != nil {
			return err
		}
		store, err = retrospective.OpenStore(state)
		if err != nil {
			return err
		}
		result, err = store.ApplyLesson(envelope, &trust)
	case "read":
		var lessons []retrospective.Lesson
		lessons, err = store.ReadLessons(&trust, project, scope, version, consumer, limit)
		result = struct {
			Advisory bool                   `json:"advisory"`
			Lessons  []retrospective.Lesson `json:"lessons"`
		}{true, lessons}
	case "status":
		result, err = store.LessonStatus(&trust)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
