package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/permission"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/runner"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/runner/skill"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/skills"
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/tools"
	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/templates"
	svcutils "github.com/PineappleBond/eino-demo-dev/server/internal/utils"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// runAgent starts the AI agent in a background goroutine and streams results via WS.
// The optional parentID parameter records a parent-child relationship for cascading stop.
func (s *ChatService) runAgent(
	ctx context.Context,
	userID, conversationID uuid.UUID,
	userContent string,
	nextSeq NextSeqFunc,
	pushUpdate PushUpdateFunc,
	parentID ...uuid.UUID,
) {
	// 1. Resolve template config: conversation → project → template
	var conv model.Conversation
	if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
		s.log.Error("runAgent: conversation not found", zap.Error(err))
		return
	}

	var project model.Project
	if err := s.db.Where("id = ?", conv.ProjectID).First(&project).Error; err != nil {
		s.log.Error("runAgent: project not found", zap.Error(err))
		return
	}

	tpl, ok := templates.Get(project.TemplateID)
	if !ok {
		s.log.Warn("runAgent: template not found, using fallback", zap.String("template_id", project.TemplateID))
		tpl, ok = templates.Get("01")
		if !ok {
			s.log.Error("runAgent: no fallback template available")
			return
		}
	}

	// Extract system prompt from first agent in template
	systemPrompt := ""
	if len(tpl.Agents) > 0 {
		systemPrompt = tpl.Agents[0].SystemPrompt
	}

	// Get model tier from project config (default: sonnet)
	modelTier := "sonnet"
	if tier, ok := project.Config["model_tier"].(string); ok && tier != "" {
		modelTier = tier
	}

	// 2. Create cancellable context
	runCtx, cancel := context.WithCancel(ctx)

	// 3. Build callback handler (creates messages on first chunk)
	callbackCfg := runner.RunCallbackConfig{
		UserID:         userID,
		ConversationID: conversationID,
		OnCompleteMessage: func(ctx context.Context, userID, subConvID uuid.UUID, role schema.RoleType, addr compose.Address, reasonContent string, outputContent string, usage *schema.TokenUsage) {
			// 如果conversationID有父ID，最终结果应该以system role冒泡到主对话，并且递归runAgent
			subConv := &model.Conversation{}
			if err := s.db.Where("id = ?", subConvID).First(&subConv).Error; err != nil {
				s.log.Error("runAgent: conversation not found", zap.Error(err))
				return
			}
			if subConv.ParentConversationID == nil || *subConv.ParentConversationID == subConvID || *subConv.ParentConversationID == uuid.Nil {
				// 没有父对话
				return
			}
			conv := &model.Conversation{}
			if err := s.db.Where("id = ?", *subConv.ParentConversationID).First(&conv).Error; err != nil {
				s.log.Error("runAgent: parent conversation not found", zap.Error(err))
				return
			}
			s.sendMessageWithRole(ctx, userID, conv.ID, "<sub_agent_result>\n"+
				""+outputContent+
				"\n</sub_agent_result>", "assistant", "sub_agent_result", nextSeq, pushUpdate)
		},
		DB:         s.db,
		Log:        s.log,
		NextSeq:    nextSeq,
		PushUpdate: pushUpdate,
		ParentCtx:  ctx, // parent context, not cancelled — used for lifecycle methods
		OnComplete: func(success bool, summary string) {
			s.mu.Lock()
			cb, ok := s.runCompleteCallbacks[conversationID]
			delete(s.runCompleteCallbacks, conversationID)
			s.mu.Unlock()
			if ok && cb != nil {
				cb(success, summary)
			}
		},
	}
	callbacks := runner.NewRootRunnerCallbacks(callbackCfg)

	// 4. Register session (stop func marks in-progress messages as stopped in DB)
	// TryStart is atomic: if another runAgent already registered, this returns false
	// and we exit gracefully to prevent concurrent agent runs.
	pid := uuid.Nil
	if len(parentID) > 0 {
		pid = parentID[0]
	}
	if !s.runSessionMgr.TryStart(conversationID, cancel, callbacks.Stop, pid) {
		// Another agent run is active — enqueue the message so it will be
		// consumed by the context injection middleware at the next model call
		// boundary, or by the deferred pending-check when the current run ends.
		s.messageQueue.Enqueue(conversationID, userContent)
		s.log.Info("runAgent: another agent run active, enqueued message",
			zap.String("conv", conversationID.String()),
		)
		cancel()
		return
	}

	// 5. Token counter — created once, reused for both pre-execution check and runtime overflow detection.
	// Limit: 80K tokens — beyond this, compaction is recommended.
	const maxPromptTokens = 80000
	var tokenCounter *svcutils.TokenCounter
	if tc, err := svcutils.NewTokenCounter(modelTier); err != nil {
		s.log.Warn("runAgent: failed to create token counter, disabling token checks", zap.Error(err))
	} else {
		tokenCounter = tc
	}

	// Max iteration scales with model tier: haiku for simple tasks, opus for
	// complex reasoning chains.
	maxIteration := map[string]int{"haiku": 20, "sonnet": 50, "opus": 100}[modelTier]
	if maxIteration == 0 {
		maxIteration = 50 // default to sonnet
	}

	// Extract workspace info for evaluator and runner config
	workspaceDir := ""
	if wd, ok := project.Config["workspace_dir"].(string); ok && wd != "" {
		workspaceDir = wd
	}

	isGitRepo := false
	if workspaceDir != "" {
		if _, err := os.Stat(filepath.Join(workspaceDir, ".git")); err == nil {
			isGitRepo = true
		}
	}

	bc := tools.ToolBuildContext{
		ConversationID: conversationID,
		UserID:         userID,
		WorkspaceDir:   workspaceDir,
	}

	runCfg := runner.RootRunnerConfig{
		ModelProvider: s.modelProvider,
		ModelTier:     modelTier,
		SystemPrompt:  systemPrompt,
		Tools: func() []tool.BaseTool {
			tls := s.toolRegistry.GetBaseTools(bc)
			// Append filesystem and HTTP tools (they implement NeedPermissioner).
			tls = append(tls, s.toolRegistry.GetPermissionTools(workspaceDir)...)
			return tls
		}(),
		MaxIteration:     maxIteration,
		ConversationID:   conversationID,
		MessageQueue:     s.messageQueue,
		ReductionEnabled: true,
		SkillBackend: func() skill.Backend {
			skillsDir := filepath.Join(workspaceDir, ".skills")
			if _, err := os.Stat(skillsDir); err != nil {
				return nil // no skills dir, skip skill support
			}
			b, err := skills.NewSkillFileBackend(skillsDir)
			if err != nil {
				s.log.Warn("runAgent: failed to create skill backend, disabling skills", zap.Error(err))
				return nil
			}
			return b
		}(),
		ModelHub: skills.NewModelHub(s.modelProvider),
		TokenCheck: &runner.TokenCheckConfig{
			Counter:   tokenCounter,
			MaxTokens: maxPromptTokens,
			OnOverflow: func(tokenCount int) {
				s.log.Warn("runAgent: token overflow detected during agent run",
					zap.Int("tokens", tokenCount),
					zap.Int("limit", maxPromptTokens),
					zap.String("conv", conversationID.String()),
				)
				// Do NOT call callbacks.Stop() here — the event loop handles
				// stopping after catching TokenOverflowError. Calling Stop()
				// here would emit a duplicate message.stop update.
			},
		},
		PermissionMW: func() *permission.Middleware {
			// Create evaluator here where workspace info is available.
			var evaluator permission.SafetyEvaluator
			evalModel, err := openai.NewChatModel(context.Background(), s.evalModelConfig)
			if err != nil {
				s.log.Warn("runAgent: failed to create permission evaluator model, using fallback", zap.Error(err))
			} else {
				evaluator = permission.NewLLMReviewer(evalModel, workspaceDir, isGitRepo)
			}

			mwCfg := permission.MiddlewareConfig{
				DB:             s.db,
				ProjectID:      project.ID,
				UserID:         userID,
				ConversationID: conversationID,
				Threshold:      2,
				Evaluator:      evaluator,
				Tools: func() []tool.BaseTool {
					tls := s.toolRegistry.GetBaseTools(bc)
					tls = append(tls, s.toolRegistry.GetPermissionTools(workspaceDir)...)
					return tls
				}(),
				PushUpdate: pushUpdate,
				NextSeq:    nextSeq,
				Mode:       permission.ConversationMode(conv.Mode),
			}
			return permission.NewMiddleware(mwCfg)
		}(),
		WorkspaceDir: workspaceDir,
		IsGitRepo:    isGitRepo,
		SummarizationCallback: func(cbCtx context.Context, compressedMsgCount int) {
			s.log.Info("runAgent: summarization callback triggered",
				zap.Int("compressed_count", compressedMsgCount),
				zap.String("conv", conversationID.String()),
			)
			// The summarization middleware already compressed state.Messages in memory.
			// We need to sync the compression to DB: insert summary message, update min_seq.
			// This is done asynchronously to not block the agent.
			go func() {
				if err := s.compressionSvc.SyncSummarizationToDB(
					context.WithoutCancel(cbCtx),
					userID, conversationID, compressedMsgCount,
					nextSeq, pushUpdate,
				); err != nil {
					s.log.Error("runAgent: failed to sync summarization to DB", zap.Error(err))
				}
			}()
		},
		Log: s.log,
	}

	rootRunner, err := runner.NewRootRunner(runCtx, runCfg, callbacks)
	if err != nil {
		s.log.Error("runAgent: failed to create runner", zap.Error(err))
		cancel()
		s.runSessionMgr.Cleanup(conversationID)
		return
	}

	// 5. Create handler
	handler := runner.NewRootRunnerHandler(callbacks, nil)

	// 7. Check for existing checkpoint to resume
	checkpointID, hasCheckpoint := s.runSessionMgr.GetCheckpointID(conversationID)

	// Fall back to DB checkpoint if session was cleaned up (e.g., after interrupt).
	if !hasCheckpoint && conv.CheckpointID != "" {
		checkpointID = conv.CheckpointID
		hasCheckpoint = true
	}

	var iter *adk.AsyncIterator[*adk.AgentEvent]

	// 6. Load conversation history — shared function handles tool_calling and tool messages.
	messages, err := loadConversationMessages(runCtx, s.db, conversationID, conv.MinSeq)
	if err != nil {
		s.log.Error("runAgent: failed to load conversation history", zap.Error(err))
	}

	// Check prompt token count before running agent to prevent context overflow.
	if len(messages) > 0 && tokenCounter != nil {
		roles := make([]string, 0, len(messages))
		contents := make([]string, 0, len(messages))
		for _, m := range messages {
			roles = append(roles, string(m.Role))
			contents = append(contents, m.Content)
		}
		promptTokens := tokenCounter.CountMessages(roles, contents)
		s.log.Info("runAgent: prompt token estimate",
			zap.Int("tokens", promptTokens),
			zap.Int("limit", maxPromptTokens),
			zap.Int("messages", len(messages)),
		)

		// Update conversation token_prompt field and push notification
		seq, seqErr := nextSeq(ctx, userID)
		if seqErr != nil {
			s.log.Error("runAgent: seq assignment failed for token update", zap.Error(seqErr))
		} else {
			if err := s.db.WithContext(ctx).Model(&model.Conversation{}).
				Where("id = ?", conversationID).
				Updates(map[string]interface{}{
					"token_prompt": promptTokens,
				}).Error; err != nil {
				s.log.Error("runAgent: failed to update conversation token_prompt", zap.Error(err))
			}

			update := model.UserUpdate{
				UserID: userID,
				Seq:    seq,
				Type:   "conversation.updated",
				Payload: model.JSONMap{
					"conversation_id": conversationID.String(),
					"token_prompt":    promptTokens,
					"seq":             seq,
				},
			}
			if dbErr := s.db.WithContext(ctx).Create(&update).Error; dbErr != nil {
				s.log.Error("failed to persist token update", zap.Error(dbErr))
			}
			pushUpdate(userID, update)
		}

	}

	if hasCheckpoint {
		resumeParams, hasResumeParams := s.runSessionMgr.GetResumeParams(conversationID)
		if hasResumeParams {
			s.log.Info("runAgent: resuming with params",
				zap.String("checkpoint", checkpointID),
				zap.Int("targets", len(resumeParams.Targets)),
			)
			// Clear resume params so they're only used once
			s.runSessionMgr.ClearResumeParams(conversationID)
			iter, err = rootRunner.ResumeWithParams(runCtx, checkpointID, resumeParams, handler)
			if err != nil {
				s.log.Error("runAgent: resumeWithParams failed, falling back to fresh run", zap.Error(err), zap.String("checkpoint", checkpointID))
				// Pass conversationID as checkpoint key so the compose layer saves state
				iter = rootRunner.Run(runCtx, messages, conversationID.String(), handler)
			}
		} else {
			iter, err = rootRunner.Resume(runCtx, checkpointID, handler)
			if err != nil {
				s.log.Error("runAgent: resume failed, falling back to fresh run", zap.Error(err), zap.String("checkpoint", checkpointID))
				// Pass conversationID as checkpoint key so the compose layer saves state
				iter = rootRunner.Run(runCtx, messages, conversationID.String(), handler)
			}
		}
	} else {
		// Pass conversationID as checkpoint key so the compose layer saves state
		// to the store on interrupt. Without this, WithCheckPointID is not set,
		// the checkpoint is never persisted, and ResumeWithParams fails with
		// "checkpoint not exist".
		iter = rootRunner.Run(runCtx, messages, conversationID.String(), handler)
	}

	// 8. Consume events in goroutine with timeout protection and panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("runAgent event loop panic recovered", zap.Any("recover", r))
				callbacks.Stop()
				s.runSessionMgr.Cleanup(conversationID)
				s.messageQueue.Cleanup(conversationID)
				s.runSessionMgr.Done(conversationID)
				cancel()
			}
		}()
		defer func() {
			// Before cleaning up, check for pending messages that were enqueued
			// after the last Drain. This happens when the agent finishes
			// naturally before consuming all queued messages.
			if s.messageQueue.HasPending(conversationID) {
				s.log.Info("runAgent: pending messages remain after agent end, spawning new run",
					zap.String("conv", conversationID.String()),
				)
				s.runSessionMgr.Cleanup(conversationID)
				s.messageQueue.Cleanup(conversationID)
				s.runSessionMgr.Done(conversationID)
				cancel()
				go func() {
					defer func() {
						if r := recover(); r != nil {
							s.log.Error("runAgent (defer spawn) panic recovered", zap.Any("recover", r))
						}
					}()
					s.runAgent(context.WithoutCancel(ctx), userID, conversationID, userContent, nextSeq, pushUpdate)
				}()
				return
			}
			s.runSessionMgr.Cleanup(conversationID)
			s.messageQueue.Cleanup(conversationID)
			s.runSessionMgr.Done(conversationID)
		}()
		defer cancel()

		timer := time.NewTimer(agentRunTimeout)
		defer timer.Stop()

		done := make(chan struct{})
		go func() {
			for {
				event, ok := iter.Next()
				if !ok {
					callbacks.OnEnd()
					break
				}
				if event == nil {
					continue
				}
				if event.Err != nil {
					// Check for our TokenOverflowError first (carries drained messages).
					var overflowErr *runner.TokenOverflowError
					if errors.As(event.Err, &overflowErr) {
						s.log.Warn("runAgent: token overflow detected, compressing and restarting",
							zap.Int("tokens", overflowErr.TokenCount),
							zap.Int("limit", overflowErr.MaxTokens),
							zap.String("conv", conversationID.String()),
							zap.Int("drained_messages", len(overflowErr.DrainedMessages)),
						)
						callbacks.Stop()

						// Re-enqueue drained messages before compression, so they survive
						// the fresh run after the event loop exits.
						for _, content := range overflowErr.DrainedMessages {
							s.messageQueue.Enqueue(conversationID, content)
						}

						// Compress in-place. After compression, the DB has updated min_seq
						// and a summary message. The fresh runAgent will load from DB
						// with seq >= min_seq.
						if err := s.compressionSvc.CompressAndResume(
							context.WithoutCancel(ctx), userID, conversationID, nextSeq, pushUpdate,
						); err != nil {
							s.log.Error("runAgent: compress failed before restart", zap.Error(err))
							// Fall through to error push below
						} else {
							s.log.Info("runAgent: compression complete, restarting agent",
								zap.String("conv", conversationID.String()),
							)
							// Restart agent with same params. Must Stop + Cleanup the
							// old session first — the old event loop is still running
							// (we are inside it) and its session is still in the map.
							// TryStart in the new runAgent would fail if we don't
							// clean up first.
							go func() {
								defer func() {
									if r := recover(); r != nil {
										s.log.Error("runAgent (post-compress) panic recovered", zap.Any("recover", r))
									}
								}()
								// Stop the old run and wait for its event loop to drain.
								// This is safe to call from within the old event loop
								// goroutine — it cancels the context and waits up to 2s.
								s.runSessionMgr.Stop(conversationID)
								s.runSessionMgr.Cleanup(conversationID)
								s.runSessionMgr.ClearCheckpointID(conversationID)
								s.runAgent(context.WithoutCancel(ctx), userID, conversationID, userContent, nextSeq, pushUpdate)
							}()
							return
						}
					} else if isContextOverflowError(event.Err) {
						s.log.Warn("runAgent: context overflow detected (API-level), stopping agent",
							zap.Error(event.Err),
							zap.String("conv", conversationID.String()),
						)
						callbacks.Stop()
					} else {
						callbacks.OnError(event.Err)
						break
					}
					// Push user-friendly error message (for both overflow types)
					errSeq, err := nextSeq(ctx, userID)
					if err != nil {
						s.log.Error("runAgent: seq assignment failed for overflow error", zap.Error(err))
					} else {
						errUpdate := model.UserUpdate{
							UserID: userID,
							Seq:    errSeq,
							Type:   "message.error",
							Payload: model.JSONMap{
								"conversation_id": conversationID.String(),
								"error":           ErrContextLimitExceeded.Error(),
							},
						}
						if dbErr := s.db.WithContext(ctx).Create(&errUpdate).Error; dbErr != nil {
							s.log.Error("failed to persist overflow error update", zap.Error(dbErr))
						}
						pushUpdate(userID, errUpdate)
					}
					break
				}
				if event.Action != nil && event.Action.Interrupted != nil {
					s.log.Info("runAgent: received interrupted event",
						zap.String("conv", conversationID.String()),
						zap.Int("interrupt_contexts", len(event.Action.Interrupted.InterruptContexts)),
					)
					// Use conversationID as the checkpoint key — this is the same key
					// passed to RootRunner.Run via WithCheckPointID. The InterruptContexts[0].ID
					// is an address-derived string that is NOT the checkpoint store key.
					checkpointKey := conversationID.String()
					s.runSessionMgr.SetCheckpointID(conversationID, checkpointKey)

					// Persist checkpoint to DB so it survives session cleanup.
					if err := s.db.WithContext(ctx).
						Model(&model.Conversation{}).
						Where("id = ?", conversationID).
						Update("checkpoint_id", checkpointKey).Error; err != nil {
						s.log.Error("failed to persist checkpoint_id to conversation",
							zap.String("conv", conversationID.String()),
							zap.Error(err),
						)
					}

					// OnInterrupted callback handles permission/HITL record updates and WS pushes
					callbacks.OnInterrupted(event.Action.Interrupted)
					break
				}
			}
			close(done)
		}()

		select {
		case <-done:
			// Normal completion
		case <-timer.C:
			s.log.Warn("runAgent: event loop timed out", zap.Duration("timeout", agentRunTimeout))
		}
	}()
}
