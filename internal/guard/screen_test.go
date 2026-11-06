package guard

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func scoreOf(v float64) *float64 { return &v }

func TestScreeningPolicy(t *testing.T) {
	high := 0.995
	cases := []struct {
		name   string
		assess Assessment
		limit  float64
		deny   bool
		status string
	}{
		{"below", Assessment{Label: LabelSensitive, Confidence: scoreOf(0.989), Model: "fake", Revision: "1"}, 0.99, false, StatusBelowThreshold},
		{"equal", Assessment{Label: LabelSensitive, Confidence: scoreOf(0.99), Model: "fake", Revision: "1"}, 0.99, true, StatusModelDeny},
		{"above", Assessment{Label: LabelSensitive, Confidence: scoreOf(high), Model: "fake", Revision: "1"}, 0, true, StatusModelDeny},
		{"safe-high", Assessment{Label: LabelSafe, Confidence: scoreOf(1), Model: "fake", Revision: "1"}, 0.99, false, StatusSafe},
		{"nan", Assessment{Label: LabelSensitive, Confidence: scoreOf(math.NaN())}, 0.99, false, StatusInvalidScore},
		{"inf", Assessment{Label: LabelSensitive, Confidence: scoreOf(math.Inf(1))}, 0.99, false, StatusInvalidScore},
		{"range", Assessment{Label: LabelSensitive, Confidence: scoreOf(1.1)}, 0.99, false, StatusInvalidScore},
		{"missing", Assessment{Label: LabelSensitive, Model: "label-only", Revision: "1"}, 0.99, false, StatusMissingScore},
		{"unknown", Assessment{Label: LabelUnknown, Confidence: scoreOf(1)}, 0.99, false, StatusUncertain},
		{"uncertain-status", Assessment{Status: StatusUncertain, Label: LabelSensitive, Confidence: scoreOf(1)}, 0.99, false, StatusUncertain},
		{"error", Assessment{Status: StatusError}, 0.99, false, StatusError},
		{"timeout", Assessment{Status: StatusTimeout}, 0.99, false, StatusTimeout},
		{"busy", Assessment{Status: StatusBusy}, 0.99, false, StatusBusy},
		{"oversize", Assessment{Status: StatusOversize}, 0.99, false, StatusOversize},
		{"malformed", Assessment{Status: StatusMalformed}, 0.99, false, StatusMalformed},
		{"bad-threshold", Assessment{Label: LabelSensitive, Confidence: scoreOf(1)}, math.NaN(), false, StatusError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &countingScreener{Assessment: tc.assess}
			policy := result("allow", "no_match")
			got, res := ScreenAfter(context.Background(), policy, "synthetic", fake, tc.limit)
			if fake.Calls != 1 {
				t.Fatalf("screener calls = %d", fake.Calls)
			}
			if tc.deny {
				if got.Decision != "deny" || got.Reason != ReasonModelSensitive || res.Layer != "model" {
					t.Fatalf("got %+v res %+v", got, res)
				}
			} else if got.Decision != "allow" {
				t.Fatalf("decision = %s", got.Decision)
			}
			if got.Status != tc.status || res.Status != tc.status {
				t.Fatalf("status = %s/%s; want %s", got.Status, res.Status, tc.status)
			}
		})
	}

	t.Run("deterministic-priority", func(t *testing.T) {
		fake := &countingScreener{Assessment: Assessment{Label: LabelSensitive, Confidence: scoreOf(1)}}
		policy := result("deny", "environment_dump")
		got, res := ScreenAfter(context.Background(), policy, "env", fake, 0.99)
		if fake.Calls != 0 {
			t.Fatal("deterministic deny used the provider")
		}
		if got.Decision != "deny" || got.Reason != "environment_dump" || got.Status != "" || res.Layer != "deterministic" {
			t.Fatalf("got %+v res %+v", got, res)
		}
	})

	t.Run("model-deny-audit-has-no-payload", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Chmod(dir, 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("XDG_STATE_HOME", dir)
		sentinel := "phase4-sentinel-value"
		fake := &countingScreener{Assessment: Assessment{Label: LabelSensitive, Confidence: scoreOf(0.995), Model: sentinel, Revision: "1"}}
		got, _ := ScreenAfter(context.Background(), result("allow", "no_match"), sentinel, fake, 0.99)
		path := filepath.Join(dir, "denials.jsonl")
		if err := AppendDenial(path, "sdk", got); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), sentinel) || strings.Contains(string(raw), "synthetic") {
			t.Fatal("audit contained payload")
		}
		var row map[string]any
		if json.Unmarshal(bytesTrim(raw), &row) != nil || len(row) != 6 || row["layer"] != "model" || row["rule"] != ReasonModelSensitive {
			t.Fatalf("audit row = %#v", row)
		}
		allow := result("allow", "no_match")
		allow.Status = StatusSafe
		before, _ := os.ReadFile(path)
		if err := AppendDenial(path, "sdk", allow); err != nil {
			t.Fatal(err)
		}
		after, _ := os.ReadFile(path)
		if string(before) != string(after) {
			t.Fatal("allow wrote an audit record")
		}
	})
}

func bytesTrim(raw []byte) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}

func TestDecideContract(t *testing.T) {
	var seen []byte
	var headers http.Header
	server, roots := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/classify" || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "nope", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		seen = append([]byte(nil), body...)
		headers = r.Header.Clone()
		_, _ = w.Write([]byte(`{"results":{"secret_disclosure":"sensitive"},"model":"decide","revision":"rev-1","inference_ms":0,"request_id":"req-1","confidence":0.999}`))
	})
	decide := trustedDecide(t, server.URL+"/v1/classify", roots)
	got := decide.Screen(context.Background(), "synthetic fixture")
	if got.Status != "" || got.Confidence != nil || got.Label != LabelSensitive || got.Model != "decide" || got.Revision != "rev-1" {
		t.Fatalf("assessment = %+v", got)
	}
	res := ApplyAssessment(got, 0.99)
	if res.Decision != "allow" || res.Status != StatusMissingScore {
		t.Fatalf("label-only result blocked: %+v", res)
	}
	var wire map[string]any
	if json.Unmarshal(seen, &wire) != nil {
		t.Fatalf("body = %s", seen)
	}
	if len(wire) != 2 || wire["text"] != "synthetic fixture" {
		t.Fatalf("wire = %#v", wire)
	}
	schema, _ := wire["schema"].(map[string]any)
	task, _ := schema["secret_disclosure"].(map[string]any)
	if task["multi_label"] != false {
		t.Fatalf("schema = %#v", schema)
	}
	if headers.Get("Authorization") != "synthetic-token" {
		t.Fatal("synthetic header was not forwarded")
	}
	response := responseFrom(res, "synthetic fixture")
	raw, _ := json.Marshal(response)
	if strings.Contains(string(raw), "synthetic fixture") || strings.Contains(string(raw), "0.999") {
		t.Fatalf("response leaked payload or fabricated score: %s", raw)
	}
}

func TestDecideTrust(t *testing.T) {
	hits := 0
	server, roots := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusNoContent)
	})
	url := server.URL + "/v1/classify"
	t.Setenv("GLINER_DECIDE_API_URL", url)
	t.Setenv("GLINER_DECIDE_HEADERS", "Authorization=GLINER_DECIDE_AUTHORIZATION")
	t.Setenv("GLINER_DECIDE_AUTHORIZATION", "synthetic-token")

	t.Run("off", func(t *testing.T) {
		before := hits
		got := (Decide{Enabled: false, Trusted: true, Roots: roots}).Screen(context.Background(), "synthetic")
		if got.Status != StatusSkipped || hits != before {
			t.Fatalf("status %s hits %d", got.Status, hits-before)
		}
	})
	t.Run("untrusted", func(t *testing.T) {
		before := hits
		got := (Decide{Enabled: true, Trusted: false, Roots: roots}).Screen(context.Background(), "synthetic")
		if got.Status != StatusUntrusted || hits != before {
			t.Fatalf("status %s hits %d", got.Status, hits-before)
		}
	})
	t.Run("missing-url", func(t *testing.T) {
		t.Setenv("GLINER_DECIDE_API_URL", "")
		before := hits
		got := (Decide{Enabled: true, Trusted: true, Roots: roots}).Screen(context.Background(), "synthetic")
		if got.Status != StatusError || hits != before {
			t.Fatalf("status %s hits %d", got.Status, hits-before)
		}
		t.Setenv("GLINER_DECIDE_API_URL", url)
	})
	t.Run("http-not-tls", func(t *testing.T) {
		plain := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
		t.Cleanup(plain.Close)
		t.Setenv("GLINER_DECIDE_API_URL", plain.URL+"/v1/classify")
		before := hits
		got := (Decide{Enabled: true, Trusted: true, Roots: roots}).Screen(context.Background(), "synthetic")
		if got.Status != StatusError || hits != before {
			t.Fatalf("status %s hits %d", got.Status, hits-before)
		}
		t.Setenv("GLINER_DECIDE_API_URL", url)
	})
	t.Run("missing-credentials", func(t *testing.T) {
		t.Setenv("GLINER_DECIDE_AUTHORIZATION", "")
		before := hits
		got := (Decide{Enabled: true, Trusted: true, Roots: roots}).Screen(context.Background(), "synthetic")
		if got.Status != StatusMissingCredentials || hits != before {
			t.Fatalf("status %s hits %d", got.Status, hits-before)
		}
		t.Setenv("GLINER_DECIDE_AUTHORIZATION", "synthetic-token")
	})
	t.Run("shell-check-does-not-call", func(t *testing.T) {
		t.Setenv("AGF_GUARD_SCREEN", "1")
		t.Setenv("AGF_GUARD_SCREEN_TRUST", "1")
		before := hits
		got := Check(Request{Version: 1, Op: "shell", Command: "printf ok", Cwd: t.TempDir(), Shell: "bash"})
		if got.Decision != "allow" || got.Status != "" || hits != before {
			t.Fatalf("shell check changed or called provider: %+v hits %d", got, hits-before)
		}
	})
	t.Run("enabled-one-request", func(t *testing.T) {
		before := hits
		got := trustedDecide(t, url, roots).Screen(context.Background(), "synthetic")
		if hits != before+1 || got.Status != StatusError {
			t.Fatalf("status %s hits %d", got.Status, hits-before)
		}
	})
	t.Run("redirect-not-followed", func(t *testing.T) {
		targetHits := 0
		target, _ := tlsServer(t, func(http.ResponseWriter, *http.Request) { targetHits++ })
		origin, originRoots := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL+"/v1/classify", http.StatusFound)
		})
		before := targetHits
		got := trustedDecide(t, origin.URL+"/v1/classify", originRoots).Screen(context.Background(), "synthetic")
		if targetHits != before || got.Status == StatusModelDeny || got.Label == LabelSensitive {
			t.Fatalf("redirect followed or denied: status %s target %d", got.Status, targetHits-before)
		}
		if got.Status != StatusError {
			t.Fatalf("redirect status = %s", got.Status)
		}
	})
	t.Run("no-implicit-proxy", func(t *testing.T) {
		proxyHits := 0
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			proxyHits++
			http.Error(w, "proxy", http.StatusBadGateway)
		}))
		t.Cleanup(proxy.Close)
		t.Setenv("HTTP_PROXY", proxy.URL)
		t.Setenv("HTTPS_PROXY", proxy.URL)
		t.Setenv("http_proxy", proxy.URL)
		t.Setenv("https_proxy", proxy.URL)
		before := hits
		_ = trustedDecide(t, url, roots).Screen(context.Background(), "synthetic")
		if proxyHits != 0 || hits != before+1 {
			t.Fatalf("proxy %d target %d", proxyHits, hits-before)
		}
	})
}

func TestDecideLimits(t *testing.T) {
	if utf8Count(strings.Repeat("a", MaxScreenCodePoints)) != MaxScreenCodePoints {
		t.Fatal("code point count drifted")
	}
	if screenLocalLimit(strings.Repeat("a", MaxScreenCodePoints+1)) != StatusOversize {
		t.Fatal("4097 code points were not oversize")
	}
	hits := 0
	server, roots := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		body, _ := io.ReadAll(r.Body)
		if len(body) > MaxScreenBody {
			t.Errorf("sent %d bytes", len(body))
		}
		_, _ = io.WriteString(w, `{"results":{"secret_disclosure":"safe"},"model":"decide","revision":"1"}`)
	})
	decide := trustedDecide(t, server.URL+"/v1/classify", roots)
	short := "synthetic"
	if screenLocalLimit(short) != "" {
		t.Fatal("short text should be sendable")
	}
	got := decide.Screen(context.Background(), short)
	if hits != 1 || got.Label != LabelSafe {
		t.Fatalf("short text status %s hits %d", got.Status, hits)
	}
	over := strings.Repeat("a", MaxCombinedSubwords)
	before := hits
	got = decide.Screen(context.Background(), over)
	if got.Status != StatusOversize || hits != before {
		t.Fatalf("over-ceiling status %s hits %d", got.Status, hits-before)
	}
	if combinedSubwordBound(over) < 55 {
		t.Fatal("bound undercounted authoritative 512-a token count")
	}
	at := strings.TrimRight(strings.Repeat("a ", 245), " ")
	overWords := strings.TrimRight(strings.Repeat("a ", 246), " ")
	if combinedSubwordBound(at) != 512 || screenLocalLimit(at) != "" {
		t.Fatalf("at-512 bound %d status %s", combinedSubwordBound(at), screenLocalLimit(at))
	}
	if combinedSubwordBound(overWords) != 514 || screenLocalLimit(overWords) != StatusOversize {
		t.Fatalf("over-512 bound %d status %s", combinedSubwordBound(overWords), screenLocalLimit(overWords))
	}
	before = hits
	got = decide.Screen(context.Background(), at)
	if hits != before+1 || got.Label != LabelSafe {
		t.Fatalf("at-512 was not sent: status %s hits %d", got.Status, hits-before)
	}
	before = hits
	got = decide.Screen(context.Background(), overWords)
	if got.Status != StatusOversize || hits != before {
		t.Fatalf("over-512 sent: status %s hits %d", got.Status, hits-before)
	}
	for _, text := range []string{"😀", "İ", "hello\n", "tab\there"} {
		before = hits
		if screenLocalLimit(text) != StatusOversize || combinedSubwordBound(text) <= MaxCombinedSubwords {
			t.Fatalf("%q bound %d status %s", text, combinedSubwordBound(text), screenLocalLimit(text))
		}
		got = decide.Screen(context.Background(), text)
		if got.Status != StatusOversize || hits != before {
			t.Fatalf("%q was sent: status %s hits %d", text, got.Status, hits-before)
		}
	}
	if combinedSubwordBound("[SEP_TEXT]") < 28 || screenLocalLimit("[SEP_TEXT]") != "" {
		t.Fatalf("special-token bound %d", combinedSubwordBound("[SEP_TEXT]"))
	}
	huge := strings.Repeat("a", MaxScreenBody+1)
	got = decide.Screen(context.Background(), huge)
	if got.Status != StatusOversize || hits != before {
		t.Fatalf("huge body was sent: %s hits %d", got.Status, hits-before)
	}
	before = hits
	server2, roots2 := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write(bytesRepeat(MaxScreenResponse + 8))
	})
	got = trustedDecide(t, server2.URL+"/v1/classify", roots2).Screen(context.Background(), short)
	if got.Status != StatusOversize || got.Label != "" {
		t.Fatalf("oversize response status %s label %s", got.Status, got.Label)
	}
}

func TestDecidePrivacy(t *testing.T) {
	sentinel := "phase4 sentinel/value"
	server, roots := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), sentinel) {
			t.Errorf("server did not receive the submitted text")
		}
		_, _ = io.WriteString(w, `{"results":{"secret_disclosure":"safe"},"model":"`+sentinel+`","revision":"rev-1"}`)
	})
	dir := t.TempDir()
	got := trustedDecide(t, server.URL+"/v1/classify", roots).Screen(context.Background(), sentinel)
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), sentinel) || strings.Contains(got.Status, sentinel) || got.Model != "" {
		t.Fatalf("assessment leaked submitted text: %s", raw)
	}
	response := Check(Request{Version: 1, Op: "screen", Text: sentinel, Cwd: dir, Source: "cli", Assessment: json.RawMessage(`{"label":"safe","confidence":1,"model":"fake","revision":"1"}`)})
	encoded, _ := json.Marshal(response)
	if strings.Contains(string(encoded), sentinel) {
		t.Fatalf("screen response leaked text: %s", encoded)
	}
	path := filepath.Join(dir, "denials.jsonl")
	if err := AppendDenial(path, "cli", response); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("allow created an audit file")
	}
}

func TestDecideDeadline(t *testing.T) {
	hits := 0
	server, roots := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	})
	start := time.Now()
	got := trustedDecide(t, server.URL+"/v1/classify", roots).Screen(context.Background(), "synthetic")
	elapsed := time.Since(start)
	if got.Status != StatusTimeout {
		t.Fatalf("status = %s", got.Status)
	}
	if elapsed > TotalTimeout+250*time.Millisecond {
		t.Fatalf("elapsed %s exceeded deadline+250ms", elapsed)
	}
	if hits != 1 {
		t.Fatalf("retries = %d", hits-1)
	}

	t.Run("connect", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ln.Close() })
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				time.Sleep(3 * time.Second)
				_ = conn.Close()
			}
		}()
		t.Setenv("GLINER_DECIDE_API_URL", "https://"+ln.Addr().String()+"/v1/classify")
		t.Setenv("GLINER_DECIDE_HEADERS", "Authorization=GLINER_DECIDE_AUTHORIZATION")
		t.Setenv("GLINER_DECIDE_AUTHORIZATION", "synthetic-token")
		start := time.Now()
		got := (Decide{Enabled: true, Trusted: true}).Screen(context.Background(), "synthetic")
		elapsed := time.Since(start)
		if got.Status != StatusTimeout && got.Status != StatusError {
			t.Fatalf("connect status = %s", got.Status)
		}
		if got.Label != "" || elapsed > TotalTimeout+250*time.Millisecond {
			t.Fatalf("connect elapsed %s status %s", elapsed, got.Status)
		}
	})

	t.Run("callback-deadline", func(t *testing.T) {
		orig := timeNow
		t.Cleanup(func() { timeNow = orig })
		fake := &deadlineScreener{}
		got, res := ScreenAfter(context.Background(), result("allow", "no_match"), "synthetic", fake, 0.99)
		if !fake.sawDeadline {
			t.Fatal("callback context had no deadline")
		}
		if got.Decision != "allow" || got.Status != StatusTimeout || res.Status != StatusTimeout {
			t.Fatalf("accepted result after callback deadline %+v %+v", got, res)
		}
		path := filepath.Join(t.TempDir(), "denials.jsonl")
		if err := AppendDenial(path, "cli", got); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("expired callback wrote an audit record")
		}
	})

	t.Run("canceled-parent", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		entered := make(chan struct{})
		release := make(chan struct{})
		fake := &holdScreener{entered: entered, release: release, Assessment: Assessment{Label: LabelSensitive, Confidence: scoreOf(1)}}
		var got Response
		var res ScreenResult
		done := make(chan struct{})
		go func() {
			got, res = ScreenAfter(ctx, result("allow", "no_match"), "synthetic", fake, 0.99)
			close(done)
		}()
		<-entered
		cancel()
		close(release)
		select {
		case <-done:
		case <-time.After(TotalTimeout + 250*time.Millisecond):
			t.Fatal("canceled parent did not return")
		}
		if got.Decision != "allow" || got.Status != StatusTimeout || res.Status != StatusTimeout {
			t.Fatalf("accepted canceled-parent result %+v %+v", got, res)
		}
		path := filepath.Join(t.TempDir(), "denials.jsonl")
		if err := AppendDenial(path, "cli", got); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("canceled parent wrote an audit record")
		}
	})

	t.Run("noncooperative-callback", func(t *testing.T) {
		release := make(chan struct{})
		finished := make(chan struct{})
		fake := &blockingScreener{release: release, finished: finished, Assessment: Assessment{Label: LabelSensitive, Confidence: scoreOf(1)}}
		var got Response
		var res ScreenResult
		done := make(chan struct{})
		start := time.Now()
		go func() {
			got, res = ScreenAfter(context.Background(), result("allow", "no_match"), "synthetic", fake, 0.99)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(TotalTimeout + 250*time.Millisecond):
			t.Fatal("noncooperative callback was not bounded")
		}
		elapsed := time.Since(start)
		if elapsed > TotalTimeout+250*time.Millisecond || got.Decision != "allow" || got.Status != StatusTimeout || res.Status != StatusTimeout {
			t.Fatalf("elapsed %s got %+v res %+v", elapsed, got, res)
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "denials.jsonl")
		if err := AppendDenial(path, "cli", got); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("timeout wrote an audit record")
		}
		close(release)
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("late callback did not finish")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("late denial wrote an audit record")
		}
	})

	t.Run("busy-no-retry", func(t *testing.T) {
		busyHits := 0
		busy, busyRoots := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
			busyHits++
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"code":"inference_busy","message":"busy"}}`)
		})
		got := trustedDecide(t, busy.URL+"/v1/classify", busyRoots).Screen(context.Background(), "synthetic")
		if got.Status != StatusBusy || busyHits != 1 {
			t.Fatalf("status %s hits %d", got.Status, busyHits)
		}
	})
}

func tlsServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *x509.CertPool) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	return server, pool
}

func trustedDecide(t *testing.T, endpoint string, roots *x509.CertPool) Decide {
	t.Helper()
	t.Setenv("GLINER_DECIDE_API_URL", endpoint)
	t.Setenv("GLINER_DECIDE_HEADERS", "Authorization=GLINER_DECIDE_AUTHORIZATION")
	t.Setenv("GLINER_DECIDE_AUTHORIZATION", "synthetic-token")
	t.Setenv("AGF_GUARD_SCREEN", "1")
	t.Setenv("AGF_GUARD_SCREEN_TRUST", "1")
	return Decide{Enabled: true, Trusted: true, Roots: roots}
}

type deadlineScreener struct {
	sawDeadline bool
}

func (d *deadlineScreener) Screen(ctx context.Context, _ string) Assessment {
	deadline, ok := ctx.Deadline()
	d.sawDeadline = ok
	if ok {
		captured := deadline
		timeNow = func() time.Time { return captured.Add(time.Nanosecond) }
	}
	return Assessment{Label: LabelSensitive, Confidence: scoreOf(1)}
}

type holdScreener struct {
	entered    chan struct{}
	release    chan struct{}
	Assessment Assessment
}

func (h *holdScreener) Screen(context.Context, string) Assessment {
	close(h.entered)
	<-h.release
	return h.Assessment
}

type blockingScreener struct {
	release    chan struct{}
	finished   chan struct{}
	Assessment Assessment
}

func (b *blockingScreener) Screen(context.Context, string) Assessment {
	<-b.release
	close(b.finished)
	return b.Assessment
}

func utf8Count(text string) int {
	n := 0
	for range text {
		n++
	}
	return n
}

func bytesRepeat(n int) []byte {
	return []byte(strings.Repeat("x", n))
}
