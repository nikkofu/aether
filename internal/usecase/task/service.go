package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	agentdomain "github.com/nikkofu/aether/internal/domain/agent"
	taskdomain "github.com/nikkofu/aether/internal/domain/task"
	"github.com/nikkofu/aether/pkg/bus"
	"github.com/nikkofu/aether/pkg/logging"
)

var (
	ErrTaskNotFound      = errors.New("task not found")
	ErrTaskNotCancelable = errors.New("task is not cancelable")
	ErrTaskNotRetryable  = errors.New("task is not retryable")
)

type SubmitInput struct {
	ParentTaskID string         `json:"parent_task_id"`
	Attempt      int            `json:"attempt"`
	Source       string         `json:"source"`
	Mode         string         `json:"mode"`
	Description  string         `json:"description"`
	Input        map[string]any `json:"input"`
	TraceID      string         `json:"trace_id"`
	OrgID        string         `json:"org_id"`
}

type Update struct {
	Task  *taskdomain.Task  `json:"task,omitempty"`
	Event *taskdomain.Event `json:"event,omitempty"`
}

type executionState struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type Service struct {
	store          taskdomain.Store
	bus            bus.Bus
	logger         logging.Logger
	subscribeMu    sync.Once
	executionMu    sync.Mutex
	executions     map[string]*executionState
	cancelledMu    sync.RWMutex
	cancelled      map[string]struct{}
	listenersMu    sync.RWMutex
	listeners      map[string]map[uint64]chan Update
	nextListenerID uint64
}

func NewService(store taskdomain.Store, b bus.Bus, logger logging.Logger) *Service {
	return &Service{
		store:      store,
		bus:        b,
		logger:     logger,
		executions: make(map[string]*executionState),
		cancelled:  make(map[string]struct{}),
		listeners:  make(map[string]map[uint64]chan Update),
	}
}

func (s *Service) StartObservers(ctx context.Context) {
	s.subscribeMu.Do(func() {
		if s.bus == nil {
			return
		}
		s.bus.SubscribeToSubject(ctx, "*", func(msg agentdomain.Message) {
			s.handleMessage(msg)
		})
	})
}

func (s *Service) Submit(ctx context.Context, input SubmitInput) (*taskdomain.Task, error) {
	if input.Description == "" {
		return nil, fmt.Errorf("task description is required")
	}

	if input.Mode == "" {
		input.Mode = "agent"
	}
	if input.Mode != "agent" {
		return nil, fmt.Errorf("unsupported task mode: %s", input.Mode)
	}
	if input.Source == "" {
		input.Source = "api"
	}
	if input.OrgID == "" {
		input.OrgID = "default"
	}
	if input.Attempt <= 0 {
		input.Attempt = 1
	}

	t := &taskdomain.Task{
		ID:           uuid.New().String(),
		ParentTaskID: input.ParentTaskID,
		Attempt:      input.Attempt,
		Source:       input.Source,
		Mode:         input.Mode,
		Description:  input.Description,
		Input:        input.Input,
		Status:       taskdomain.StatusQueued,
		TraceID:      input.TraceID,
		CurrentStage: "queued",
	}

	if err := s.store.Create(ctx, t); err != nil {
		return nil, err
	}

	s.ensureExecution(t.ID)

	submittedEvent := &taskdomain.Event{
		TaskID: t.ID,
		Type:   "task.submitted",
		Payload: map[string]any{
			"description":    input.Description,
			"source":         input.Source,
			"mode":           input.Mode,
			"attempt":        input.Attempt,
			"parent_task_id": input.ParentTaskID,
		},
	}
	if err := s.store.AppendEvent(ctx, submittedEvent); err != nil {
		return nil, err
	}

	t.Status = taskdomain.StatusRunning
	t.CurrentStage = "supervisor"
	if err := s.store.Update(ctx, t); err != nil {
		return nil, err
	}

	s.publishUpdate(t, submittedEvent)

	if s.bus != nil {
		s.bus.Publish(ctx, agentdomain.Message{
			ID:        t.ID,
			From:      input.Source,
			To:        "supervisor",
			Type:      "task",
			Timestamp: time.Now(),
			Payload: map[string]any{
				"description": input.Description,
				"org_id":      input.OrgID,
				"trace_id":    input.TraceID,
				"task_id":     t.ID,
			},
		})
	}

	return t, nil
}

func (s *Service) Get(ctx context.Context, id string) (*taskdomain.Task, error) {
	task, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, s.normalizeError(err)
	}
	return task, nil
}

func (s *Service) List(ctx context.Context, limit int) ([]*taskdomain.Task, error) {
	return s.store.List(ctx, limit)
}

func (s *Service) ListEvents(ctx context.Context, taskID string, limit int) ([]*taskdomain.Event, error) {
	return s.store.ListEvents(ctx, taskID, limit)
}

func (s *Service) Cancel(ctx context.Context, id, reason string) (*taskdomain.Task, error) {
	t, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	switch t.Status {
	case taskdomain.StatusCompleted, taskdomain.StatusFailed:
		return nil, fmt.Errorf("%w: %s", ErrTaskNotCancelable, t.Status)
	case taskdomain.StatusCancelled:
		return t, nil
	}

	if reason == "" {
		reason = "Cancelled via API."
	}

	t.Status = taskdomain.StatusCancelled
	t.CurrentStage = "cancelled"
	t.ErrorSummary = reason
	if err := s.store.Update(ctx, t); err != nil {
		return nil, err
	}
	s.markCancelled(t.ID)
	s.cancelExecution(t.ID)

	cancelEvent := &taskdomain.Event{
		TaskID: t.ID,
		Type:   "task.cancel_requested",
		Payload: map[string]any{
			"reason": reason,
			"soft":   false,
		},
	}
	if err := s.store.AppendEvent(ctx, cancelEvent); err != nil && s.logger != nil {
		s.logger.Error(ctx, "failed to append cancel event",
			logging.String("task_id", id),
			logging.Err(err),
		)
	}

	s.publishUpdate(t, cancelEvent)
	return t, nil
}

func (s *Service) Retry(ctx context.Context, id string) (*taskdomain.Task, error) {
	original, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	switch original.Status {
	case taskdomain.StatusQueued, taskdomain.StatusRunning, taskdomain.StatusReviewing:
		return nil, fmt.Errorf("%w: %s", ErrTaskNotRetryable, original.Status)
	}

	input := cloneMap(original.Input)
	if input == nil {
		input = make(map[string]any)
	}
	input["retry_of"] = original.ID
	input["retry_status"] = original.Status

	retriedTask, err := s.Submit(ctx, SubmitInput{
		ParentTaskID: original.ID,
		Attempt:      original.Attempt + 1,
		Source:       "task_retry",
		Mode:         original.Mode,
		Description:  original.Description,
		Input:        input,
		TraceID:      original.TraceID,
	})
	if err != nil {
		return nil, err
	}

	retryEvent := &taskdomain.Event{
		TaskID: original.ID,
		Type:   "task.retry_requested",
		Payload: map[string]any{
			"retry_task_id": retriedTask.ID,
			"retry_status":  original.Status,
		},
	}
	if err := s.store.AppendEvent(ctx, retryEvent); err != nil && s.logger != nil {
		s.logger.Error(ctx, "failed to append retry event",
			logging.String("task_id", id),
			logging.Err(err),
		)
	}

	s.publishUpdate(original, retryEvent)
	return retriedTask, nil
}

func (s *Service) RecoverInterrupted(ctx context.Context) (int, error) {
	tasks, err := s.store.ListByStatus(ctx, []taskdomain.Status{
		taskdomain.StatusQueued,
		taskdomain.StatusRunning,
		taskdomain.StatusReviewing,
	}, 500)
	if err != nil {
		return 0, err
	}

	recovered := 0
	for _, task := range tasks {
		if task == nil {
			continue
		}

		previousStatus := task.Status
		task.Status = taskdomain.StatusInterrupted
		task.CurrentStage = "interrupted"
		if task.ErrorSummary == "" {
			task.ErrorSummary = "Task execution was interrupted before daemon recovery completed."
		}

		if err := s.store.Update(ctx, task); err != nil {
			if s.logger != nil {
				s.logger.Error(ctx, "failed to mark task interrupted",
					logging.String("task_id", task.ID),
					logging.Err(err),
				)
			}
			continue
		}

		event := &taskdomain.Event{
			TaskID: task.ID,
			Type:   "task.interrupted",
			Payload: map[string]any{
				"previous_status": previousStatus,
				"recovered_at":    time.Now().UTC().Format(time.RFC3339),
			},
		}
		if err := s.store.AppendEvent(ctx, event); err != nil && s.logger != nil {
			s.logger.Error(ctx, "failed to append interrupted task event",
				logging.String("task_id", task.ID),
				logging.Err(err),
			)
		}

		s.finishExecution(task.ID, taskdomain.StatusInterrupted)
		s.publishUpdate(task, event)
		recovered++
	}

	return recovered, nil
}

func (s *Service) Subscribe(taskID string, buffer int) (<-chan Update, func()) {
	if buffer <= 0 {
		buffer = 32
	}

	ch := make(chan Update, buffer)
	listenerID := atomic.AddUint64(&s.nextListenerID, 1)

	s.listenersMu.Lock()
	if s.listeners[taskID] == nil {
		s.listeners[taskID] = make(map[uint64]chan Update)
	}
	s.listeners[taskID][listenerID] = ch
	s.listenersMu.Unlock()

	cancel := func() {
		s.listenersMu.Lock()
		defer s.listenersMu.Unlock()

		taskListeners, ok := s.listeners[taskID]
		if !ok {
			return
		}

		listener, ok := taskListeners[listenerID]
		if !ok {
			return
		}

		delete(taskListeners, listenerID)
		close(listener)
		if len(taskListeners) == 0 {
			delete(s.listeners, taskID)
		}
	}

	return ch, cancel
}

func (s *Service) ContextForTask(parent context.Context, taskID string) (context.Context, context.CancelFunc) {
	if taskID == "" {
		return context.WithCancel(parent)
	}

	execution := s.ensureExecution(taskID)
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(execution.ctx, cancel)

	return ctx, func() {
		stop()
		cancel()
	}
}

func (s *Service) IsTaskCancelled(taskID string) bool {
	if taskID == "" {
		return false
	}
	if s.isCancelled(taskID) {
		return true
	}

	task, err := s.store.Get(context.Background(), taskID)
	if err != nil {
		return false
	}
	return task.Status == taskdomain.StatusCancelled
}

func (s *Service) handleMessage(msg agentdomain.Message) {
	if msg.Type == "token" {
		return
	}

	taskID := s.taskIDFromMessage(msg)
	if taskID == "" {
		return
	}

	ctx := context.Background()
	t, err := s.store.Get(ctx, taskID)
	if err != nil {
		return
	}

	event := &taskdomain.Event{
		TaskID:    taskID,
		MessageID: msg.ID,
		From:      msg.From,
		To:        msg.To,
		Type:      msg.Type,
		Payload:   msg.Payload,
	}
	if err := s.store.AppendEvent(ctx, event); err != nil && s.logger != nil {
		s.logger.Error(ctx, "failed to append task event",
			logging.String("task_id", taskID),
			logging.String("event_type", msg.Type),
			logging.Err(err),
		)
	}

	if s.isCancelled(taskID) || t.Status == taskdomain.StatusCancelled {
		currentTask, currentErr := s.store.Get(ctx, taskID)
		if currentErr == nil {
			s.publishUpdate(currentTask, event)
		} else {
			s.publishUpdate(t, event)
		}
		return
	}

	switch msg.Type {
	case "task":
		t.Status = taskdomain.StatusRunning
		t.CurrentStage = "supervisor"
	case "task_plan_request":
		t.Status = taskdomain.StatusRunning
		t.CurrentStage = "planner"
	case "instruction":
		t.Status = taskdomain.StatusRunning
		t.CurrentStage = "coder"
	case "review_request":
		t.Status = taskdomain.StatusReviewing
		t.CurrentStage = "reviewer"
	case "review_result":
		approved, _ := msg.Payload["approved"].(bool)
		if approved {
			t.Status = taskdomain.StatusReviewing
			t.CurrentStage = "approved"
		} else {
			t.Status = taskdomain.StatusRunning
			t.CurrentStage = "coder"
		}
	case "work_progress":
		t.Status = taskdomain.StatusRunning
		if msg.From != "" {
			t.CurrentStage = msg.From
		}
	case "final_report":
		t.Status = taskdomain.StatusCompleted
		t.CurrentStage = "completed"
		if result, ok := msg.Payload["result"].(string); ok {
			t.FinalOutput = result
		}
	case agentdomain.TypeSystemAlert:
		t.Status = taskdomain.StatusFailed
		t.CurrentStage = "failed"
		if message, ok := msg.Payload["message"].(string); ok {
			t.ErrorSummary = message
		}
	}

	if traceID, ok := msg.Payload["trace_id"].(string); ok && traceID != "" {
		t.TraceID = traceID
	}

	if err := s.store.Update(ctx, t); err != nil && s.logger != nil {
		s.logger.Error(ctx, "failed to update task state", logging.String("task_id", taskID), logging.Err(err))
		return
	}

	if isTerminalStatus(t.Status) {
		s.finishExecution(t.ID, t.Status)
	}

	s.publishUpdate(t, event)
}

func (s *Service) taskIDFromMessage(msg agentdomain.Message) string {
	if msg.ID != "" {
		if _, err := s.Get(context.Background(), msg.ID); err == nil {
			return msg.ID
		}
	}
	if msg.Payload == nil {
		return ""
	}
	taskID, _ := msg.Payload["task_id"].(string)
	return taskID
}

func (s *Service) publishUpdate(task *taskdomain.Task, event *taskdomain.Event) {
	if task == nil {
		return
	}

	s.listenersMu.RLock()
	defer s.listenersMu.RUnlock()

	listeners := s.listeners[task.ID]
	if len(listeners) == 0 {
		return
	}

	update := Update{
		Task:  cloneTask(task),
		Event: cloneEvent(event),
	}

	for _, listener := range listeners {
		select {
		case listener <- update:
		default:
		}
	}
}

func (s *Service) normalizeError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "task not found") {
		return fmt.Errorf("%w: %v", ErrTaskNotFound, err)
	}
	return err
}

func cloneTask(task *taskdomain.Task) *taskdomain.Task {
	if task == nil {
		return nil
	}

	cloned := *task
	cloned.Input = cloneMap(task.Input)
	return &cloned
}

func cloneEvent(event *taskdomain.Event) *taskdomain.Event {
	if event == nil {
		return nil
	}

	cloned := *event
	cloned.Payload = cloneMap(event.Payload)
	return &cloned
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}

	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func (s *Service) markCancelled(taskID string) {
	s.cancelledMu.Lock()
	defer s.cancelledMu.Unlock()
	s.cancelled[taskID] = struct{}{}
}

func (s *Service) isCancelled(taskID string) bool {
	s.cancelledMu.RLock()
	defer s.cancelledMu.RUnlock()
	_, ok := s.cancelled[taskID]
	return ok
}

func (s *Service) ensureExecution(taskID string) *executionState {
	s.executionMu.Lock()
	defer s.executionMu.Unlock()

	if execution, ok := s.executions[taskID]; ok {
		return execution
	}

	ctx, cancel := context.WithCancel(context.Background())
	execution := &executionState{
		ctx:    ctx,
		cancel: cancel,
	}
	s.executions[taskID] = execution
	return execution
}

func (s *Service) cancelExecution(taskID string) {
	s.executionMu.Lock()
	execution := s.executions[taskID]
	s.executionMu.Unlock()
	if execution != nil {
		execution.cancel()
	}
}

func (s *Service) finishExecution(taskID string, status taskdomain.Status) {
	if status == taskdomain.StatusCancelled {
		s.markCancelled(taskID)
	}

	s.executionMu.Lock()
	execution := s.executions[taskID]
	delete(s.executions, taskID)
	s.executionMu.Unlock()
	if execution != nil {
		execution.cancel()
	}
}

func isTerminalStatus(status taskdomain.Status) bool {
	switch status {
	case taskdomain.StatusCompleted, taskdomain.StatusFailed, taskdomain.StatusCancelled, taskdomain.StatusInterrupted:
		return true
	default:
		return false
	}
}
