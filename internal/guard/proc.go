package guard

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var procEnviron = regexp.MustCompile(`^/proc/[^/]+/(task/[^/]+/)?environ$`)

// Preserve the submitted component order: a symlink must be resolved before a
// following "..". Lexical proc matches still deny even for nonexistent targets.
// Only Lstat/Readlink metadata is used; no target contents or glob expansion.
func procPath(cwd, path string) bool {
	if !filepath.IsAbs(path) {
		path = cwd + string(filepath.Separator) + path
	}
	sep := string(filepath.Separator)
	volume := filepath.VolumeName(path)
	root := volume + sep
	prefix := root
	parts := strings.Split(strings.TrimPrefix(path, volume), sep)
	links := 0
	for steps := 0; steps < 8192 && len(parts) > 0; steps++ {
		// Cleaning here is deny-only: never use its result to traverse metadata.
		if procEnviron.MatchString(filepath.ToSlash(filepath.Clean(prefix + sep + strings.Join(parts, sep)))) {
			return true
		}
		part := parts[0]
		parts = parts[1:]
		switch part {
		case "", ".":
			continue
		case "..":
			prefix = filepath.Dir(prefix)
			continue
		}
		candidate := filepath.Join(prefix, part) // A single non-dot component only.
		info, err := os.Lstat(candidate)
		if err != nil {
			return false
		}
		if info.Mode()&os.ModeSymlink == 0 {
			prefix = candidate
			continue
		}
		links++
		if links > 40 {
			return false // The OS cannot follow this chain for a content read either.
		}
		target, err := os.Readlink(candidate)
		if err != nil {
			return false
		}
		if filepath.IsAbs(target) {
			prefix = root
			target = strings.TrimPrefix(target, volume)
		}
		parts = append(strings.Split(target, sep), parts...)
	}
	return procEnviron.MatchString(filepath.ToSlash(prefix))
}
