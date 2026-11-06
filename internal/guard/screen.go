package guard

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultThreshold    = 0.99
	MaxScreenBody       = 64 << 10
	MaxScreenResponse   = 64 << 10
	MaxScreenCodePoints = 4096
	// MaxCombinedSubwords is the product limit counted by the pinned collator,
	// not a byte ceiling. schemaCollatorSubwords is the exact subword length of
	// the fixed schema prefix for this client.
	MaxCombinedSubwords    = 512
	schemaCollatorSubwords = 20
	ConnectTimeout         = 200 * time.Millisecond
	TotalTimeout           = time.Second

	StatusSkipped            = "skipped"
	StatusUntrusted          = "untrusted"
	StatusUncertain          = "uncertain"
	StatusError              = "error"
	StatusTimeout            = "timeout"
	StatusBusy               = "busy"
	StatusOversize           = "oversize"
	StatusMalformed          = "malformed"
	StatusMissingScore       = "missing_score"
	StatusInvalidScore       = "invalid_score"
	StatusMissingCredentials = "missing_credentials"
	StatusBelowThreshold     = "below_threshold"
	StatusSafe               = "safe"
	StatusModelDeny          = "model_deny"

	ReasonModelSensitive = "model_sensitive"
)

var timeNow = time.Now

// Label is a typed screening assessment. Unknown is not a denial.
type Label string

const (
	LabelSafe      Label = "safe"
	LabelSensitive Label = "sensitive"
	LabelUnknown   Label = "unknown"
)

// Assessment is one screener result. Confidence is nil when the provider did
// not return a native finite score. A nil score never denies.
type Assessment struct {
	Label      Label
	Confidence *float64
	Model      string
	Revision   string
	Status     string
}

// ScreenResult is the policy outcome. Pass statuses are not audit records.
type ScreenResult struct {
	Decision string
	Reason   string
	Status   string
	Model    string
	Revision string
	Layer    string
}

// Screener is the only screening extension point. There is no registry.
type Screener interface {
	Screen(ctx context.Context, text string) Assessment
}

// ApplyAssessment denies only a sensitive label with a finite native score in
// [0, 1] at or above threshold. Every other outcome allows. threshold 0 selects
// the default. An invalid threshold fails open and never denies.
func ApplyAssessment(a Assessment, threshold float64) ScreenResult {
	out := ScreenResult{Decision: "allow", Reason: "no_match", Model: a.Model, Revision: a.Revision}
	if status, ok := failOpenStatus(a.Status); ok {
		out.Status = status
		return out
	}
	if a.Status != "" {
		out.Status = StatusMalformed
		return out
	}
	switch a.Label {
	case LabelSafe, LabelSensitive, LabelUnknown, "":
	default:
		out.Status = StatusMalformed
		return out
	}
	if a.Confidence == nil {
		out.Status = StatusMissingScore
		return out
	}
	score := *a.Confidence
	if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > 1 {
		out.Status = StatusInvalidScore
		return out
	}
	if a.Label != LabelSensitive {
		if a.Label == LabelUnknown || a.Label == "" {
			out.Status = StatusUncertain
			return out
		}
		out.Status = StatusSafe
		return out
	}
	limit, ok := resolveThreshold(threshold)
	if !ok {
		out.Status = StatusError
		return out
	}
	if score < limit {
		out.Status = StatusBelowThreshold
		return out
	}
	out.Decision = "deny"
	out.Reason = ReasonModelSensitive
	out.Status = StatusModelDeny
	out.Layer = "model"
	return out
}

func failOpenStatus(status string) (string, bool) {
	switch status {
	case StatusUncertain, StatusError, StatusTimeout, StatusBusy, StatusOversize, StatusMalformed, StatusMissingScore, StatusInvalidScore, StatusMissingCredentials, StatusSkipped, StatusUntrusted:
		return status, true
	}
	return "", false
}

func resolveThreshold(threshold float64) (float64, bool) {
	if threshold == 0 {
		return DefaultThreshold, true
	}
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 1 {
		return 0, false
	}
	return threshold, true
}

// ScreenAfter applies deterministic deny before any screener call.
func ScreenAfter(ctx context.Context, policy Response, text string, screener Screener, threshold float64) (Response, ScreenResult) {
	if policy.Decision == "deny" {
		return policy, ScreenResult{Decision: "deny", Reason: policy.Reason, Status: "deterministic", Layer: "deterministic"}
	}
	if screener == nil {
		res := ApplyAssessment(Assessment{Status: StatusSkipped}, threshold)
		return withScreen(policy, res, text), res
	}
	res := ApplyAssessment(callScreener(ctx, screener, text), threshold)
	return withScreen(policy, res, text), res
}

// callScreener owns one absolute deadline captured before the callback starts.
// That same instant cancels the callback, fires the timer, and rejects a ready result.
func callScreener(ctx context.Context, screener Screener, text string) Assessment {
	deadline, ok := absoluteDeadline(ctx)
	if !ok {
		return Assessment{Status: StatusTimeout}
	}
	callCtx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	out := make(chan Assessment, 1)
	go func() {
		defer func() {
			if recover() != nil {
				select {
				case out <- Assessment{Status: StatusError}:
				default:
				}
			}
		}()
		select {
		case out <- screener.Screen(callCtx, text):
		default:
		}
	}()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	var parentDone <-chan struct{}
	if ctx != nil {
		parentDone = ctx.Done()
	}
	select {
	case assessment := <-out:
		if !admitCallback(timeNow(), deadline, ctx, callCtx) {
			return Assessment{Status: StatusTimeout}
		}
		return assessment
	case <-timer.C:
		return Assessment{Status: StatusTimeout}
	case <-parentDone:
		return Assessment{Status: StatusTimeout}
	case <-callCtx.Done():
		return Assessment{Status: StatusTimeout}
	}
}

func absoluteDeadline(ctx context.Context) (time.Time, bool) {
	now := timeNow()
	deadline := now.Add(TotalTimeout)
	if ctx != nil {
		if parent, ok := ctx.Deadline(); ok && parent.Before(deadline) {
			deadline = parent
		}
	}
	if !now.Before(deadline) {
		return time.Time{}, false
	}
	return deadline, true
}

func admitCallback(now, deadline time.Time, parent, callCtx context.Context) bool {
	if parent != nil && parent.Err() != nil {
		return false
	}
	if callCtx != nil && callCtx.Err() != nil {
		return false
	}
	return now.Before(deadline)
}

func withScreen(policy Response, res ScreenResult, secret string) Response {
	if res.Decision == "deny" {
		return Response{Version: 1, Decision: "deny", Reason: ReasonModelSensitive, PolicyVersion: 1, Status: res.Status, Model: publishToken(res.Model, secret), Revision: publishToken(res.Revision, secret)}
	}
	if policy.Version == 0 {
		policy = result("allow", "no_match")
	}
	policy.Status = res.Status
	policy.Model = publishToken(res.Model, secret)
	policy.Revision = publishToken(res.Revision, secret)
	return policy
}

type countingScreener struct {
	Assessment Assessment
	Calls      int
}

func (c *countingScreener) Screen(context.Context, string) Assessment {
	c.Calls++
	return c.Assessment
}

// Decide is the single HTTP adapter. The classify URL is read only from
// GLINER_DECIDE_API_URL. Header values are read from named environment
// variables at request time and are never returned.
type Decide struct {
	Enabled   bool
	Trusted   bool
	Threshold float64
	Roots     *x509.CertPool
	lookup    func(string) (string, bool)
}

func (d Decide) env(key string) string {
	lookup := d.lookup
	if lookup == nil {
		lookup = os.LookupEnv
	}
	value, _ := lookup(key)
	return value
}

func (d Decide) Screen(ctx context.Context, text string) Assessment {
	if !d.Enabled {
		return Assessment{Status: StatusSkipped}
	}
	if !d.Trusted {
		return Assessment{Status: StatusUntrusted}
	}
	endpoint, ok := classifyURL(d.env("GLINER_DECIDE_API_URL"))
	if !ok {
		return Assessment{Status: StatusError}
	}
	headers, ok := d.headerValues()
	if !ok {
		return Assessment{Status: StatusMissingCredentials}
	}
	if reason := screenLocalLimit(text); reason != "" {
		return Assessment{Status: reason}
	}
	body, err := classifyBody(text)
	if err != nil || len(body) > MaxScreenBody {
		return Assessment{Status: StatusOversize}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, TotalTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Assessment{Status: StatusError}
	}
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := d.client().Do(req)
	if err != nil {
		return Assessment{Status: transportStatus(err)}
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, MaxScreenResponse+1))
	if err != nil {
		return Assessment{Status: transportStatus(err)}
	}
	if len(payload) > MaxScreenResponse {
		return Assessment{Status: StatusOversize}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return Assessment{Status: StatusBusy}
	}
	if resp.StatusCode == http.StatusRequestEntityTooLarge {
		return Assessment{Status: StatusOversize}
	}
	if resp.StatusCode == http.StatusUnprocessableEntity {
		if budgetExceeded(payload) {
			return Assessment{Status: StatusOversize}
		}
		return Assessment{Status: StatusMalformed}
	}
	if resp.StatusCode != http.StatusOK {
		return Assessment{Status: StatusError}
	}
	label, model, revision, ok := parseDecide(payload)
	if !ok {
		return Assessment{Status: StatusMalformed}
	}
	return Assessment{Label: label, Model: publishToken(model, text), Revision: publishToken(revision, text)}
}

func (d Decide) client() *http.Client {
	dialer := &net.Dialer{Timeout: ConnectTimeout}
	transport := &http.Transport{
		Proxy:                 func(*http.Request) (*url.URL, error) { return nil, nil },
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   ConnectTimeout,
		ResponseHeaderTimeout: TotalTimeout,
		DisableKeepAlives:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: d.Roots},
	}
	return &http.Client{
		Timeout:       TotalTimeout,
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect_refused") },
	}
}

func classifyURL(raw string) (string, bool) {
	if raw == "" || strings.ContainsAny(raw, " \t\r\n") {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return raw, true
}

func (d Decide) headerValues() (map[string]string, bool) {
	spec := d.env("GLINER_DECIDE_HEADERS")
	if spec == "" {
		return nil, false
	}
	out := map[string]string{}
	for _, part := range strings.Split(spec, ",") {
		name, envName, ok := splitHeader(part)
		if !ok {
			return nil, false
		}
		value := d.env(envName)
		if value == "" || strings.ContainsRune(value, 0) {
			return nil, false
		}
		out[name] = value
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

var headerName = regexp.MustCompile(`^[!#$%&'*+.^_\x60|~0-9A-Za-z-]+$`)
var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

func splitHeader(part string) (string, string, bool) {
	name, env, ok := strings.Cut(strings.TrimSpace(part), "=")
	if !ok || !headerName.MatchString(name) || !envName.MatchString(env) {
		return "", "", false
	}
	return name, env, true
}

func screenLocalLimit(text string) string {
	if text == "" || !utf8.ValidString(text) {
		return StatusMalformed
	}
	if utf8.RuneCountInString(text) > MaxScreenCodePoints {
		return StatusOversize
	}
	if combinedSubwordBound(text) > MaxCombinedSubwords {
		return StatusOversize
	}
	return ""
}

// combinedSubwordBound upper-bounds collate_fn_inference original_lengths for
// printable ASCII only. The pinned Unigram tokenizer has byte_fallback false,
// NFC/whitespace normalization that does not expand ASCII, and a metaspace
// prepend. Each collator word is therefore at most one token per character plus
// the metaspace token. Two tokens per non-space character covers that worst
// case, including a collator-added period. Anything else fails open as oversize.
func combinedSubwordBound(text string) int {
	if !printableASCII(text) {
		return MaxCombinedSubwords + 1
	}
	prepared := text
	if strings.IndexByte(".!?", prepared[len(prepared)-1]) < 0 {
		prepared += "."
	}
	nonspace := 0
	for i := 0; i < len(prepared); i++ {
		if prepared[i] != ' ' {
			nonspace++
		}
	}
	return schemaCollatorSubwords + 2*nonspace
}

func printableASCII(text string) bool {
	if text == "" {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < 0x20 || text[i] > 0x7e {
			return false
		}
	}
	return true
}

func classifyBody(text string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(struct {
		Text   string         `json:"text"`
		Schema map[string]any `json:"schema"`
	}{text, map[string]any{
		"secret_disclosure": map[string]any{
			"labels":      []string{"sensitive", "safe", "uncertain"},
			"multi_label": false,
		},
	}})
	if err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func transportStatus(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return StatusTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return StatusTimeout
	}
	return StatusError
}

func budgetExceeded(payload []byte) bool {
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(payload, &body) != nil {
		return false
	}
	return body.Error.Code == "token_budget_exceeded"
}

func parseDecide(payload []byte) (Label, string, string, bool) {
	var fields map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	if dec.Decode(&fields) != nil {
		return "", "", "", false
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return "", "", "", false
	}
	raw, ok := fields["results"]
	if !ok {
		return "", "", "", false
	}
	var results map[string]json.RawMessage
	if json.Unmarshal(raw, &results) != nil {
		return "", "", "", false
	}
	labelRaw, ok := results["secret_disclosure"]
	if !ok {
		return "", "", "", false
	}
	var label string
	if json.Unmarshal(labelRaw, &label) != nil {
		return "", "", "", false
	}
	switch Label(label) {
	case LabelSafe, LabelSensitive, LabelUnknown:
	case "uncertain":
		label = string(LabelUnknown)
	default:
		return "", "", "", false
	}
	model, revision := jsonString(fields["model"]), jsonString(fields["revision"])
	return Label(label), model, revision, true
}

func jsonString(raw json.RawMessage) string {
	value, ok := jsonStringStrict(raw)
	if !ok {
		return ""
	}
	return value
}

func jsonStringStrict(raw json.RawMessage) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	// encoding/json accepts null into a string without error. Only a JSON string counts.
	if len(trimmed) < 2 || trimmed[0] != '"' {
		return "", false
	}
	var value string
	if json.Unmarshal(trimmed, &value) != nil {
		return "", false
	}
	return value, true
}

func jsonNumberStrict(raw json.RawMessage) (float64, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || (trimmed[0] != '-' && (trimmed[0] < '0' || trimmed[0] > '9')) {
		return 0, false
	}
	var value float64
	if json.Unmarshal(trimmed, &value) != nil {
		return 0, false
	}
	return value, true
}

func publishToken(value, secret string) string {
	if !tokenPattern.MatchString(value) {
		return ""
	}
	if secret != "" && (strings.Contains(value, secret) || strings.Contains(secret, value)) {
		return ""
	}
	return value
}

func screenRequest(req Request) Response {
	if req.Version != 1 || !absoluteCwd(req.Cwd) || !validSource(req.Source) || strings.ContainsRune(req.Text, 0) {
		return Error("invalid_request")
	}
	if req.Command != "" || req.Shell != "" || len(req.Paths) != 0 || req.Text == "" {
		return Error("invalid_request")
	}
	if len(req.Assessment) > 0 {
		assessment, reason := parseAssessment(req.Assessment)
		if reason != "" {
			return Error(reason)
		}
		return responseFrom(ApplyAssessment(assessment, envThreshold()), req.Text)
	}
	decide := Decide{Enabled: os.Getenv("AGF_GUARD_SCREEN") == "1", Trusted: os.Getenv("AGF_GUARD_SCREEN_TRUST") == "1", Threshold: envThreshold()}
	return responseFrom(ApplyAssessment(callScreener(context.Background(), decide, req.Text), decide.Threshold), req.Text)
}

func absoluteCwd(cwd string) bool {
	return filepath.IsAbs(cwd) && !strings.ContainsRune(cwd, 0)
}

func envThreshold() float64 {
	raw := strings.TrimSpace(os.Getenv("AGF_GUARD_SCREEN_THRESHOLD"))
	if raw == "" {
		return 0
	}
	value, ok := jsonNumberStrict([]byte(raw))
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 1 {
		return math.NaN()
	}
	return value
}

func parseAssessment(raw json.RawMessage) (Assessment, string) {
	var fields map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(raw))
	if dec.Decode(&fields) != nil || fields == nil {
		return Assessment{}, "invalid_request"
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return Assessment{}, "invalid_request"
	}
	for key := range fields {
		switch key {
		case "label", "confidence", "model", "revision", "status":
		default:
			return Assessment{}, "invalid_request"
		}
	}
	for _, key := range []string{"label", "model", "revision", "status"} {
		if value, ok := fields[key]; ok {
			if _, ok := jsonStringStrict(value); !ok {
				return Assessment{Status: StatusMalformed}, ""
			}
		}
	}
	if value, ok := fields["confidence"]; ok {
		if _, ok := jsonNumberStrict(value); !ok {
			return Assessment{Status: StatusMalformed}, ""
		}
	}
	var label string
	if value, ok := fields["label"]; ok {
		label, _ = jsonStringStrict(value)
	}
	model, _ := jsonStringStrict(fields["model"])
	revision, _ := jsonStringStrict(fields["revision"])
	assessment := Assessment{Label: Label(label), Model: model, Revision: revision}
	if value, ok := fields["status"]; ok {
		assessment.Status, _ = jsonStringStrict(value)
	}
	if value, ok := fields["confidence"]; ok {
		score, _ := jsonNumberStrict(value)
		assessment.Confidence = &score
	}
	return assessment, ""
}

func responseFrom(res ScreenResult, secret string) Response {
	out := result("allow", "no_match")
	out.Status = res.Status
	out.Model = publishToken(res.Model, secret)
	out.Revision = publishToken(res.Revision, secret)
	if res.Decision == "deny" {
		out.Decision = "deny"
		out.Reason = ReasonModelSensitive
		out.Status = StatusModelDeny
	}
	return out
}
