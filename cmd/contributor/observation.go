package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/projectbluefin/donate-clanker/internal/hive"
)

const (
	observationSchemaVersion = 1
	taskFinishedEvent        = "task_finished"
)

type taskObservation struct {
	SchemaVersion int       `json:"schema_version"`
	Event         string    `json:"event"`
	TaskID        string    `json:"task_id"`
	Kind          string    `json:"kind"`
	Repo          string    `json:"repo"`
	Number        int       `json:"number"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	DurationMS    int64     `json:"duration_ms"`
	Outcome       string    `json:"outcome"`
}

func writeTaskObservation(writer io.Writer, assignment hive.Assignment, startedAt, finishedAt time.Time, runCtx context.Context, runErr error) {
	if writer == nil {
		return
	}

	observation := taskObservation{
		SchemaVersion: observationSchemaVersion,
		Event:         taskFinishedEvent,
		TaskID:        assignment.TaskID,
		Kind:          assignment.Kind,
		Repo:          assignment.Repo,
		Number:        assignment.Number,
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
		DurationMS:    finishedAt.Sub(startedAt).Milliseconds(),
		Outcome:       observationOutcome(runCtx, runErr),
	}
	_ = json.NewEncoder(writer).Encode(observation)
}

func observationOutcome(runCtx context.Context, runErr error) string {
	if errors.Is(runCtx.Err(), context.Canceled) || errors.Is(runErr, context.Canceled) {
		return "cancelled"
	}
	if runErr != nil {
		return "failure"
	}
	return "success"
}
