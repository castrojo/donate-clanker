package hive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultReconnectDelay = time.Second
	maxReconnectDelay     = time.Minute
	heartbeatInterval     = 30 * time.Second
	heartbeatTimeout      = 90 * time.Second
	writeTimeout          = 10 * time.Second
	maxMessageSize        = 64 * 1024
)

var (
	ErrAuthenticationFailed = errors.New("hive authentication failed")
	ErrHeartbeatTimeout     = errors.New("hive heartbeat timeout")
	ErrTaskRevoked          = errors.New("hive task revoked")

	errConnectionUnavailable = errors.New("hive connection unavailable")
)

type TaskReport struct {
	Result  string
	Summary string
	Output  []string
}

type AssignmentHandler interface {
	Handle(context.Context, Assignment) (TaskReport, error)
}

type AssignmentRefresher interface {
	Refresh(context.Context, Assignment) error
}

type websocketDialer interface {
	DialContext(context.Context, string, http.Header) (*websocket.Conn, *http.Response, error)
}

type Client struct {
	Dialer            websocketDialer
	ReconnectDelay    time.Duration
	MaxReconnectDelay time.Duration
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	WriteTimeout      time.Duration
	Now               func() time.Time

	writeMu  sync.Mutex
	mu       sync.Mutex
	conn     *websocket.Conn
	authed   bool
	lastPong time.Time
	seq      int
	active   *taskExecution
	pending  *wsMessage
}

type taskExecution struct {
	assignment     Assignment
	cancel         context.CancelCauseFunc
	suppressReport bool
}

type authError struct {
	reason         string
	acceptedModels []string
}

func (e *authError) Error() string {
	reason := strings.TrimSpace(e.reason)
	if reason == "" {
		reason = "authentication failed"
	}
	if len(e.acceptedModels) == 0 {
		return fmt.Sprintf("%s: %s", ErrAuthenticationFailed, reason)
	}
	return fmt.Sprintf("%s: %s (accepted models: %s)", ErrAuthenticationFailed, reason, strings.Join(e.acceptedModels, ", "))
}

func (e *authError) Unwrap() error {
	return ErrAuthenticationFailed
}

func NewClient() *Client {
	return &Client{
		Dialer:            &websocket.Dialer{},
		ReconnectDelay:    defaultReconnectDelay,
		MaxReconnectDelay: maxReconnectDelay,
		HeartbeatInterval: heartbeatInterval,
		HeartbeatTimeout:  heartbeatTimeout,
		WriteTimeout:      writeTimeout,
		Now:               time.Now,
	}
}

func (c *Client) Run(ctx context.Context, creds Credentials, handler AssignmentHandler) error {
	if handler == nil {
		return errors.New("missing assignment handler")
	}
	if strings.TrimSpace(creds.RegistrationToken) == "" {
		return fmt.Errorf("%w: missing registration token", ErrInvalidCredentials)
	}
	if strings.TrimSpace(creds.WSURL) == "" {
		return fmt.Errorf("%w: missing Hive WebSocket URL", ErrInvalidCredentials)
	}
	if c.Dialer == nil {
		c.Dialer = &websocket.Dialer{}
	}
	if c.ReconnectDelay <= 0 {
		c.ReconnectDelay = defaultReconnectDelay
	}
	if c.MaxReconnectDelay <= 0 {
		c.MaxReconnectDelay = maxReconnectDelay
	}
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = heartbeatInterval
	}
	if c.HeartbeatTimeout <= 0 {
		c.HeartbeatTimeout = heartbeatTimeout
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = writeTimeout
	}
	if c.Now == nil {
		c.Now = time.Now
	}

	defer c.shutdownActive(context.Cause(ctx))

	delay := c.ReconnectDelay
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		conn, _, err := c.Dialer.DialContext(ctx, creds.WSURL, nil)
		if err != nil {
			if err := sleepContext(ctx, delay); err != nil {
				return err
			}
			delay = minDuration(delay*2, c.MaxReconnectDelay)
			continue
		}
		delay = c.ReconnectDelay

		err = c.handleConnection(ctx, conn, creds, handler)
		if err == nil {
			continue
		}
		if errors.Is(err, ErrAuthenticationFailed) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := sleepContext(ctx, delay); err != nil {
			return err
		}
		delay = minDuration(delay*2, c.MaxReconnectDelay)
	}
}

func (c *Client) handleConnection(ctx context.Context, conn *websocket.Conn, creds Credentials, handler AssignmentHandler) error {
	conn.SetReadLimit(maxMessageSize)
	c.setConnection(conn)
	defer func() {
		_ = conn.Close()
		c.clearConnection(conn)
	}()

	incoming := make(chan wsMessage, 8)
	readErr := make(chan error, 1)
	go readLoop(ctx, conn, incoming, readErr)

	heartbeat := time.NewTicker(c.HeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case msg, ok := <-incoming:
			if !ok {
				return errConnectionUnavailable
			}
			c.touch()

			switch msg.Type {
			case "auth_challenge":
				if err := c.writeMessage(wsMessage{
					Type:              "auth_response",
					Seq:               c.nextSeq(),
					RegistrationToken: creds.RegistrationToken,
					CLIBackend:        creds.CLIBackend,
					Model:             creds.Model,
				}); err != nil {
					return err
				}

			case "auth_ok":
				c.setAuthenticated(conn, true)
				if err := c.resumeAfterAuth(); err != nil {
					return err
				}

			case "auth_failed":
				c.setAuthenticated(conn, false)
				c.revokeActive(&authError{reason: msg.Reason, acceptedModels: msg.AcceptedModels})
				return &authError{reason: msg.Reason, acceptedModels: msg.AcceptedModels}

			case "task_assign":
				if err := c.handleTaskAssign(ctx, handler, msg); err != nil {
					return err
				}

			case "token_refresh":
				c.handleTokenRefresh(ctx, handler, msg)

			case "task_revoke":
				c.handleTaskRevoke(msg)

			case "ping":
				if err := c.writeMessage(wsMessage{Type: "pong", Seq: msg.Seq}); err != nil {
					return err
				}

			case "pong":
				// liveness already updated by touch()
			}

		case err := <-readErr:
			if errors.Is(err, context.Canceled) {
				return ctx.Err()
			}
			return err

		case <-heartbeat.C:
			if !c.isAuthenticated() {
				continue
			}
			if c.Now().Sub(c.lastPongAt()) > c.HeartbeatTimeout {
				return ErrHeartbeatTimeout
			}
			if err := c.writeMessage(wsMessage{Type: "ping", Seq: c.nextSeq()}); err != nil {
				return err
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *Client) handleTaskAssign(ctx context.Context, handler AssignmentHandler, msg wsMessage) error {
	assignment, err := assignmentFromMessage(msg)
	if err != nil {
		if strings.TrimSpace(msg.TaskID) != "" {
			_ = c.writeOrQueue(wsMessage{
				Type:   "task_failed",
				Seq:    c.nextSeq(),
				TaskID: msg.TaskID,
				Reason: err.Error(),
			})
			_ = c.writeReady()
			return nil
		}
		return err
	}

	c.mu.Lock()
	if c.active != nil {
		c.mu.Unlock()
		_ = c.writeOrQueue(wsMessage{
			Type:   "task_failed",
			Seq:    c.nextSeq(),
			TaskID: assignment.TaskID,
			Reason: "already has active task",
		})
		return nil
	}

	taskCtx, cancel := context.WithCancelCause(ctx)
	exec := &taskExecution{
		assignment: assignment,
		cancel:     cancel,
	}
	c.active = exec
	c.mu.Unlock()

	if err := c.writeMessage(wsMessage{
		Type:   "task_accepted",
		Seq:    c.nextSeq(),
		TaskID: assignment.TaskID,
	}); err != nil {
		c.clearActive(exec)
		return err
	}

	go c.runTask(taskCtx, handler, exec)
	return nil
}

func (c *Client) handleTokenRefresh(ctx context.Context, handler AssignmentHandler, msg wsMessage) {
	c.mu.Lock()
	active := c.active
	if active == nil || active.assignment.TaskID != msg.TaskID {
		c.mu.Unlock()
		return
	}

	active.assignment.GitHubToken = msg.GitHubToken
	if expiresAt := strings.TrimSpace(msg.TokenExpiresAt); expiresAt != "" {
		if parsed, err := time.Parse(time.RFC3339, expiresAt); err == nil {
			active.assignment.TokenExpiresAt = parsed
		}
	}
	updated := active.assignment
	c.mu.Unlock()

	refresher, ok := handler.(AssignmentRefresher)
	if !ok {
		return
	}
	_ = refresher.Refresh(ctx, updated)
}

func (c *Client) handleTaskRevoke(msg wsMessage) {
	c.mu.Lock()
	active := c.active
	if active == nil || active.assignment.TaskID != msg.TaskID {
		c.mu.Unlock()
		return
	}
	active.suppressReport = true
	cancel := active.cancel
	c.mu.Unlock()

	cancel(fmt.Errorf("%w: %s", ErrTaskRevoked, strings.TrimSpace(msg.Reason)))
}

func (c *Client) runTask(ctx context.Context, handler AssignmentHandler, exec *taskExecution) {
	report, err := handler.Handle(ctx, exec.assignment)
	c.finishTask(exec, report, err)
}

func (c *Client) finishTask(exec *taskExecution, report TaskReport, err error) {
	c.mu.Lock()
	if c.active != exec {
		c.mu.Unlock()
		return
	}
	suppress := exec.suppressReport
	c.active = nil
	c.mu.Unlock()

	if suppress {
		_ = c.writeReady()
		return
	}

	msg := wsMessage{
		Seq:        c.nextSeq(),
		TaskID:     exec.assignment.TaskID,
		TmuxOutput: append([]string(nil), report.Output...),
	}

	if err != nil {
		msg.Type = "task_failed"
		msg.Reason = failureReason(err)
	} else {
		msg.Type = "task_complete"
		msg.Result = firstNonEmpty(report.Result, "completed")
		msg.Summary = report.Summary
	}

	_ = c.writeOrQueue(msg)
	_ = c.writeReady()
}

func (c *Client) resumeAfterAuth() error {
	c.mu.Lock()
	active := c.active
	pending := c.pending
	c.mu.Unlock()

	if active != nil {
		if err := c.writeMessage(wsMessage{
			Type:   "task_accepted",
			Seq:    c.nextSeq(),
			TaskID: active.assignment.TaskID,
		}); err != nil {
			return err
		}
		if err := c.writeMessage(wsMessage{
			Type:   "task_progress",
			Seq:    c.nextSeq(),
			TaskID: active.assignment.TaskID,
			Kind:   active.assignment.Kind,
			Repo:   active.assignment.Repo,
			Number: active.assignment.Number,
			Title:  active.assignment.Title,
			Status: "working",
		}); err != nil {
			return err
		}
		return nil
	}

	if pending != nil {
		if err := c.writeMessage(*pending); err != nil {
			return err
		}
		c.mu.Lock()
		if c.pending == pending {
			c.pending = nil
		}
		c.mu.Unlock()
	}

	return c.writeReady()
}

func (c *Client) writeReady() error {
	return c.writeMessage(wsMessage{
		Type: "ready",
		Seq:  c.nextSeq(),
	})
}

func (c *Client) writeOrQueue(msg wsMessage) error {
	if err := c.writeMessage(msg); err != nil {
		c.mu.Lock()
		c.pending = &msg
		c.mu.Unlock()
		return err
	}

	c.mu.Lock()
	if c.pending != nil && c.pending.TaskID == msg.TaskID && c.pending.Type == msg.Type {
		c.pending = nil
	}
	c.mu.Unlock()
	return nil
}

func (c *Client) writeMessage(msg wsMessage) error {
	c.mu.Lock()
	conn := c.conn
	authed := c.authed
	c.mu.Unlock()

	if conn == nil {
		return errConnectionUnavailable
	}
	if msg.Type != "auth_response" && msg.Type != "pong" && !authed {
		return errConnectionUnavailable
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	_ = conn.SetWriteDeadline(c.Now().Add(c.WriteTimeout))
	if err := conn.WriteJSON(msg); err != nil {
		return err
	}
	return nil
}

func (c *Client) setConnection(conn *websocket.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn = conn
	c.authed = false
	c.lastPong = c.Now()
}

func (c *Client) setAuthenticated(conn *websocket.Conn, authed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != conn {
		return
	}
	c.authed = authed
	c.lastPong = c.Now()
}

func (c *Client) clearConnection(conn *websocket.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == conn {
		c.conn = nil
		c.authed = false
	}
}

func (c *Client) clearActive(exec *taskExecution) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active == exec {
		c.active = nil
	}
}

func (c *Client) revokeActive(err error) {
	c.mu.Lock()
	active := c.active
	if active != nil {
		active.suppressReport = true
	}
	c.mu.Unlock()

	if active != nil {
		active.cancel(err)
	}
}

func (c *Client) shutdownActive(cause error) {
	c.revokeActive(cause)

	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
}

func (c *Client) touch() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastPong = c.Now()
}

func (c *Client) lastPongAt() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastPong
}

func (c *Client) isAuthenticated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.authed
}

func (c *Client) nextSeq() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	return c.seq
}

func readLoop(ctx context.Context, conn *websocket.Conn, incoming chan<- wsMessage, readErr chan<- error) {
	defer close(incoming)

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			select {
			case readErr <- err:
			default:
			}
			return
		}

		var msg wsMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		select {
		case incoming <- msg:
		case <-ctx.Done():
			select {
			case readErr <- ctx.Err():
			default:
			}
			return
		}
	}
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}

func failureReason(err error) string {
	if err == nil {
		return "task failed"
	}
	reason := strings.TrimSpace(strings.Join(strings.Fields(err.Error()), " "))
	if reason == "" {
		return "task failed"
	}
	return reason
}
