package engine

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/teoclub/hermes-forge/internal/provider"
	"github.com/teoclub/hermes-forge/internal/schema"
	"github.com/teoclub/hermes-forge/internal/tools"
)

func TestRunWithoutToolsSendsFinalMessageAndStops(t *testing.T) {
	p := &mockProvider{
		responses: []*schema.Response{
			assistantResponse("done"),
		},
	}
	r := &mockRegistry{
		defs: []schema.ToolDefinition{{Name: "read_file"}},
	}
	reporter := &recordingReporter{}
	eng := NewAgentEngine(p, r, "/tmp/work", false)

	if err := eng.Run(context.Background(), "hello", reporter); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(p.calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(p.calls))
	}
	if len(p.calls[0].prompt) != 2 {
		t.Fatalf("initial prompt length = %d, want 2", len(p.calls[0].prompt))
	}
	if p.calls[0].prompt[0].Role != schema.RoleSystem {
		t.Fatalf("first prompt role = %s, want system", p.calls[0].prompt[0].Role)
	}
	if got := schema.MessageText(p.calls[0].prompt[1].Content); got != "hello" {
		t.Fatalf("user prompt = %q, want hello", got)
	}
	if !reflect.DeepEqual(p.calls[0].tools, r.defs) {
		t.Fatalf("available tools = %#v, want %#v", p.calls[0].tools, r.defs)
	}
	if got := reporter.snapshot(); !reflect.DeepEqual(got, []string{"message:done"}) {
		t.Fatalf("reporter events = %#v", got)
	}
}

func TestRunWithThinkingUsesPlanningCallBeforeToolEnabledAction(t *testing.T) {
	p := &mockProvider{
		responses: []*schema.Response{
			assistantResponse("plan first"),
			assistantResponse("final answer"),
		},
	}
	r := &mockRegistry{
		defs: []schema.ToolDefinition{{Name: "bash"}},
	}
	reporter := &recordingReporter{}
	eng := NewAgentEngine(p, r, "/tmp/work", true)

	if err := eng.Run(context.Background(), "do it", reporter); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(p.calls) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(p.calls))
	}
	if p.calls[0].tools != nil {
		t.Fatalf("thinking call tools = %#v, want nil", p.calls[0].tools)
	}
	if !reflect.DeepEqual(p.calls[1].tools, r.defs) {
		t.Fatalf("action call tools = %#v, want %#v", p.calls[1].tools, r.defs)
	}
	if got := schema.MessageText(p.calls[1].prompt[len(p.calls[1].prompt)-1].Content); got != "plan first" {
		t.Fatalf("action prompt last message = %q, want thinking text", got)
	}
	if got := reporter.snapshot(); !reflect.DeepEqual(got, []string{"thinking", "message:final answer"}) {
		t.Fatalf("reporter events = %#v", got)
	}
}

func TestRunExecutesToolCallsAndFeedsOrderedToolResultsBackToProvider(t *testing.T) {
	firstCall := schema.ToolCall{ID: "call_1", Name: "first", Arguments: json.RawMessage(`{"value":1}`)}
	secondCall := schema.ToolCall{ID: "call_2", Name: "second", Arguments: json.RawMessage(`{"value":2}`)}
	p := &mockProvider{
		responses: []*schema.Response{
			{
				Message: schema.Message{
					Role:      schema.RoleAssistant,
					ToolCalls: []schema.ToolCall{firstCall, secondCall},
				},
			},
			assistantResponse("finished"),
		},
	}
	r := &mockRegistry{
		results: map[string]schema.ToolResult{
			"call_1": {ToolCallID: "call_1", Output: "first result"},
			"call_2": {ToolCallID: "call_2", Output: "second result"},
		},
	}
	reporter := &recordingReporter{}
	eng := NewAgentEngine(p, r, "/tmp/work", false)

	if err := eng.Run(context.Background(), "use tools", reporter); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(p.calls) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(p.calls))
	}
	secondPrompt := p.calls[1].prompt
	if len(secondPrompt) < 2 {
		t.Fatalf("second prompt too short: %#v", secondPrompt)
	}
	gotToolMessages := secondPrompt[len(secondPrompt)-2:]
	wantIDs := []string{"call_1", "call_2"}
	wantOutputs := []string{"first result", "second result"}
	for i, msg := range gotToolMessages {
		if msg.Role != schema.RoleTool {
			t.Fatalf("tool message %d role = %s, want %s", i, msg.Role, schema.RoleTool)
		}
		if msg.ToolCallID != wantIDs[i] {
			t.Fatalf("tool message %d id = %q, want %q", i, msg.ToolCallID, wantIDs[i])
		}
		if got := schema.MessageText(msg.Content); got != wantOutputs[i] {
			t.Fatalf("tool message %d content = %q, want %q", i, got, wantOutputs[i])
		}
	}
	gotExecutedIDs := r.executedIDs()
	sort.Strings(gotExecutedIDs)
	if !reflect.DeepEqual(gotExecutedIDs, wantIDs) {
		t.Fatalf("executed ids = %#v, want %#v", gotExecutedIDs, wantIDs)
	}
	assertReporterContains(t, reporter, "tool-call:first:{\"value\":1}")
	assertReporterContains(t, reporter, "tool-call:second:{\"value\":2}")
	assertReporterContains(t, reporter, "tool-result:first:false:first result")
	assertReporterContains(t, reporter, "tool-result:second:false:second result")
	assertReporterContains(t, reporter, "message:finished")
}

func TestRunReportsToolErrorsAndStillFeedsObservation(t *testing.T) {
	call := schema.ToolCall{ID: "call_error", Name: "broken", Arguments: json.RawMessage(`{}`)}
	p := &mockProvider{
		responses: []*schema.Response{
			{
				Message: schema.Message{
					Role:      schema.RoleAssistant,
					ToolCalls: []schema.ToolCall{call},
				},
			},
			assistantResponse("handled"),
		},
	}
	r := &mockRegistry{
		results: map[string]schema.ToolResult{
			"call_error": {ToolCallID: "call_error", Output: "boom", IsError: true},
		},
	}
	reporter := &recordingReporter{}
	eng := NewAgentEngine(p, r, "/tmp/work", false)

	if err := eng.Run(context.Background(), "use broken tool", reporter); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	secondPrompt := p.calls[1].prompt
	gotObservation := secondPrompt[len(secondPrompt)-1]
	if gotObservation.Role != schema.RoleTool || gotObservation.ToolCallID != "call_error" {
		t.Fatalf("tool observation = %#v", gotObservation)
	}
	if got := schema.MessageText(gotObservation.Content); got != "boom" {
		t.Fatalf("tool observation content = %q, want boom", got)
	}
	assertReporterContains(t, reporter, "tool-result:broken:true:boom")
}

func TestRunReturnsProviderErrors(t *testing.T) {
	t.Run("thinking phase", func(t *testing.T) {
		p := &mockProvider{errors: []error{errors.New("thinking failed")}}
		eng := NewAgentEngine(p, &mockRegistry{}, "/tmp/work", true)

		err := eng.Run(context.Background(), "prompt", nil)
		if err == nil || !strings.Contains(err.Error(), "Thinking 阶段失败") {
			t.Fatalf("Run() error = %v, want thinking phase error", err)
		}
	})

	t.Run("action phase", func(t *testing.T) {
		p := &mockProvider{errors: []error{errors.New("action failed")}}
		eng := NewAgentEngine(p, &mockRegistry{}, "/tmp/work", false)

		err := eng.Run(context.Background(), "prompt", nil)
		if err == nil || !strings.Contains(err.Error(), "Action 阶段失败") {
			t.Fatalf("Run() error = %v, want action phase error", err)
		}
	})
}

func TestRunWithZeroValueMocksSimulatesThinkingToolAndFinalAnswer(t *testing.T) {
	p := &mockProvider{}
	r := &mockRegistry{}
	reporter := &recordingReporter{}
	eng := NewAgentEngine(p, r, "/tmp/work", true)

	if err := eng.Run(context.Background(), "检查当前目录", reporter); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(p.calls) != 4 {
		t.Fatalf("provider calls = %d, want 4", len(p.calls))
	}
	if p.calls[0].tools != nil {
		t.Fatalf("first thinking tools = %#v, want nil", p.calls[0].tools)
	}
	if got := p.calls[1].tools; len(got) != 1 || got[0].Name != "bash" {
		t.Fatalf("first action tools = %#v, want bash tool", got)
	}
	if got := r.executedIDs(); !reflect.DeepEqual(got, []string{"call_123"}) {
		t.Fatalf("executed ids = %#v, want [call_123]", got)
	}

	secondThinkingPrompt := p.calls[2].prompt
	gotObservation := secondThinkingPrompt[len(secondThinkingPrompt)-1]
	if gotObservation.Role != schema.RoleTool || gotObservation.ToolCallID != "call_123" {
		t.Fatalf("tool observation = %#v", gotObservation)
	}
	if got := schema.MessageText(gotObservation.Content); got != "-rw-r--r--  1 user group  234 Oct 24 10:00 main.go\n" {
		t.Fatalf("tool observation content = %q", got)
	}

	assertReporterContains(t, reporter, "thinking")
	assertReporterContains(t, reporter, "tool-call:bash:{\"command\": \"ls -la\"}")
	assertReporterContains(t, reporter, "tool-result:bash:false:-rw-r--r--  1 user group  234 Oct 24 10:00 main.go\n")
	assertReporterContains(t, reporter, "message:根据工具返回的结果，我看到了 main.go，任务圆满完成！")
}

type providerCall struct {
	prompt []schema.Message
	tools  []schema.ToolDefinition
}

type mockProvider struct {
	mu        sync.Mutex
	calls     []providerCall
	responses []*schema.Response
	errors    []error
	turn      int
}

func (p *mockProvider) Name() string {
	return "fake"
}

func (p *mockProvider) Generate(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...provider.Option) (*schema.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	callIndex := len(p.calls)
	p.calls = append(p.calls, providerCall{
		prompt: append([]schema.Message(nil), prompt...),
		tools:  append([]schema.ToolDefinition(nil), availableTools...),
	})
	if callIndex < len(p.errors) && p.errors[callIndex] != nil {
		return nil, p.errors[callIndex]
	}
	if len(p.responses) == 0 && len(p.errors) == 0 {
		return p.generateDefaultResponse(availableTools), nil
	}
	if callIndex >= len(p.responses) {
		return nil, errors.New("unexpected Generate call")
	}
	return p.responses[callIndex], nil
}

func (p *mockProvider) generateDefaultResponse(availableTools []schema.ToolDefinition) *schema.Response {
	if len(availableTools) == 0 {
		return assistantResponse("【推理中】目标是检查文件。我不能直接盲猜，我需要先调用 bash 工具执行 ls 命令，看看当前目录下有什么，然后再做定夺。")
	}

	p.turn++
	if p.turn == 1 {
		return &schema.Response{
			Message: schema.Message{
				Role:    schema.RoleAssistant,
				Content: []schema.ContentPart{schema.Text("我要执行我刚才计划的步骤了。")},
				ToolCalls: []schema.ToolCall{
					{ID: "call_123", Name: "bash", Arguments: json.RawMessage(`{"command": "ls -la"}`)},
				},
			},
		}
	}

	return assistantResponse("根据工具返回的结果，我看到了 main.go，任务圆满完成！")
}

func (p *mockProvider) Stream(ctx context.Context, prompt []schema.Message, availableTools []schema.ToolDefinition, opts ...provider.Option) (<-chan *schema.StreamChunk, error) {
	ch := make(chan *schema.StreamChunk)
	close(ch)
	return ch, nil
}

type mockRegistry struct {
	mu      sync.Mutex
	defs    []schema.ToolDefinition
	results map[string]schema.ToolResult
	calls   []schema.ToolCall
}

func (r *mockRegistry) Register(tool tools.Tool) {
	panic("mockRegistry.Register should not be called")
}

func (r *mockRegistry) Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, call)
	if result, ok := r.results[call.ID]; ok {
		result.ToolCallID = call.ID
		return result
	}
	return schema.ToolResult{
		ToolCallID: call.ID,
		Output:     "-rw-r--r--  1 user group  234 Oct 24 10:00 main.go\n",
		IsError:    false,
	}
}

func (r *mockRegistry) GetAvailableTools() []schema.ToolDefinition {
	if r.defs == nil {
		return []schema.ToolDefinition{{Name: "bash"}}
	}
	return append([]schema.ToolDefinition(nil), r.defs...)
}

func (r *mockRegistry) executedIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := make([]string, 0, len(r.calls))
	for _, call := range r.calls {
		ids = append(ids, call.ID)
	}
	return ids
}

type recordingReporter struct {
	mu     sync.Mutex
	events []string
}

func (r *recordingReporter) OnThinking(ctx context.Context) {
	r.append("thinking")
}

func (r *recordingReporter) OnToolCall(ctx context.Context, toolName string, args string) {
	r.append("tool-call:" + toolName + ":" + args)
}

func (r *recordingReporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	r.append("tool-result:" + toolName + ":" + boolString(isError) + ":" + result)
}

func (r *recordingReporter) OnMessage(ctx context.Context, content string) {
	r.append("message:" + content)
}

func (r *recordingReporter) append(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recordingReporter) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

func assistantResponse(text string) *schema.Response {
	return &schema.Response{
		Message: schema.Message{
			Role:    schema.RoleAssistant,
			Content: []schema.ContentPart{schema.Text(text)},
		},
	}
}

func assertReporterContains(t *testing.T, reporter *recordingReporter, want string) {
	t.Helper()

	for _, event := range reporter.snapshot() {
		if event == want {
			return
		}
	}
	t.Fatalf("reporter events = %#v, want to contain %q", reporter.snapshot(), want)
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
