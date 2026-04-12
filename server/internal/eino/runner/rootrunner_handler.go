package runner

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/PineappleBond/eino-demo-dev/server/internal/utils"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/atomic"
)

var _handler callbacks.Handler = &RootRunnerHandler{}

type RootRunnerHandlerCallback interface {
	OnInputToolCalling(ctx context.Context, info *callbacks.RunInfo, addr compose.Address, input callbacks.CallbackInput)
	OnOutputToolCalling(ctx context.Context, info *callbacks.RunInfo, addr compose.Address, output callbacks.CallbackOutput)
	OnThinking(ctx context.Context, role schema.RoleType, addr compose.Address, reasoningContent string)
	OnOutputting(ctx context.Context, role schema.RoleType, addr compose.Address, content string)
	OnCompleted(ctx context.Context, role schema.RoleType, addr compose.Address, reasoningContent string, outputContent string, usage *schema.TokenUsage)
}
type RootRunnerHandler struct {
	callback         RootRunnerHandlerCallback
	tracer           trace.Tracer
	onCompletedTimes *atomic.Int32
}

func (h *RootRunnerHandler) OnStart(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
	if info != nil {
		if info.Component == components.ComponentOfTool {
			_, span := h.tracer.Start(ctx, "OnStart:"+string(info.Component)+":"+info.Name)
			span.SetAttributes(attribute.String("input", utils.TruncateString(fmt.Sprintf("%#v", input), 100)))
			h.callback.OnInputToolCalling(ctx, info, compose.GetCurrentAddress(ctx), input)
			span.End()
		} else if info.Component == "ToolsNode" {

		}
	}
	return ctx
}

func (h *RootRunnerHandler) OnEnd(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
	if info != nil {
		if info.Component == components.ComponentOfTool {
			_, span := h.tracer.Start(ctx, "OnEnd:"+string(info.Component)+":"+info.Name)
			span.SetAttributes(attribute.String("output", utils.TruncateString(fmt.Sprintf("%#v", output), 100)))
			h.callback.OnOutputToolCalling(ctx, info, compose.GetCurrentAddress(ctx), output)
			span.End()
		}
	}
	return ctx
}

func (h *RootRunnerHandler) OnEndWithStreamOutput(ctx context.Context, info *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
	_, span := h.tracer.Start(ctx, "OnEndWithStreamOutput:"+string(info.Component)+":"+info.Name)
	defer span.End()
	defer output.Close()
	for {
		chunk, err := output.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				span.SetStatus(codes.Error, err.Error())
			}
			break
		}
		modelOutput := einomodel.ConvCallbackOutput(chunk)
		if modelOutput == nil {
			continue
		}
		msg := modelOutput.Message // 不能为nil
		if msg == nil {
			continue
		}
		tokenUsage := modelOutput.TokenUsage // 有可能为nil
		outputExtra := modelOutput.Extra     // 有可能为nil
		_, _ = tokenUsage, outputExtra
		responseMeta := msg.ResponseMeta
		/*
			const (
				// Assistant is the role of an assistant, means the message is returned by ChatModel.
				Assistant RoleType = "assistant"
				// User is the role of a user, means the message is a user message.
				User RoleType = "user"
				// System is the role of a system, means the message is a system message.
				System RoleType = "system"
				// Tool is the role of a tool, means the message is a tool call output.
				Tool RoleType = "tool"
			)
		*/
		role := msg.Role
		content := msg.Content
		reasoningContent := msg.ReasoningContent
		addr := compose.GetCurrentAddress(ctx)
		if responseMeta == nil || responseMeta.FinishReason == "" {
			// 流式传输中...
			if reasoningContent != "" {
				// 正在思考
				// 递增的，不是全部内容
				// 1
				// . **分析
				// 用户输入: **
				// ...
				h.callback.OnThinking(ctx, role, addr, reasoningContent)
			} else if content != "" {
				h.callback.OnOutputting(ctx, role, addr, content)
			}
		} else {
			// 生成完毕
			span.SetAttributes(
				attribute.String("role", string(role)),
				attribute.String("content", utils.TruncateString(content, 100)),
				attribute.String("reasoning_content", utils.TruncateString(reasoningContent, 100)),
				attribute.String("response_meta", utils.TruncateString(responseMeta.FinishReason, 100)),
			)
			if responseMeta.Usage != nil {
				span.SetAttributes(
					attribute.Int("Usage.CompletionTokens", responseMeta.Usage.CompletionTokens),
					attribute.Int("Usage.TotalTokens", responseMeta.Usage.TotalTokens),
					attribute.Int("Usage.PromptTokens", responseMeta.Usage.PromptTokens),
				)
			}
			if h.onCompletedTimes.Add(1) == 1 {
				h.callback.OnCompleted(ctx, role, addr, reasoningContent, content, responseMeta.Usage)
			}
		}

	}
	return ctx
}

func NewRootRunnerHandler(callback RootRunnerHandlerCallback, tracer trace.Tracer) *RootRunnerHandler {
	if tracer == nil {
		tracer = otel.Tracer("eino-demo:RootRunner-handler")
	}
	return &RootRunnerHandler{callback: callback, tracer: tracer, onCompletedTimes: atomic.NewInt32(0)}
}

func (h *RootRunnerHandler) OnError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	if info != nil {
		_, span := h.tracer.Start(ctx, "OnError:"+string(info.Component)+":"+info.Name)
		defer span.End()
		span.SetStatus(codes.Error, err.Error())
	}
	return ctx
}

func (h *RootRunnerHandler) OnStartWithStreamInput(ctx context.Context, info *callbacks.RunInfo, input *schema.StreamReader[callbacks.CallbackInput]) context.Context {
	return ctx
}
