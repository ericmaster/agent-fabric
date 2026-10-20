//go:build linux

// Spec: docs/specs/task-retrospective.md — Standing maintenance authority.
package retrospective

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestPiMaintenanceAnchoredRaces(t *testing.T) {
	for _, kind := range []string{"new-destination", "parent-alias", "existing-destination", "external-writer"} {
		t.Run(kind, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			parent := filepath.Join(root, "src")
			if err := os.Mkdir(parent, 0700); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(parent, "value.txt")
			image := piMaintenanceImage{Path: "src/value.txt", Exists: kind != "new-destination", Before: []byte("before"), After: []byte("after"), Mode: 0600}
			if image.Exists {
				if err := os.WriteFile(file, image.Before, 0600); err != nil {
					t.Fatal(err)
				}
			}
			foreign := filepath.Join(outside, "value.txt")
			if err := os.WriteFile(foreign, []byte("outside"), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var child *exec.Cmd
			breaks := make(chan os.Signal, 1)
			signal.Notify(breaks, syscall.SIGIO)
			defer signal.Stop(breaks)
			hook := func(stage string) {
				if kind == "new-destination" && stage == "new-file-after-check" {
					if err := os.WriteFile(file, []byte("foreign"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if kind == "parent-alias" && stage == "existing-file-before-write" {
					if err := os.Rename(parent, filepath.Join(root, "saved")); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(outside, parent); err != nil {
						t.Fatal(err)
					}
				}
				if kind == "existing-destination" && stage == "existing-file-before-write" {
					if err := os.Rename(file, file+".saved"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(file, []byte("foreign"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if kind == "external-writer" && stage == "existing-file-before-write" {
					child = exec.CommandContext(ctx, "sh", "-c", `printf foreign > "$1"`, "controlled-lease-test", file)
					child.Env = []string{"PATH=/usr/bin:/bin"}
					if err := child.Start(); err != nil {
						t.Fatal(err)
					}
					select {
					case <-breaks:
					case <-ctx.Done():
						t.Fatal("kernel did not report the competing writer")
					}
				}
			}
			err := writePiMaintenanceAnchored(root, image, hook)
			if err == nil {
				t.Fatal("racing writer/ancestor was accepted")
			}
			if child != nil {
				if err := child.Wait(); err != nil {
					t.Fatal(err)
				}
			}
			body, _ := os.ReadFile(foreign)
			if string(body) != "outside" {
				t.Fatal("ancestor alias redirected a source write")
			}
			if kind != "parent-alias" {
				body, _ = os.ReadFile(file)
				if string(body) != "foreign" {
					t.Fatal("foreign destination/writer was overwritten")
				}
			}
		})
	}
}
