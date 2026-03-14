package app

import (
	"context"
	"fmt"

	"github.com/nikkofu/aether/internal/domain/strategy/strategic"
	taskdomain "github.com/nikkofu/aether/internal/domain/task"
	taskusecase "github.com/nikkofu/aether/internal/usecase/task"
)

type strategicTaskLauncher struct {
	tasks *taskusecase.Service
}

func (l *strategicTaskLauncher) LaunchTask(ctx context.Context, req strategic.TaskLaunchRequest) error {
	if l == nil || l.tasks == nil {
		return fmt.Errorf("task service unavailable")
	}

	pattern := taskdomain.NormalizeWorkflowPattern(req.WorkflowPattern)
	if pattern == "" {
		pattern = taskdomain.PatternSequential
	}

	_, err := l.tasks.Submit(ctx, taskusecase.SubmitInput{
		Source:          "strategic_engine",
		Mode:            "agent",
		WorkflowPattern: pattern,
		Description:     req.Description,
		Input:           req.Input,
		TraceID:         req.TraceID,
		OrgID:           req.OrgID,
	})
	return err
}

var _ strategic.TaskLauncher = (*strategicTaskLauncher)(nil)
