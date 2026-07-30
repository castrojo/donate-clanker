package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/projectbluefin/donate-clanker/internal/hive"
)

func TestWriteTaskObservationUsesOnlyAllowedMetadata(t *testing.T) {
	startedAt := time.Date(2026, time.July, 30, 1, 2, 3, 0, time.UTC)
	finishedAt := startedAt.Add(1500 * time.Millisecond)
	assignment := hive.Assignment{
		TaskID:      "task-123",
		Kind:        "issue",
		Repo:        "projectbluefin/donate-clanker",
		Number:      42,
		Title:       "private title",
		URL:         "https://example.invalid/private-url",
		Prompt:      "private prompt",
		GitHubToken: "private-token",
	}

	var output bytes.Buffer
	writeTaskObservation(&output, assignment, startedAt, finishedAt, context.Background(), errors.New("private error text"))
	if !strings.HasSuffix(output.String(), "\n") {
		t.Fatalf("observation is not newline-delimited: %q", output.String())
	}

	var observation map[string]any
	if err := json.Unmarshal(output.Bytes(), &observation); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	wantKeys := []string{
		"duration_ms",
		"event",
		"finished_at",
		"kind",
		"number",
		"outcome",
		"repo",
		"schema_version",
		"started_at",
		"task_id",
	}
	gotKeys := make([]string, 0, len(observation))
	for key := range observation {
		gotKeys = append(gotKeys, key)
	}
	sort.Strings(gotKeys)
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("observation keys = %v, want %v", gotKeys, wantKeys)
	}

	if got, want := observation["schema_version"], float64(observationSchemaVersion); got != want {
		t.Fatalf("schema_version = %v, want %v", got, want)
	}
	if got, want := observation["event"], taskFinishedEvent; got != want {
		t.Fatalf("event = %v, want %q", got, want)
	}
	if got, want := observation["duration_ms"], float64(1500); got != want {
		t.Fatalf("duration_ms = %v, want %v", got, want)
	}
	if got, want := observation["outcome"], "failure"; got != want {
		t.Fatalf("outcome = %v, want %q", got, want)
	}
	for key, want := range map[string]any{
		"task_id": "task-123",
		"kind":    "issue",
		"repo":    "projectbluefin/donate-clanker",
		"number":  float64(42),
	} {
		if got := observation[key]; got != want {
			t.Fatalf("%s = %v, want %v", key, got, want)
		}
	}

	for _, forbidden := range []string{
		"private title",
		"private-url",
		"private prompt",
		"private-token",
		"private error text",
		"title",
		"url",
		"prompt",
		"token",
		"summary",
		"output",
		"command",
		"error",
	} {
		if strings.Contains(output.String(), forbidden) {
			t.Errorf("observation contains forbidden value %q: %s", forbidden, output.String())
		}
	}
}

func TestWriteTaskObservationOutcomes(t *testing.T) {
	startedAt := time.Date(2026, time.July, 30, 1, 2, 3, 0, time.UTC)
	tests := []struct {
		name    string
		ctx     context.Context
		err     error
		outcome string
	}{
		{name: "success", ctx: context.Background(), outcome: "success"},
		{name: "failure", ctx: context.Background(), err: errors.New("goose failed"), outcome: "failure"},
		{name: "cancellation", ctx: cancelledContext(t), err: errors.New("redacted Goose cancellation"), outcome: "cancelled"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			writeTaskObservation(
				&output,
				hive.Assignment{TaskID: "task-123", Kind: "issue", Repo: "projectbluefin/donate-clanker", Number: 42},
				startedAt,
				startedAt.Add(time.Millisecond),
				test.ctx,
				test.err,
			)

			var observation taskObservation
			if err := json.Unmarshal(output.Bytes(), &observation); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if observation.Outcome != test.outcome {
				t.Fatalf("outcome = %q, want %q", observation.Outcome, test.outcome)
			}
		})
	}
}

func TestWriteTaskObservationIgnoresWriteFailures(t *testing.T) {
	writeTaskObservation(
		failingObservationWriter{},
		hive.Assignment{TaskID: "task-123", Kind: "issue", Repo: "projectbluefin/donate-clanker", Number: 42},
		time.Now(),
		time.Now(),
		context.Background(),
		nil,
	)
}

func cancelledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cancel()
	return ctx
}

type failingObservationWriter struct{}

func (failingObservationWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}
