package dag

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nikkofu/aether/internal/domain/capability"
	"github.com/nikkofu/aether/internal/core/memory"
	"github.com/nikkofu/aether/pkg/observability"
	"github.com/nikkofu/aether/internal/domain/policy"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// PipelineExecutor 负责并发执行 Pipeline。
type PipelineExecutor struct {
	registry   *capability.CapabilityRegistry
	renderer   capability.PromptRenderer
	policy     policy.Policy
	memory     memory.Store
	tracer     observability.Tracer
	maxWorkers int
	eventChan  chan ExecutionEvent
	once       sync.Once
}

func NewPipelineExecutor(reg *capability.CapabilityRegistry, pol policy.Policy, mem memory.Store, tracer observability.Tracer, workers int) *PipelineExecutor {
	if workers <= 0 {
		workers = 5
	}
	if pol == nil {
		pol = policy.NewDefaultPolicy()
	}
	return &PipelineExecutor{
		registry:   reg,
		renderer:   capability.NewDefaultRenderer(),
		policy:     pol,
		memory:     mem,
		tracer:     tracer,
		maxWorkers: workers,
		eventChan:  make(chan ExecutionEvent, 100),
	}
}

func (e *PipelineExecutor) Events() <-chan ExecutionEvent {
	return e.eventChan
}

func (e *PipelineExecutor) sendEvent(event ExecutionEvent) {
	select {
	case e.eventChan <- event:
	default:
	}
}

func (e *PipelineExecutor) Execute(ctx context.Context, p *Pipeline) (map[string]map[string]any, error) {
	// Tracing: pipeline execution
	tracer := otel.Tracer("aether-tracer")
	var span oteltrace.Span
	ctx, span = tracer.Start(ctx, "Pipeline.Execute")
	span.SetAttributes(attribute.Int("node_count", len(p.Nodes)))
	defer span.End()

	if e.tracer != nil {
		var cSpan observability.Span
		_, cSpan = e.tracer.StartSpan(ctx, "Pipeline.Execute", map[string]any{"node_count": len(p.Nodes)})
		defer cSpan.End()
	}

	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("pipeline validation failed: %w", err)
	}

	defer e.once.Do(func() { close(e.eventChan) })

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(map[string]map[string]any)
	var mu sync.RWMutex

	pipelineExecutionID := uuid.New().String()

	inDegree := make(map[string]int)
	adj := make(map[string][]string)
	nodeMap := make(map[string]Node)

	for _, node := range p.Nodes {
		nodeMap[node.ID] = node
		inDegree[node.ID] = len(node.DependsOn)
		for _, dep := range node.DependsOn {
			adj[dep] = append(adj[dep], node.ID)
		}
	}

	ready := make(chan string, len(p.Nodes))
	for id, deg := range inDegree {
		if deg == 0 {
			ready <- id
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(p.Nodes))
	workerTokens := make(chan struct{}, e.maxWorkers)
	totalNodes := len(p.Nodes)
	completedCh := make(chan struct{}, totalNodes)

	e.sendEvent(ExecutionEvent{Type: EventPipelineStarted, Timestamp: time.Now()})

	go func() {
		for i := 0; i < totalNodes; i++ {
			select {
			case <-ctx.Done():
				return
			case id := <-ready:
				wg.Add(1)
				go func(nodeID string) {
					defer wg.Done()
					startTime := time.Now()

					// Tracing: node execution
					var nodeSpan oteltrace.Span
					nodeCtx, nodeSpan := tracer.Start(ctx, "Node.Execute:"+nodeID)
					nodeSpan.SetAttributes(attribute.String("skill", nodeMap[nodeID].Skill))
					defer nodeSpan.End()

					e.sendEvent(ExecutionEvent{Type: EventNodeStarted, NodeID: nodeID, Timestamp: startTime})

					select {
					case workerTokens <- struct{}{}:
						defer func() { <-workerTokens }()
					case <-nodeCtx.Done():
						return
					}

					node := nodeMap[nodeID]

					mu.RLock()
					data := make(map[string]any)
					for k, v := range results {
						data[k] = v
					}
					mu.RUnlock()

					input, err := e.resolveValue(nodeID, node.Input, data)
					if err != nil {
						e.handleNodeFailure(nodeID, startTime, err, errCh, cancel)
						return
					}
					inputMap := input.(map[string]any)

					evalCtx := policy.EvaluationContext{NodeID: nodeID, Skill: node.Skill, Input: inputMap}
					decision, err := e.policy.Evaluate(nodeCtx, evalCtx)
					if err != nil || decision != policy.DecisionAllow {
						e.handleNodeFailure(nodeID, startTime, fmt.Errorf("policy blocked: %v", decision), errCh, cancel)
						return
					}

					capInst, ok := e.registry.Get(node.Skill)
					if !ok {
						err := fmt.Errorf("skill not found: %s", node.Skill)
						nodeSpan.RecordError(err)
						nodeSpan.SetStatus(codes.Error, err.Error())
						e.handleNodeFailure(nodeID, startTime, err, errCh, cancel)
						return
					}

					output, err := capInst.Execute(nodeCtx, inputMap)
					if err != nil {
						nodeSpan.RecordError(err)
						nodeSpan.SetStatus(codes.Error, err.Error())
						e.handleNodeFailure(nodeID, startTime, err, errCh, cancel)
						return
					}
					nodeSpan.SetStatus(codes.Ok, "success")

					if e.memory != nil {
						record := memory.ExecutionRecord{
							PipelineID: pipelineExecutionID,
							NodeID:     nodeID,
							Output:     output,
							Timestamp:  time.Now(),
						}
						go e.memory.Save(context.Background(), record)
					}

					e.sendEvent(ExecutionEvent{Type: EventNodeCompleted, NodeID: nodeID, Timestamp: time.Now(), Duration: time.Since(startTime)})

					mu.Lock()
					results[nodeID] = output
					for _, next := range adj[nodeID] {
						inDegree[next]--
						if inDegree[next] == 0 {
							ready <- next
						}
					}
					mu.Unlock()
					completedCh <- struct{}{}
				}(id)
			}
		}
	}()

	var finalErr error
WaitLoop:
	for i := 0; i < totalNodes; i++ {
		select {
		case <-completedCh:
		case err := <-errCh:
			finalErr = err
			break WaitLoop
		case <-ctx.Done():
			finalErr = ctx.Err()
			break WaitLoop
		}
	}

	wg.Wait()
	e.sendEvent(ExecutionEvent{Type: EventPipelineCompleted, Timestamp: time.Now(), Error: finalErr})

	return results, finalErr
}

func (e *PipelineExecutor) handleNodeFailure(nodeID string, start time.Time, err error, errCh chan error, cancel context.CancelFunc) {
	e.sendEvent(ExecutionEvent{Type: EventNodeFailed, NodeID: nodeID, Timestamp: time.Now(), Duration: time.Since(start), Error: err})
	select {
	case errCh <- err:
	default:
	}
	cancel()
}

func (e *PipelineExecutor) resolveValue(nodeID string, val any, data map[string]any) (any, error) {
	switch v := val.(type) {
	case string:
		return e.renderer.Render(nodeID, v, data)
	case map[string]any:
		res := make(map[string]any)
		for k, v := range v {
			rv, err := e.resolveValue(nodeID, v, data)
			if err != nil {
				return nil, err
			}
			res[k] = rv
		}
		return res, nil
	case []any:
		res := make([]any, len(v))
		for i, v := range v {
			rv, err := e.resolveValue(nodeID, v, data)
			if err != nil {
				return nil, err
			}
			res[i] = rv
		}
		return res, nil
	default:
		return v, nil
	}
}
