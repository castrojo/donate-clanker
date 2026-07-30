package hive

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestLoadCredentialsDerivesContributorWebSocketURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "contributor.env")
	content := strings.Join([]string{
		"HIVE_REGISTRATION_TOKEN=secret-token",
		"HIVE_HUB=wss://example.hive.kubestellar.io/contribute",
		"CONTRIBUTOR_ID=c-123",
		"CONTRIBUTOR_USERNAME=tester",
		"AGENT_BACKEND=goose",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	creds, err := LoadCredentials(path, map[string]string{"GOOSE_MODEL": "Qwen3.6-35B-A3B"})
	if err != nil {
		t.Fatalf("LoadCredentials() error = %v", err)
	}

	if creds.WSURL != "wss://example.hive.kubestellar.io/api/contribute/ws" {
		t.Fatalf("LoadCredentials().WSURL = %q", creds.WSURL)
	}
	if creds.Model != "Qwen3.6-35B-A3B" {
		t.Fatalf("LoadCredentials().Model = %q", creds.Model)
	}
	if creds.CLIBackend != "goose" {
		t.Fatalf("LoadCredentials().CLIBackend = %q", creds.CLIBackend)
	}
}

func TestLoadCredentialsAcceptsExplicitWebSocketOverride(t *testing.T) {
	creds, err := LoadCredentials("", map[string]string{
		"HIVE_REGISTRATION_TOKEN": "secret-token",
		"HIVE_WS_URL":             "wss://example.hive.kubestellar.io/api/contribute/ws",
		"AGENT_BACKEND":           "goose",
	})
	if err != nil {
		t.Fatalf("LoadCredentials() error = %v", err)
	}
	if creds.WSURL != "wss://example.hive.kubestellar.io/api/contribute/ws" {
		t.Fatalf("LoadCredentials().WSURL = %q", creds.WSURL)
	}
}

func TestLoadCredentialsRejectsUnencryptedWebSocketURLs(t *testing.T) {
	tests := map[string]map[string]string{
		"HIVE_HUB": {
			"HIVE_REGISTRATION_TOKEN": "secret-token",
			"HIVE_HUB":                "ws://example.hive.kubestellar.io/contribute",
			"AGENT_BACKEND":           "goose",
		},
		"HIVE_WS_URL": {
			"HIVE_REGISTRATION_TOKEN": "secret-token",
			"HIVE_WS_URL":             "ws://example.hive.kubestellar.io/api/contribute/ws",
			"AGENT_BACKEND":           "goose",
		},
	}

	for name, env := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := LoadCredentials("", env)
			if err == nil {
				t.Fatal("LoadCredentials() error = nil, want error")
			}
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("LoadCredentials() error = %v, want ErrInvalidCredentials", err)
			}
			if !strings.Contains(err.Error(), "use wss") {
				t.Fatalf("LoadCredentials() error = %q, want wss detail", err)
			}
		})
	}
}

func TestClientReturnsAuthFailureWithAcceptedModels(t *testing.T) {
	wsURL, conns, cleanup := newWSTestServer(t)
	defer cleanup()

	client := NewClient()
	client.ReconnectDelay = 10 * time.Millisecond
	client.MaxReconnectDelay = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Run(ctx, Credentials{
			RegistrationToken: "secret-token",
			WSURL:             wsURL,
			CLIBackend:        "goose",
			Model:             "local",
		}, &blockingHandler{})
	}()

	conn := <-conns
	writeWSMessage(t, conn, wsMessage{Type: "auth_challenge", Seq: 1, Nonce: "nonce-1"})
	auth := readWSMessage(t, conn)
	if auth.Type != "auth_response" {
		t.Fatalf("auth message type = %q, want auth_response", auth.Type)
	}
	if auth.RegistrationToken != "secret-token" {
		t.Fatalf("auth token = %q", auth.RegistrationToken)
	}
	if auth.CLIBackend != "goose" {
		t.Fatalf("auth backend = %q", auth.CLIBackend)
	}
	writeWSMessage(t, conn, wsMessage{
		Type:           "auth_failed",
		Reason:         "model rejected",
		AcceptedModels: []string{"claude-*", "gpt-4o"},
	})

	err := <-errCh
	if !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("Run() error = %v, want ErrAuthenticationFailed", err)
	}
	if !strings.Contains(err.Error(), "claude-*") {
		t.Fatalf("Run() error = %q, want accepted models detail", err)
	}
}

func TestClientRespondsToPingAndCompletesTask(t *testing.T) {
	wsURL, conns, cleanup := newWSTestServer(t)
	defer cleanup()

	handler := newBlockingHandler()
	client := NewClient()
	client.ReconnectDelay = 10 * time.Millisecond
	client.MaxReconnectDelay = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Run(ctx, Credentials{
			RegistrationToken: "secret-token",
			WSURL:             wsURL,
			CLIBackend:        "goose",
			Model:             "Qwen3.6-35B-A3B",
		}, handler)
	}()

	conn := <-conns
	authenticateConnection(t, conn)

	ready := readWSMessage(t, conn)
	if ready.Type != "ready" {
		t.Fatalf("message after auth_ok = %q, want ready", ready.Type)
	}

	writeWSMessage(t, conn, wsMessage{
		Type:           "task_assign",
		TaskID:         "task-123",
		Kind:           "issue",
		Repo:           "projectbluefin/common",
		Number:         7,
		Title:          "Fix the bug",
		URL:            "https://github.com/projectbluefin/common/issues/7",
		Prompt:         "Follow the assignment exactly.",
		GitHubToken:    "ghs_assignment_token",
		TokenExpiresAt: time.Now().Add(55 * time.Minute).UTC().Format(time.RFC3339),
	})

	accepted := readWSMessage(t, conn)
	if accepted.Type != "task_accepted" || accepted.TaskID != "task-123" {
		t.Fatalf("accepted = %+v", accepted)
	}

	started := <-handler.started
	if started.TaskID != "task-123" || started.Repo != "projectbluefin/common" || started.Number != 7 {
		t.Fatalf("started assignment = %+v", started)
	}
	if started.GitHubToken != "ghs_assignment_token" {
		t.Fatalf("started token = %q", started.GitHubToken)
	}

	writeWSMessage(t, conn, wsMessage{Type: "ping", Seq: 42})
	pong := readWSMessage(t, conn)
	if pong.Type != "pong" || pong.Seq != 42 {
		t.Fatalf("pong = %+v", pong)
	}

	handler.finish <- handlerOutcome{
		report: TaskReport{
			Result:  "completed",
			Summary: "goose completed the task",
			Output:  []string{"last output line"},
		},
	}

	completed := readWSMessage(t, conn)
	if completed.Type != "task_complete" || completed.TaskID != "task-123" {
		t.Fatalf("completed = %+v", completed)
	}
	if completed.Result != "completed" {
		t.Fatalf("task_complete result = %q", completed.Result)
	}
	if completed.Summary != "goose completed the task" {
		t.Fatalf("task_complete summary = %q", completed.Summary)
	}
	if len(completed.TmuxOutput) != 1 || completed.TmuxOutput[0] != "last output line" {
		t.Fatalf("task_complete tmux_output = %#v", completed.TmuxOutput)
	}

	ready = readWSMessage(t, conn)
	if ready.Type != "ready" {
		t.Fatalf("message after task_complete = %q, want ready", ready.Type)
	}

	cancel()
	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func TestClientReconnectsAndRestoresActiveTaskIdentity(t *testing.T) {
	wsURL, conns, cleanup := newWSTestServer(t)
	defer cleanup()

	handler := newBlockingHandler()
	client := NewClient()
	client.ReconnectDelay = 10 * time.Millisecond
	client.MaxReconnectDelay = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Run(ctx, Credentials{
			RegistrationToken: "secret-token",
			WSURL:             wsURL,
			CLIBackend:        "goose",
			Model:             "local",
		}, handler)
	}()

	conn1 := <-conns
	authenticateConnection(t, conn1)
	if ready := readWSMessage(t, conn1); ready.Type != "ready" {
		t.Fatalf("message after auth_ok = %q, want ready", ready.Type)
	}

	writeWSMessage(t, conn1, wsMessage{
		Type:   "task_assign",
		TaskID: "task-456",
		Kind:   "issue",
		Repo:   "projectbluefin/bluefin",
		Number: 22,
		Title:  "Repair reconnect handling",
		Prompt: "Keep working through reconnects.",
		URL:    "https://github.com/projectbluefin/bluefin/issues/22",
	})
	if accepted := readWSMessage(t, conn1); accepted.Type != "task_accepted" {
		t.Fatalf("task_accepted = %+v", accepted)
	}
	<-handler.started
	_ = conn1.Close()

	conn2 := <-conns
	authenticateConnection(t, conn2)

	resumeAccepted := readWSMessage(t, conn2)
	if resumeAccepted.Type != "task_accepted" || resumeAccepted.TaskID != "task-456" {
		t.Fatalf("resume task_accepted = %+v", resumeAccepted)
	}

	progress := readWSMessage(t, conn2)
	if progress.Type != "task_progress" {
		t.Fatalf("resume progress = %+v", progress)
	}
	if progress.TaskID != "task-456" || progress.Kind != "issue" || progress.Repo != "projectbluefin/bluefin" || progress.Number != 22 || progress.Title != "Repair reconnect handling" {
		t.Fatalf("resume progress identity = %+v", progress)
	}

	handler.finish <- handlerOutcome{
		report: TaskReport{
			Result:  "completed",
			Summary: "done after reconnect",
		},
	}

	completed := readWSMessage(t, conn2)
	if completed.Type != "task_complete" || completed.TaskID != "task-456" {
		t.Fatalf("completed = %+v", completed)
	}
	if ready := readWSMessage(t, conn2); ready.Type != "ready" {
		t.Fatalf("message after completion = %+v", ready)
	}

	cancel()
	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func TestClientQueuesCompletionUntilReconnect(t *testing.T) {
	wsURL, conns, cleanup := newWSTestServer(t)
	defer cleanup()

	handler := newBlockingHandler()
	client := NewClient()
	client.ReconnectDelay = 10 * time.Millisecond
	client.MaxReconnectDelay = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Run(ctx, Credentials{
			RegistrationToken: "secret-token",
			WSURL:             wsURL,
			CLIBackend:        "goose",
		}, handler)
	}()

	conn1 := <-conns
	authenticateConnection(t, conn1)
	if ready := readWSMessage(t, conn1); ready.Type != "ready" {
		t.Fatalf("message after auth_ok = %q, want ready", ready.Type)
	}

	writeWSMessage(t, conn1, wsMessage{
		Type:   "task_assign",
		TaskID: "task-789",
		Kind:   "issue",
		Repo:   "projectbluefin/dakota",
		Number: 3,
		Title:  "Persist completion across reconnect",
		Prompt: "Reconnect and finish cleanly.",
	})
	if accepted := readWSMessage(t, conn1); accepted.Type != "task_accepted" {
		t.Fatalf("task_accepted = %+v", accepted)
	}
	<-handler.started
	client.mu.Lock()
	conn1Done := client.connDone
	client.mu.Unlock()
	_ = conn1.Close()
	<-conn1Done

	handler.finish <- handlerOutcome{
		report: TaskReport{
			Result:  "completed",
			Summary: "finished while disconnected",
		},
	}

	conn2 := <-conns
	authenticateConnection(t, conn2)

	completed := readWSMessage(t, conn2)
	if completed.Type != "task_complete" || completed.TaskID != "task-789" {
		t.Fatalf("queued completion = %+v", completed)
	}
	if completed.Summary != "finished while disconnected" {
		t.Fatalf("queued completion summary = %q", completed.Summary)
	}
	if ready := readWSMessage(t, conn2); ready.Type != "ready" {
		t.Fatalf("message after queued completion = %+v", ready)
	}

	cancel()
	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func TestClientRefreshesAndRevokesActiveTask(t *testing.T) {
	wsURL, conns, cleanup := newWSTestServer(t)
	defer cleanup()

	handler := newBlockingHandler()
	client := NewClient()
	client.ReconnectDelay = 10 * time.Millisecond
	client.MaxReconnectDelay = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Run(ctx, Credentials{
			RegistrationToken: "secret-token",
			WSURL:             wsURL,
			CLIBackend:        "goose",
		}, handler)
	}()

	conn := <-conns
	authenticateConnection(t, conn)
	if ready := readWSMessage(t, conn); ready.Type != "ready" {
		t.Fatalf("message after auth_ok = %q, want ready", ready.Type)
	}

	writeWSMessage(t, conn, wsMessage{
		Type:        "task_assign",
		TaskID:      "task-refresh",
		Kind:        "issue",
		Repo:        "projectbluefin/common",
		Number:      11,
		Title:       "Exercise refresh and revoke",
		GitHubToken: "ghs_initial_token",
	})
	if accepted := readWSMessage(t, conn); accepted.Type != "task_accepted" {
		t.Fatalf("task_accepted = %+v", accepted)
	}
	<-handler.started

	expiresAt := time.Now().Add(50 * time.Minute).UTC().Format(time.RFC3339)
	writeWSMessage(t, conn, wsMessage{
		Type:           "token_refresh",
		TaskID:         "task-refresh",
		GitHubToken:    "ghs_refreshed_token",
		TokenExpiresAt: expiresAt,
	})

	refreshed := <-handler.refreshes
	if refreshed.GitHubToken != "ghs_refreshed_token" {
		t.Fatalf("refreshed token = %q", refreshed.GitHubToken)
	}
	if refreshed.TokenExpiresAt.IsZero() {
		t.Fatal("refreshed token expiry should be parsed")
	}

	writeWSMessage(t, conn, wsMessage{
		Type:   "task_revoke",
		TaskID: "task-refresh",
		Reason: "timeout",
	})

	ready := readWSMessage(t, conn)
	if ready.Type != "ready" {
		t.Fatalf("message after task_revoke = %+v", ready)
	}
	assertNoMessage(t, conn)

	cancel()
	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

func TestClientRejectsAssignmentMissingIdentity(t *testing.T) {
	wsURL, conns, cleanup := newWSTestServer(t)
	defer cleanup()

	handler := newBlockingHandler()
	client := NewClient()
	client.ReconnectDelay = 10 * time.Millisecond
	client.MaxReconnectDelay = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Run(ctx, Credentials{
			RegistrationToken: "secret-token",
			WSURL:             wsURL,
			CLIBackend:        "goose",
		}, handler)
	}()

	conn := <-conns
	authenticateConnection(t, conn)
	if ready := readWSMessage(t, conn); ready.Type != "ready" {
		t.Fatalf("message after auth_ok = %q, want ready", ready.Type)
	}

	writeWSMessage(t, conn, wsMessage{
		Type:   "task_assign",
		TaskID: "task-invalid",
		Kind:   "issue",
	})

	failed := readWSMessage(t, conn)
	if failed.Type != "task_failed" || failed.TaskID != "task-invalid" {
		t.Fatalf("task_failed = %+v", failed)
	}
	if !strings.Contains(failed.Reason, "invalid hive assignment") {
		t.Fatalf("task_failed reason = %q", failed.Reason)
	}
	if ready := readWSMessage(t, conn); ready.Type != "ready" {
		t.Fatalf("message after task_failed = %+v", ready)
	}

	select {
	case started := <-handler.started:
		t.Fatalf("handler should not have started: %+v", started)
	case <-time.After(100 * time.Millisecond):
	}

	cancel()
	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
}

type blockingHandler struct {
	started   chan Assignment
	refreshes chan Assignment
	finish    chan handlerOutcome
}

type handlerOutcome struct {
	report TaskReport
	err    error
}

func newBlockingHandler() *blockingHandler {
	return &blockingHandler{
		started:   make(chan Assignment, 8),
		refreshes: make(chan Assignment, 8),
		finish:    make(chan handlerOutcome, 8),
	}
}

func (h *blockingHandler) Handle(ctx context.Context, assignment Assignment) (TaskReport, error) {
	h.started <- assignment
	select {
	case outcome := <-h.finish:
		return outcome.report, outcome.err
	case <-ctx.Done():
		return TaskReport{}, context.Cause(ctx)
	}
}

func (h *blockingHandler) Refresh(_ context.Context, assignment Assignment) error {
	h.refreshes <- assignment
	return nil
}

func newWSTestServer(t *testing.T) (string, <-chan *websocket.Conn, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	conns := make(chan *websocket.Conn, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("Upgrade() error = %v", err)
			return
		}
		conns <- conn
	}))

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/contribute/ws"
	return wsURL, conns, func() {
		server.Close()
		for {
			select {
			case conn := <-conns:
				_ = conn.Close()
			default:
				return
			}
		}
	}
}

func authenticateConnection(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	writeWSMessage(t, conn, wsMessage{Type: "auth_challenge", Seq: 1, Nonce: "nonce"})
	auth := readWSMessage(t, conn)
	if auth.Type != "auth_response" {
		t.Fatalf("auth message type = %q, want auth_response", auth.Type)
	}
	writeWSMessage(t, conn, wsMessage{Type: "auth_ok", ContributorID: "c-123", TrustTier: "newcomer"})
}

func readWSMessage(t *testing.T, conn *websocket.Conn) wsMessage {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	var msg wsMessage
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	return msg
}

func writeWSMessage(t *testing.T, conn *websocket.Conn, msg wsMessage) {
	t.Helper()
	if err := conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetWriteDeadline() error = %v", err)
	}
	if err := conn.WriteJSON(msg); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
}

func assertNoMessage(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	var msg wsMessage
	if err := conn.ReadJSON(&msg); err == nil {
		t.Fatalf("unexpected message = %+v", msg)
	} else if !websocket.IsCloseError(err) && !isTimeout(err) {
		t.Fatalf("ReadJSON() error = %v, want timeout or close", err)
	}
}

func isTimeout(err error) bool {
	var netErr interface{ Timeout() bool }
	return errors.As(err, &netErr) && netErr.Timeout()
}
