//go:build !linux

package guard

// Do not approximate Linux's no-follow, cross-process audit guarantees on an
// unverified platform. Decisions remain usable with the fixed audit warning.
func appendAudit(path string, line []byte) error { return errAudit }
