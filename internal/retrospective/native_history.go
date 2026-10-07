// Spec: docs/specs/task-retrospective.md — OpenCode 2.0.24 history adapter.
package retrospective

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
)

type nativeHistoryTime struct {
	Created   int64 `json:"created"`
	Ran       int64 `json:"ran,omitempty"`
	Streamed  int64 `json:"streamed,omitempty"`
	Completed int64 `json:"completed,omitempty"`
}
type nativeHistoryModel struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID"`
	Variant    string `json:"variant,omitempty"`
}
type nativeHistoryRow struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Time     nativeHistoryTime  `json:"time"`
	Text     *string            `json:"text,omitempty"`
	Content  []json.RawMessage  `json:"content,omitempty"`
	Agent    string             `json:"agent,omitempty"`
	Model    nativeHistoryModel `json:"model,omitempty"`
	Outcome  string             `json:"outcome,omitempty"`
	Previous json.RawMessage    `json:"previous,omitempty"`
	Location struct {
		Directory   string `json:"directory"`
		WorkspaceID string `json:"workspaceID,omitempty"`
	} `json:"location,omitempty"`
	ProjectID       string            `json:"projectID,omitempty"`
	Subpath         string            `json:"subpath,omitempty"`
	Files           []json.RawMessage `json:"files,omitempty"`
	Agents          json.RawMessage   `json:"agents,omitempty"`
	Skills          json.RawMessage   `json:"skills,omitempty"`
	Finish          string            `json:"finish,omitempty"`
	RawFinish       string            `json:"rawFinish,omitempty"`
	Error           json.RawMessage   `json:"error,omitempty"`
	Retry           json.RawMessage   `json:"retry,omitempty"`
	Metadata        json.RawMessage   `json:"metadata,omitempty"`
	Snapshot        json.RawMessage   `json:"snapshot,omitempty"`
	ProviderState   json.RawMessage   `json:"providerState,omitempty"`
	Cost            json.RawMessage   `json:"cost,omitempty"`
	Tokens          json.RawMessage   `json:"tokens,omitempty"`
	Status          string            `json:"status,omitempty"`
	Reason          string            `json:"reason,omitempty"`
	Summary         *string           `json:"summary,omitempty"`
	Recent          *string           `json:"recent,omitempty"`
	ProviderContext json.RawMessage   `json:"providerContext,omitempty"`
}

// Reject unknown/malformed wire structure without exposing raw private content.
// Spec: docs/specs/task-retrospective.md via nativeHistory
func decodeNativeHistory(raw []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return errors.New("malformed native history structure")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("trailing native history content")
	}
	return nil
}

// Fetch complete pages before staging. Controls count against all budgets too.
// Spec: docs/specs/task-retrospective.md via HTTPSessionClient.Messages
func (c *HTTPSessionClient) nativeHistory(ctx context.Context, id string) ([]SessionMessageRow, error) {
	var wire []nativeHistoryRow
	cursor := ""
	seen := map[string]bool{}
	totalBytes := 0
	for page := 0; page < messagePageLimit; page++ {
		elem := "/session/" + url.PathEscape(id) + "/message"
		if cursor != "" {
			elem += "?cursor=" + url.QueryEscape(cursor)
		}
		raw, err := c.roundTrip(ctx, http.MethodGet, elem, nil)
		if err != nil {
			return nil, err
		}
		totalBytes += len(raw)
		if totalBytes > exchangeLimit {
			return nil, errors.New("native history exceeded byte budget")
		}
		var envelope struct {
			Data   *[]json.RawMessage `json:"data"`
			Cursor *struct {
				Next     json.RawMessage `json:"next"`
				Previous json.RawMessage `json:"previous,omitempty"`
			} `json:"cursor"`
		}
		if err := decodeNativeHistory(raw, &envelope); err != nil {
			return nil, err
		}
		if envelope.Data == nil || envelope.Cursor == nil || len(envelope.Cursor.Next) == 0 {
			return nil, errors.New("native history page missing data/cursor")
		}
		for _, rawRow := range *envelope.Data {
			var row nativeHistoryRow
			if err := decodeNativeHistory(rawRow, &row); err != nil {
				return nil, err
			}
			if row.ID == "" || row.Time.Created <= 0 {
				return nil, errors.New("native history row missing identity/time")
			}
			allowed := " id type time metadata "
			switch row.Type {
			case "user":
				allowed += "text files agents skills "
			case "assistant":
				allowed += "agent model content snapshot finish rawFinish providerState cost tokens error retry "
			case "idle":
				allowed += "outcome "
			case "agent-switched":
				allowed += "agent previous "
			case "model-switched":
				allowed += "model previous "
			case "location-switched":
				allowed += "location projectID subpath previous "
			case "compaction":
				allowed += "status reason model providerState summary recent providerContext cost tokens error "
			default:
				return nil, errors.New("native history row type unsupported; coverage incomplete")
			}
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(rawRow, &fields)
			for field := range fields {
				if !strings.Contains(allowed, " "+field+" ") {
					return nil, errors.New("native history row fields incompatible with type")
				}
			}
			wire = append(wire, row)
			if len(wire) > EnvelopeMaxRecords {
				return nil, errors.New("native history exceeded row budget")
			}
		}
		var next *string
		if err := json.Unmarshal(envelope.Cursor.Next, &next); err != nil {
			return nil, errors.New("native history cursor malformed")
		}
		if next == nil || *next == "" {
			break
		}
		if seen[*next] {
			return nil, errors.New("native history cursor repeated")
		}
		seen[*next] = true
		cursor = *next
		if page == messagePageLimit-1 {
			return nil, errors.New("session history exceeded the cursor page limit")
		}
	}
	sort.SliceStable(wire, func(i, j int) bool {
		if wire[i].Time.Created == wire[j].Time.Created {
			return wire[i].ID < wire[j].ID
		}
		return wire[i].Time.Created < wire[j].Time.Created
	})
	var rows []SessionMessageRow
	count := 0
	lastAssistant := -1
	controls := false
	var latestUser int64
	for _, row := range wire {
		if row.Type == "user" && row.Time.Created > latestUser {
			latestUser = row.Time.Created
		}
	}
	for _, row := range wire {
		normalized, err := normalizeNativeHistory(row, id)
		if err != nil {
			return nil, err
		}
		// A historical idle belongs to its earlier turn, not a later admitted user.
		// Existing staged parts keep their original seq; do not append stale errors
		// at the tail merely because backfill arrived after live observation.
		if row.Type == "idle" && row.Time.Created < latestUser {
			normalized.Events = nil
		}
		count += 1 + len(normalized.Parts) + len(normalized.Events)
		if count > EnvelopeMaxRecords {
			return nil, errors.New("native history exceeded normalized record budget")
		}
		rows = append(rows, normalized)
		if row.Type == "assistant" {
			lastAssistant = len(rows) - 1
		}
		controls = controls || normalized.Control != ""
	}
	if lastAssistant >= 0 {
		final := false
		for _, part := range rows[lastAssistant].Parts {
			if part.Type == "text" && !part.NonFinal && part.Text != "" {
				final = true
			}
		}
		if !final {
			if count == EnvelopeMaxRecords {
				return nil, errors.New("native history exceeded normalized record budget")
			}
			rows[lastAssistant].Events = append(rows[lastAssistant].Events, EnvelopeRecord{Kind: "removed", SessionID: id, RecordID: "native-final-gap", Detail: "native history lacks a completed final assistant answer"})
		}
	} else if controls {
		return nil, errors.New("native control history contains no assistant evidence")
	}
	return rows, nil
}

// Status/control rows have no message role. Never derive success from idle.
// Spec: docs/specs/task-retrospective.md via nativeHistory
func normalizeNativeHistory(wire nativeHistoryRow, id string) (SessionMessageRow, error) {
	row := SessionMessageRow{}
	row.Native = true
	row.Info.ID = wire.ID
	row.Info.SessionID = id
	event := func(kind, detail string) {
		row.Events = append(row.Events, EnvelopeRecord{Kind: "event", SessionID: id, Event: kind, Detail: detail})
	}
	bad := errors.New("native history row type/content malformed or unsupported")
	switch wire.Type {
	case "user":
		if wire.Text == nil || wire.Content != nil {
			return row, bad
		}
		if len(*wire.Text) > EnvelopeRecordMaxBytes {
			return row, errors.New("native user text exceeds record bound; coverage incomplete")
		}
		if len(wire.Files) > 0 || len(wire.Agents) > 0 || len(wire.Skills) > 0 {
			return row, errors.New("native user attachments/context unsupported; coverage incomplete")
		}
		row.Info.Role = "user"
		row.Parts = []SessionPartRow{{ID: wire.ID + ":text", SessionID: id, MessageID: wire.ID, Type: "text", Text: *wire.Text}}
	case "assistant":
		if wire.Content == nil || wire.Text != nil || wire.Agent == "" || wire.Model.ID == "" || wire.Model.ProviderID == "" {
			return row, bad
		}
		row.Info.Role = "assistant"
		for ordinal, raw := range wire.Content {
			part, err := nativeHistoryPart(raw, wire, id, ordinal)
			if err != nil {
				return row, err
			}
			row.Parts = append(row.Parts, part)
		}
		if wire.Time.Completed <= 0 || len(wire.Retry) > 0 {
			row.Events = append(row.Events, EnvelopeRecord{Kind: "removed", SessionID: id, RecordID: wire.ID + ":incomplete", Detail: "native assistant history incomplete/retrying"})
		}
		if len(wire.Error) > 0 && string(wire.Error) != "null" {
			row.Info.Error = wire.Error
			event("session.error", "native assistant failed")
		}
	case "idle", "agent-switched", "model-switched", "location-switched", "compaction":
		if wire.Text != nil || wire.Content != nil {
			return row, bad
		}
		row.Control = wire.Type
		switch wire.Type {
		case "idle":
			switch wire.Outcome {
			case "succeeded":
			case "failed":
				event("session.error", "native idle failed")
			case "interrupted":
				event("session.error", "MessageAbortedError")
			default:
				return row, bad
			}
			event("session.idle", "")
		case "agent-switched":
			if wire.Agent == "" {
				return row, bad
			}
			if len(wire.Previous) > 0 {
				var previous string
				if json.Unmarshal(wire.Previous, &previous) != nil || previous == "" {
					return row, bad
				}
			}
		case "model-switched":
			if wire.Model.ID == "" || wire.Model.ProviderID == "" {
				return row, bad
			}
			if len(wire.Previous) > 0 {
				var previous nativeHistoryModel
				if decodeNativeHistory(wire.Previous, &previous) != nil || previous.ID == "" || previous.ProviderID == "" {
					return row, bad
				}
			}
		case "location-switched":
			if !filepath.IsAbs(wire.Location.Directory) {
				return row, bad
			}
			row.Events = append(row.Events, EnvelopeRecord{Kind: "removed", SessionID: id, RecordID: wire.ID, Detail: "native task history changed execution location"})
		case "compaction":
			if wire.Status != "running" && wire.Status != "completed" && wire.Status != "failed" {
				return row, bad
			}
			if wire.Reason != "auto" && wire.Reason != "manual" {
				return row, bad
			}
			if wire.Status == "failed" && len(wire.Error) == 0 {
				return row, bad
			}
			if wire.Status != "failed" && (wire.Summary == nil || wire.Recent == nil) {
				return row, bad
			}
			event("session.compacted", "")
		}
	default:
		return row, bad
	}
	return row, nil
}

// Exact native content mapping: message-scoped tools; reasoning bodies discarded.
// Spec: docs/specs/task-retrospective.md via normalizeNativeHistory
func nativeHistoryPart(raw json.RawMessage, wire nativeHistoryRow, id string, ordinal int) (SessionPartRow, error) {
	var content struct {
		Type                string            `json:"type"`
		Text                *string           `json:"text,omitempty"`
		ID                  string            `json:"id,omitempty"`
		Name                string            `json:"name,omitempty"`
		State               json.RawMessage   `json:"state,omitempty"`
		Executed            *bool             `json:"executed,omitempty"`
		ProviderState       json.RawMessage   `json:"providerState,omitempty"`
		ProviderResultState json.RawMessage   `json:"providerResultState,omitempty"`
		Time                nativeHistoryTime `json:"time,omitempty"`
	}
	if err := decodeNativeHistory(raw, &content); err != nil {
		return SessionPartRow{}, err
	}
	part := SessionPartRow{SessionID: id, MessageID: wire.ID, Type: content.Type}
	bad := errors.New("native assistant content malformed or unsupported")
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	allowed := ""
	switch content.Type {
	case "text":
		allowed = " type text state "
	case "reasoning":
		allowed = " type text state time "
	case "tool":
		allowed = " type id name executed providerState providerResultState state time "
	default:
		return part, bad
	}
	for field := range fields {
		if !strings.Contains(allowed, " "+field+" ") {
			return part, bad
		}
	}
	switch content.Type {
	case "text":
		if content.Text == nil {
			return part, bad
		}
		if len(*content.Text) > EnvelopeRecordMaxBytes {
			return part, errors.New("native assistant text exceeds record bound; coverage incomplete")
		}
		part.ID = fmt.Sprintf("%s:text:%d", wire.ID, ordinal)
		part.Text = *content.Text
		part.NonFinal = wire.Time.Completed <= 0 || wire.Finish != "stop"
	case "reasoning":
		if content.Text == nil {
			return part, bad
		}
		part.ID = fmt.Sprintf("%s:reasoning:%d", wire.ID, ordinal) // never copy private text
	case "tool":
		if content.ID == "" || content.Name == "" || content.Time.Created <= 0 || content.Text != nil {
			return part, bad
		}
		part.ID = wire.ID + ":" + content.ID
		part.CallID = part.ID
		part.Tool = content.Name
		var state struct {
			Status  string          `json:"status"`
			Input   json.RawMessage `json:"input"`
			Content []struct {
				Type string  `json:"type"`
				Text *string `json:"text"`
			} `json:"content,omitempty"`
			Error    json.RawMessage `json:"error,omitempty"`
			Metadata json.RawMessage `json:"metadata,omitempty"`
		}
		if err := decodeNativeHistory(content.State, &state); err != nil {
			return part, err
		}
		if len(state.Input) == 0 {
			return part, bad
		}
		if len(state.Input) > EnvelopeRecordMaxBytes {
			return part, errors.New("native tool input exceeds record bound; coverage incomplete")
		}
		switch state.Status {
		case "streaming":
			var input string
			if json.Unmarshal(state.Input, &input) != nil {
				return part, bad
			}
		case "running", "completed", "error":
			var input map[string]json.RawMessage
			if json.Unmarshal(state.Input, &input) != nil || input == nil {
				return part, bad
			}
		default:
			return part, bad
		}
		var output []string
		for _, result := range state.Content {
			if result.Type != "text" || result.Text == nil {
				return part, bad
			}
			output = append(output, *result.Text)
		}
		if state.Status == "completed" && len(state.Content) == 0 {
			return part, bad
		}
		if state.Status == "error" {
			var failure struct {
				Type     string          `json:"type"`
				Message  *string         `json:"message"`
				Status   *int            `json:"status,omitempty"`
				Response json.RawMessage `json:"response,omitempty"`
			}
			if decodeNativeHistory(state.Error, &failure) != nil || failure.Type == "" || failure.Message == nil {
				return part, bad
			}
			publicError := failure.Type + ": " + *failure.Message
			if len(publicError) > EnvelopeErrorDetailBytes {
				return part, errors.New("native tool error exceeds observable detail bound; coverage incomplete")
			}
			state.Error, _ = json.Marshal(publicError)
		}
		if len(state.Metadata) > 0 {
			var metadata map[string]json.RawMessage
			if json.Unmarshal(state.Metadata, &metadata) != nil || metadata == nil {
				return part, bad
			}
			if value, ok := metadata["truncated"]; ok {
				var truncated bool
				if json.Unmarshal(value, &truncated) != nil {
					return part, bad
				}
				if truncated {
					return part, errors.New("native tool output truncated; coverage incomplete")
				}
			}
		}
		status := state.Status
		if status == "streaming" {
			status = "running"
		}
		if len(strings.Join(output, "\n")) > EnvelopeRecordMaxBytes {
			return part, errors.New("native tool output exceeds record bound; coverage incomplete")
		}
		encoded, err := json.Marshal(SessionToolState{Status: status, Input: state.Input, Output: strings.Join(output, "\n"), Error: state.Error})
		if err != nil {
			return part, bad
		}
		part.State = encoded
	default:
		return part, bad
	}
	return part, nil
}
