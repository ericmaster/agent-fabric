package guard

import (
	"path/filepath"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type policy struct {
	cwd   string
	nodes int
}

func merge(a, b Response) Response {
	if b.Decision == "deny" || (a.Decision == "allow" && b.Decision == "error") {
		return b
	}
	return a
}

func (p *policy) inspect(text, dialect string, depth int, captured bool) Response {
	if depth > 8 {
		return Error("limit_exceeded")
	}
	variant := syntax.LangBash
	if dialect == "posix" {
		variant = syntax.LangPOSIX
	}
	// Keep this parse's original bytes before the parser cooks document literals.
	sources := make(map[*syntax.Redirect]string)
	f, err := syntax.NewParser(syntax.Variant(variant)).Parse(strings.NewReader(text), "")
	if err != nil {
		return Error("parse_error")
	}
	syntax.Walk(f, func(n syntax.Node) bool {
		if n == nil || p.nodes > 4096 {
			return false
		}
		p.nodes++
		if r, ok := n.(*syntax.Redirect); ok {
			sources[r] = text
		}
		return true
	})
	if p.nodes > 4096 {
		return Error("limit_exceeded")
	}
	out := result("allow", "no_match")
	var parents []syntax.Node
	syntax.Walk(f, func(n syntax.Node) bool {
		if n == nil {
			parents = parents[:len(parents)-1]
			return true
		}
		var stmt *syntax.Stmt
		unsafeCapture := captured
		for _, ancestor := range parents {
			switch a := ancestor.(type) {
			case *syntax.Stmt:
				stmt = a
			case *syntax.CmdSubst, *syntax.ProcSubst:
				unsafeCapture = true
			case *syntax.BinaryCmd:
				if a.Op == syntax.Pipe || a.Op == syntax.PipeAll {
					unsafeCapture = true
				}
			}
		}
		switch node := n.(type) {
		case *syntax.Redirect:
			if node.Op == syntax.RdrIn || node.Op == syntax.RdrInOut {
				path, _ := word(node.Word, true)
				if procPath(p.cwd, path) {
					out = merge(out, result("deny", "proc_environ_read"))
				}
			}
			if quotedHeredoc(node) {
				// The pinned parser can misclassify mixed-quoted bodies as
				// expansions. Only the selected shell program reparses them.
				return false
			}
		case *syntax.CallExpr:
			out = merge(out, p.call(node.Args, stmt, depth, unsafeCapture, sources))
		}
		parents = append(parents, n)
		return true
	})
	if p.nodes > 4096 {
		return Error("limit_exceeded")
	}
	return out
}

// Word decoding is syntax-driven, not a shell tokenizer. Dynamic parts are
// never expanded; only path matching uses a wildcard placeholder for them.
func word(w *syntax.Word, loose bool) (string, bool) {
	if w == nil {
		return "", false
	}
	var parts func([]syntax.WordPart, bool) (string, bool)
	parts = func(ps []syntax.WordPart, quoted bool) (string, bool) {
		var b strings.Builder
		for _, part := range ps {
			switch v := part.(type) {
			case *syntax.Lit:
				s := v.Value
				for i := 0; i < len(s); i++ {
					if s[i] == '\\' && i+1 < len(s) && (!quoted || strings.ContainsRune("$`\"\\\n", rune(s[i+1]))) {
						i++
						if s[i] == '\n' {
							continue
						}
					}
					b.WriteByte(s[i])
				}
			case *syntax.SglQuoted:
				if v.Dollar {
					return "", false // ANSI-C quoting is outside the static string seam.
				}
				b.WriteString(v.Value)
			case *syntax.DblQuoted:
				s, ok := parts(v.Parts, true)
				if !ok {
					return "", false
				}
				b.WriteString(s)
			default:
				if !loose {
					return "", false
				}
				b.WriteByte('*')
			}
		}
		return b.String(), true
	}
	return parts(w.Parts, false)
}

func (p *policy) call(args []*syntax.Word, stmt *syntax.Stmt, depth int, captured bool, sources map[*syntax.Redirect]string) Response {
	allow := result("allow", "no_match")
	for len(args) > 0 {
		name, static := word(args[0], false)
		if !static {
			return allow
		}
		name = filepath.Base(name)
		switch name {
		case "command", "builtin", "exec":
			args = args[1:]
			if len(args) > 0 {
				if option, _ := word(args[0], false); option == "--" || option == "-p" {
					args = args[1:]
				}
			}
			continue
		case "env":
			i := 1
			options := true
			for i < len(args) {
				arg, ok := word(args[i], false)
				if !ok {
					// An assignment may contain substitutions; Walk checks these.
					if strings.Contains(args[i].Lit(), "=") {
						options = false
						i++
						continue
					}
					loose, _ := word(args[i], true)
					if strings.Contains(loose, "=") {
						options = false
						i++
						continue
					}
					return allow
				}
				if options && (arg == "--help" || arg == "--version") {
					return allow
				}
				if options && (arg == "-u" || arg == "--unset") {
					i += 2
					continue
				}
				if options && arg == "--" {
					options = false
					i++
					continue
				}
				if options && (arg == "-i" || arg == "-0" || arg == "--ignore-environment" || arg == "--null" || strings.HasPrefix(arg, "--unset=")) {
					i++
					continue
				}
				if options && strings.HasPrefix(arg, "-") {
					return allow // Exotic wrappers/options are outside v1 coverage.
				}
				if strings.Contains(arg, "=") {
					options = false // Assignments continue after the option terminator.
					i++
					continue
				}
				break
			}
			if i >= len(args) {
				return result("deny", "environment_dump")
			}
			args = args[i:]
			continue
		case "printenv":
			if !captured && stmt != nil && len(stmt.Redirs) == 1 {
				r := stmt.Redirs[0]
				path, ok := word(r.Word, false)
				if ok && path == "/dev/null" && r.Op == syntax.RdrOut && (r.N == nil || r.N.Value == "1") {
					return allow
				}
			}
			return result("deny", "environment_dump")
		case "set":
			if len(args) == 1 {
				return result("deny", "environment_dump")
			}
		case "eval":
			args = args[1:]
			if len(args) > 0 {
				if option, _ := word(args[0], false); option == "--" {
					args = args[1:]
				}
			}
			var code []string
			for _, arg := range args {
				s, ok := word(arg, false)
				if !ok {
					return Error("unsupported_dynamic_shell")
				}
				code = append(code, s)
			}
			return p.inspect(strings.Join(code, " "), "bash", depth+1, captured)
		case "sh", "bash":
			dialect := "posix"
			if name == "bash" {
				dialect = "bash"
			}
			stdin := false
			i := 1
			for ; i < len(args); i++ {
				arg, ok := word(args[i], false)
				if !ok || !strings.HasPrefix(arg, "-") {
					break
				}
				if arg == "--" {
					i++
					break
				}
				flags := strings.TrimPrefix(arg, "-")
				if flags == "" || strings.Trim(flags, "celsuxv") != "" {
					return allow // Unsupported shell options are not interpreted.
				}
				if strings.Contains(flags, "s") {
					stdin = true
				}
				if strings.Contains(flags, "c") {
					if i+1 >= len(args) {
						return Error("invalid_request")
					}
					code, ok := word(args[i+1], false)
					if !ok {
						return Error("unsupported_dynamic_shell")
					}
					return p.inspect(code, dialect, depth+1, captured)
				}
			}
			if (stdin || i == len(args)) && stmt != nil {
				return p.stdin(stmt.Redirs, dialect, depth, captured, sources)
			}
		case "cat", "head", "tail", "od", "xxd", "hexdump", "strings", "grep", "sed", "awk", "less", "more", "dd":
			for _, arg := range args[1:] {
				path, _ := word(arg, true)
				if name == "dd" {
					path = strings.TrimPrefix(path, "if=")
				}
				if procPath(p.cwd, path) {
					return result("deny", "proc_environ_read")
				}
			}
		}
		return allow
	}
	return allow
}

// Project only this call's descriptor sources. Duplication snapshots the source
// at that point; later replacement of that descriptor cannot change its copy.
// No files are opened, and only the final static fd0 body is parsed as a program.
func (p *policy) stdin(redirs []*syntax.Redirect, dialect string, depth int, captured bool, sources map[*syntax.Redirect]string) Response {
	type source struct {
		redirect *syntax.Redirect
		text     string
		closed   bool
	}
	fds := map[int]source{} // Missing sources are inherited/unknown, not empty.
	selected := false
	for _, r := range redirs {
		dest := 1
		switch r.Op {
		case syntax.RdrIn, syntax.RdrInOut, syntax.DplIn, syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
			dest = 0
		case syntax.RdrAll, syntax.AppAll:
			fds[1], fds[2] = source{}, source{}
			continue
		}
		if r.N != nil {
			n, err := strconv.Atoi(r.N.Value)
			if err != nil {
				continue // Named descriptor allocation is not a numeric fd0 target.
			}
			dest = n
		}
		if dest == 0 {
			selected = true
		}
		switch r.Op {
		case syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
			fds[dest] = source{redirect: r, text: sources[r]}
		case syntax.DplIn, syntax.DplOut:
			operand, static := word(r.Word, false)
			if static && operand == "-" {
				fds[dest] = source{closed: true}
				continue
			}
			from, err := strconv.Atoi(strings.TrimSuffix(operand, "-"))
			if !static || err != nil {
				fds[dest] = source{}
				continue
			}
			fds[dest] = fds[from]
			if strings.HasSuffix(operand, "-") {
				fds[from] = source{closed: true}
				if from == 0 {
					selected = true
				}
			}
		default:
			fds[dest] = source{} // File/dynamic source: never read its contents.
		}
	}
	input := fds[0]
	if !selected || input.closed {
		return result("allow", "no_match")
	}
	code, static := stdinProgram(input.redirect, input.text)
	if !static {
		return Error("unsupported_dynamic_shell")
	}
	return p.inspect(code, dialect, depth+1, captured)
}

func quotedHeredoc(r *syntax.Redirect) bool {
	if r.Op != syntax.Hdoc && r.Op != syntax.DashHdoc {
		return false
	}
	for _, part := range r.Word.Parts {
		switch v := part.(type) {
		case *syntax.Lit:
			if strings.Contains(v.Value, "\\") {
				return true
			}
		case *syntax.SglQuoted, *syntax.DblQuoted:
			return true
		}
	}
	return false
}

// Decode original document bytes, not normalized AST literal values. Descriptor
// copies retain both the redirect and its own parse's immutable source text.
func stdinProgram(r *syntax.Redirect, text string) (string, bool) {
	if r == nil {
		return "", false // Unknown fd source, distinct from an empty document.
	}
	if r.Op == syntax.WordHdoc {
		code, static := word(r.Word, false)
		return code + "\n", static
	}
	if r.Hdoc == nil {
		return "", true
	}
	quoted := quotedHeredoc(r)
	if !quoted {
		for _, part := range r.Hdoc.Parts {
			if _, ok := part.(*syntax.Lit); !ok {
				return "", false // No evaluation of document expansions.
			}
		}
	}
	start, end := int(r.Hdoc.Pos().Offset()), int(r.Hdoc.End().Offset())
	lower := int(r.Word.End().Offset())
	if start < lower || end < start || end > len(text) {
		return "", false
	}
	// An initial unquoted continuation can be omitted even from Hdoc.Pos.
	// Reattach only that exact adjacent source prefix, bounded by this redirect.
	for start >= lower+2 && text[start-2:start] == "\\\n" {
		start -= 2
	}
	s := text[start:end]
	// In v3.8.0 Hdoc.End includes the closing delimiter line, not just the body.
	if boundary := strings.LastIndexByte(s, '\n'); boundary >= 0 {
		s = s[:boundary+1]
	} else {
		s = "" // Empty tab-stripped body followed immediately by its delimiter.
	}
	if r.Op == syntax.DashHdoc {
		lines := strings.Split(s, "\n")
		for i := range lines {
			lines[i] = strings.TrimLeft(lines[i], "\t")
		}
		s = strings.Join(lines, "\n")
	}
	if quoted {
		return s, true
	}
	var code strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && strings.ContainsRune("\\$`\n", rune(s[i+1])) {
			i++
			if s[i] == '\n' {
				continue
			}
		}
		code.WriteByte(s[i])
	}
	return code.String(), true
}
