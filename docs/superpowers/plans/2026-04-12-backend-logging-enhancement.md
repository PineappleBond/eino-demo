# Backend Logging Enhancement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add structured zap logging to all handler, service, and eino files that currently have zero or minimal logging, enabling production troubleshooting without a debugger.

**Architecture:** Thread `*zap.Logger` through handler registration functions (already available in DI), add `s.log.Info/Error/Warn/Debug` calls at entry points, error paths, and key mutations in service and eino files. Follow existing patterns from `chat.go` and `rootrunner_callbacks.go`.

**Tech Stack:** Go `go.uber.org/zap`, Fx DI, GORM

---

## File Map

| File | Change Type | Responsibility |
|------|-------------|----------------|
| `server/internal/di/module.go` | Modify | Pass `log` to all Register*Routes that lack it |
| `server/internal/handler/conversation.go` | Modify | Add `log` param, log each endpoint |
| `server/internal/handler/project.go` | Modify | Add `log` param, log each endpoint |
| `server/internal/handler/template.go` | Modify | Add `log` param, log each endpoint |
| `server/internal/handler/settings.go` | Modify | Add `log` param, log each endpoint |
| `server/internal/handler/user.go` | Modify | Add entry logs to existing endpoints |
| `server/internal/service/conversation.go` | Modify | Add logs to CRUD and mutation operations |
| `server/internal/service/project.go` | Modify | Add logs to CRUD and mutation operations |
| `server/internal/service/template.go` | Modify | Add logs to template listing and project creation |
| `server/internal/service/user.go` | Modify | Add logs to GetMe |
| `server/internal/eino/runner/rootrunner.go` | Modify | Add `Log` to config, log runner creation |

---

### Task 1: Wire logger into handler registration functions (DI + signatures)

**Files:**
- Modify: `server/internal/handler/conversation.go:19`
- Modify: `server/internal/handler/project.go:17`
- Modify: `server/internal/handler/template.go:17`
- Modify: `server/internal/handler/settings.go:17`
- Modify: `server/internal/di/module.go:83-86`

Add `log *zap.Logger` parameter to the four registration functions that currently lack it, then update DI invocations.

- [ ] **Step 1.1: Update `handler/conversation.go` signature**

Change:
```go
// Before:
func RegisterConversationRoutes(
    api *gin.RouterGroup,
    svc *service.ConversationService,
    chatSvc *service.ChatService,
    wsManager *ws.Manager,
) {
```
To:
```go
// After:
func RegisterConversationRoutes(
    api *gin.RouterGroup,
    svc *service.ConversationService,
    chatSvc *service.ChatService,
    wsManager *ws.Manager,
    log *zap.Logger,
) {
```

Add import `"go.uber.org/zap"` to the imports block (check if already present; if not, add it).

- [ ] **Step 1.2: Update `handler/project.go` signature**

Change:
```go
// Before:
func RegisterProjectRoutes(api *gin.RouterGroup, svc *service.ProjectService, wsManager *ws.Manager) {
```
To:
```go
// After:
func RegisterProjectRoutes(api *gin.RouterGroup, svc *service.ProjectService, wsManager *ws.Manager, log *zap.Logger) {
```

Add import `"go.uber.org/zap"`.

- [ ] **Step 1.3: Update `handler/template.go` signature**

Change:
```go
// Before:
func RegisterTemplateRoutes(api *gin.RouterGroup, svc *service.TemplateService, wsManager *ws.Manager) {
```
To:
```go
// After:
func RegisterTemplateRoutes(api *gin.RouterGroup, svc *service.TemplateService, wsManager *ws.Manager, log *zap.Logger) {
```

Add import `"go.uber.org/zap"`.

- [ ] **Step 1.4: Update `handler/settings.go` signature**

Change:
```go
// Before:
func RegisterSettingsRoutes(api *gin.RouterGroup, svc *service.SettingsService, wsManager *ws.Manager) {
```
To:
```go
// After:
func RegisterSettingsRoutes(api *gin.RouterGroup, svc *service.SettingsService, wsManager *ws.Manager, log *zap.Logger) {
```

Add import `"go.uber.org/zap"`.

- [ ] **Step 1.5: Update `di/module.go` call sites**

Change lines 83-86 from:
```go
handler.RegisterSettingsRoutes(api, settingsSvc, wsManager)
handler.RegisterTemplateRoutes(api, tplSvc, wsManager)
handler.RegisterProjectRoutes(api, projectSvc, wsManager)
handler.RegisterConversationRoutes(api, convSvc, chatSvc, wsManager)
```
To:
```go
handler.RegisterSettingsRoutes(api, settingsSvc, wsManager, log)
handler.RegisterTemplateRoutes(api, tplSvc, wsManager, log)
handler.RegisterProjectRoutes(api, projectSvc, wsManager, log)
handler.RegisterConversationRoutes(api, convSvc, chatSvc, wsManager, log)
```

- [ ] **Step 1.6: Compile check**

Run: `cd /Users/leichujun/go/src/github.com/PineappleBond/eino-demo-dev/server && go build ./...`
Expected: zero errors.

- [ ] **Step 1.7: Commit**

```bash
git add server/internal/handler/conversation.go server/internal/handler/project.go server/internal/handler/template.go server/internal/handler/settings.go server/internal/di/module.go
git commit -m "refactor: thread zap.Logger into handler registration functions"
```

---

### Task 2: Add logging to `handler/conversation.go`

**Files:**
- Modify: `server/internal/handler/conversation.go`

Add `log.Info` at entry of each endpoint and `log.Error` on each error path.

- [ ] **Step 2.1: Add logs to `ListConversations` (GET `/projects/:id/conversations`)**

After line 26 (`userID := getUserID(c)`), add:
```go
projectID, err := uuid.Parse(c.Param("id"))
if err != nil {
    log.Warn("list conversations: invalid project ID", zap.String("id", c.Param("id")))
    respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
    return
}
conversations, err := svc.ListConversations(userID, projectID)
if err != nil {
    log.Error("list conversations failed",
        zap.String("user_id", userID.String()),
        zap.String("project_id", projectID.String()),
        zap.Error(err),
    )
    respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list conversations")
    return
}
```

- [ ] **Step 2.2: Add logs to `CreateConversation` (POST `/projects/:id/conversations`)**

After line 45 (`userID := getUserID(c)`), replace the error handling block:
```go
projectID, err := uuid.Parse(c.Param("id"))
if err != nil {
    log.Warn("create conversation: invalid project ID", zap.String("id", c.Param("id")))
    respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
    return
}
var req types.PostProjectsIdConversationsJSONBody
if err := c.ShouldBindJSON(&req); err != nil {
    if err == io.EOF {
        req = types.PostProjectsIdConversationsJSONBody{}
    } else {
        log.Warn("create conversation: invalid request body",
            zap.String("user_id", userID.String()),
            zap.Error(err),
        )
        respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
        return
    }
}
svcReq := service.CreateConversationRequest{
    Title: valueOrZero(req.Title),
}
conv, err := svc.CompleteCreateConversation(
    c.Request.Context(),
    userID,
    projectID,
    svcReq,
    wsManager.NextSeq,
    func(userID uuid.UUID, update model.UserUpdate) {
        wsUpdate := convert.ToUpdate(update)
        wsManager.PushToUserConnections(userID, wsUpdate)
    },
)
if err != nil {
    log.Error("create conversation failed",
        zap.String("user_id", userID.String()),
        zap.String("project_id", projectID.String()),
        zap.Error(err),
    )
    respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to create conversation")
    return
}
log.Info("conversation created",
    zap.String("user_id", userID.String()),
    zap.String("project_id", projectID.String()),
    zap.String("conv_id", conv.ID.String()),
)
```

- [ ] **Step 2.3: Add logs to `DeleteConversation` (DELETE `/conversations/:id`)**

Replace the error handling block (lines 83-101):
```go
userID := getUserID(c)
conversationID, err := uuid.Parse(c.Param("id"))
if err != nil {
    log.Warn("delete conversation: invalid conversation ID", zap.String("id", c.Param("id")))
    respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
    return
}
if err := svc.CompleteDeleteConversation(
    c.Request.Context(),
    userID,
    conversationID,
    wsManager.NextSeq,
    func(userID uuid.UUID, update model.UserUpdate) {
        wsUpdate := convert.ToUpdate(update)
        wsManager.PushToUserConnections(userID, wsUpdate)
    },
); err != nil {
    log.Error("delete conversation failed",
        zap.String("user_id", userID.String()),
        zap.String("conv_id", conversationID.String()),
        zap.Error(err),
    )
    respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
    return
}
log.Info("conversation deleted",
    zap.String("user_id", userID.String()),
    zap.String("conv_id", conversationID.String()),
)
```

- [ ] **Step 2.4: Add logs to `RenameConversation` (PATCH `/conversations/:id`)**

Replace lines 106-136:
```go
userID := getUserID(c)
conversationID, err := uuid.Parse(c.Param("id"))
if err != nil {
    log.Warn("rename conversation: invalid conversation ID", zap.String("id", c.Param("id")))
    respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
    return
}
var req types.PatchConversationsIdJSONBody
if err := c.ShouldBindJSON(&req); err != nil {
    log.Warn("rename conversation: invalid request body",
        zap.String("user_id", userID.String()),
        zap.Error(err),
    )
    respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
    return
}
svcReq := service.RenameConversationRequest{
    Title: req.Title,
}
conv, err := svc.CompleteRenameConversation(
    c.Request.Context(),
    userID,
    conversationID,
    svcReq,
    wsManager.NextSeq,
    func(userID uuid.UUID, update model.UserUpdate) {
        wsUpdate := convert.ToUpdate(update)
        wsManager.PushToUserConnections(userID, wsUpdate)
    },
)
if err != nil {
    log.Error("rename conversation failed",
        zap.String("user_id", userID.String()),
        zap.String("conv_id", conversationID.String()),
        zap.Error(err),
    )
    respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
    return
}
log.Info("conversation renamed",
    zap.String("user_id", userID.String()),
    zap.String("conv_id", conversationID.String()),
    zap.String("title", conv.Title),
)
```

- [ ] **Step 2.5: Add logs to `UpdateStatus` (PUT `/conversations/:id`)**

Replace lines 140-170:
```go
userID := getUserID(c)
conversationID, err := uuid.Parse(c.Param("id"))
if err != nil {
    log.Warn("update conversation status: invalid conversation ID", zap.String("id", c.Param("id")))
    respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
    return
}
var req types.PutConversationsIdJSONBody
if err := c.ShouldBindJSON(&req); err != nil {
    log.Warn("update conversation status: invalid request body",
        zap.String("user_id", userID.String()),
        zap.Error(err),
    )
    respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
    return
}
svcReq := service.UpdateConversationStatusRequest{
    Status: req.Status,
}
conv, err := svc.UpdateStatus(
    c.Request.Context(),
    userID,
    conversationID,
    svcReq,
    wsManager.NextSeq,
    func(userID uuid.UUID, update model.UserUpdate) {
        wsUpdate := convert.ToUpdate(update)
        wsManager.PushToUserConnections(userID, wsUpdate)
    },
)
if err != nil {
    log.Error("update conversation status failed",
        zap.String("user_id", userID.String()),
        zap.String("conv_id", conversationID.String()),
        zap.Error(err),
    )
    respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
    return
}
log.Info("conversation status updated",
    zap.String("user_id", userID.String()),
    zap.String("conv_id", conversationID.String()),
    zap.String("status", conv.Status),
)
```

- [ ] **Step 2.6: Add logs to `ListMembers` (GET `/conversations/:id/members`)**

Replace lines 174-190:
```go
userID := getUserID(c)
conversationID, err := uuid.Parse(c.Param("id"))
if err != nil {
    log.Warn("list members: invalid conversation ID", zap.String("id", c.Param("id")))
    respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
    return
}
members, err := svc.ListMembers(userID, conversationID)
if err != nil {
    log.Error("list members failed",
        zap.String("user_id", userID.String()),
        zap.String("conv_id", conversationID.String()),
        zap.Error(err),
    )
    respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list members")
    return
}
```

- [ ] **Step 2.7: Add logs to `Compact` (POST `/conversations/:id/compact`)**

Replace lines 194-215:
```go
userID := getUserID(c)
conversationID, err := uuid.Parse(c.Param("id"))
if err != nil {
    log.Warn("compact conversation: invalid conversation ID", zap.String("id", c.Param("id")))
    respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid conversation ID")
    return
}
if err := chatSvc.StartCompaction(c.Request.Context(), userID, conversationID, wsManager.NextSeq, func(userID uuid.UUID, update model.UserUpdate) {
    wsUpdate := convert.ToUpdate(update)
    wsManager.PushToUserConnections(userID, wsUpdate)
}); err != nil {
    if errors.Is(err, service.ErrConversationNotFound) {
        log.Warn("compact conversation: not found",
            zap.String("user_id", userID.String()),
            zap.String("conv_id", conversationID.String()),
        )
        respondError(c, http.StatusNotFound, "NOT_FOUND", "conversation not found")
    } else {
        log.Error("compact conversation failed",
            zap.String("user_id", userID.String()),
            zap.String("conv_id", conversationID.String()),
            zap.Error(err),
        )
        respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
    }
    return
}
log.Info("conversation compaction started",
    zap.String("user_id", userID.String()),
    zap.String("conv_id", conversationID.String()),
)
```

- [ ] **Step 2.8: Compile check and commit**

Run: `cd /Users/leichujun/go/src/github.com/PineappleBond/eino-demo-dev/server && go build ./...`
Expected: zero errors.

```bash
git add server/internal/handler/conversation.go
git commit -m "feat: add logging to conversation handler endpoints"
```

---

### Task 3: Add logging to `handler/project.go`

**Files:**
- Modify: `server/internal/handler/project.go`

- [ ] **Step 3.1: Add logs to `ListProjects`**

After the userID line, before the svc call:
```go
userID := getUserID(c)
projects, err := svc.ListProjects(userID)
if err != nil {
    log.Error("list projects failed",
        zap.String("user_id", userID.String()),
        zap.Error(err),
    )
    respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list projects")
    return
}
```

- [ ] **Step 3.2: Add logs to `GetProject`**

Replace lines 32-44:
```go
userID := getUserID(c)
projectID, err := uuid.Parse(c.Param("id"))
if err != nil {
    log.Warn("get project: invalid project ID", zap.String("id", c.Param("id")))
    respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
    return
}
project, err := svc.GetProject(userID, projectID)
if err != nil {
    log.Error("get project failed",
        zap.String("user_id", userID.String()),
        zap.String("project_id", projectID.String()),
        zap.Error(err),
    )
    respondError(c, http.StatusNotFound, "NOT_FOUND", "project not found")
    return
}
```

- [ ] **Step 3.3: Add logs to `UpdateProject`**

Replace lines 47-72:
```go
userID := getUserID(c)
projectID, err := uuid.Parse(c.Param("id"))
if err != nil {
    log.Warn("update project: invalid project ID", zap.String("id", c.Param("id")))
    respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
    return
}
var req types.PutProjectsIdJSONBody
if err := c.ShouldBindJSON(&req); err != nil {
    log.Warn("update project: invalid request body",
        zap.String("user_id", userID.String()),
        zap.Error(err),
    )
    respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
    return
}
var name string
if req.Name != nil {
    name = *req.Name
}
var config model.JSONMap
if req.Config != nil {
    config = *req.Config
}
project, err := svc.UpdateProject(userID, projectID, name, config)
if err != nil {
    log.Error("update project failed",
        zap.String("user_id", userID.String()),
        zap.String("project_id", projectID.String()),
        zap.Error(err),
    )
    respondError(c, http.StatusNotFound, "NOT_FOUND", "project not found")
    return
}
log.Info("project updated",
    zap.String("user_id", userID.String()),
    zap.String("project_id", projectID.String()),
)
```

- [ ] **Step 3.4: Add logs to `DeleteProject`**

Replace lines 75-96:
```go
userID := getUserID(c)
projectID, err := uuid.Parse(c.Param("id"))
if err != nil {
    log.Warn("delete project: invalid project ID", zap.String("id", c.Param("id")))
    respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid project ID")
    return
}
if err := svc.CompleteDeleteProject(
    c.Request.Context(),
    userID,
    projectID,
    wsManager.NextSeq,
    func(userID uuid.UUID, update model.UserUpdate) {
        wsUpdate := convert.ToUpdate(update)
        wsManager.PushToUserConnections(userID, wsUpdate)
    },
); err != nil {
    log.Error("delete project failed",
        zap.String("user_id", userID.String()),
        zap.String("project_id", projectID.String()),
        zap.Error(err),
    )
    respondError(c, http.StatusNotFound, "NOT_FOUND", "project not found")
    return
}
log.Info("project deleted",
    zap.String("user_id", userID.String()),
    zap.String("project_id", projectID.String()),
)
```

- [ ] **Step 3.5: Compile check and commit**

Run: `cd /Users/leichujun/go/src/github.com/PineappleBond/eino-demo-dev/server && go build ./...`
Expected: zero errors.

```bash
git add server/internal/handler/project.go
git commit -m "feat: add logging to project handler endpoints"
```

---

### Task 4: Add logging to `handler/template.go` and `handler/settings.go`

**Files:**
- Modify: `server/internal/handler/template.go`
- Modify: `server/internal/handler/settings.go`

- [ ] **Step 4.1: Add logs to `handler/template.go` — ListTemplates**

Replace lines 18-24:
```go
api.GET("/templates", func(c *gin.Context) {
    list := svc.ListTemplates()
    log.Debug("list templates", zap.Int("count", len(list)))
    result := make([]types.Template, len(list))
    for i, t := range list {
        result[i] = convert.ToTemplate(t)
    }
    respondJSON(c, http.StatusOK, result)
})
```

- [ ] **Step 4.2: Add logs to `handler/template.go` — GetTemplate**

Replace lines 27-34:
```go
api.GET("/templates/:id", func(c *gin.Context) {
    id := c.Param("id")
    t, err := svc.GetTemplate(id)
    if err != nil {
        log.Warn("get template not found", zap.String("template_id", id))
        respondError(c, http.StatusNotFound, "NOT_FOUND", "template not found")
        return
    }
    respondJSON(c, http.StatusOK, convert.ToTemplate(t.TemplateInfo))
})
```

- [ ] **Step 4.3: Add logs to `handler/template.go` — CreateProjectFromTemplate**

Replace lines 37-72:
```go
api.POST("/templates/:id/projects", func(c *gin.Context) {
    userID := getUserID(c)
    templateID := c.Param("id")

    var req struct {
        Name   string         `json:"name"`
        Config map[string]any `json:"config"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        log.Warn("create project from template: invalid request body",
            zap.String("template_id", templateID),
            zap.Error(err),
        )
        respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
        return
    }

    cfg := model.JSONMap(req.Config)
    if cfg == nil {
        cfg = make(map[string]any)
    }

    project, err := svc.CompleteCreateProjectFromTemplate(
        c.Request.Context(),
        userID,
        templateID,
        req.Name,
        cfg,
        wsManager.NextSeq,
        func(userID uuid.UUID, update model.UserUpdate) {
            wsUpdate := convert.ToUpdate(update)
            wsManager.PushToUserConnections(userID, wsUpdate)
        },
    )
    if err != nil {
        log.Error("create project from template failed",
            zap.String("user_id", userID.String()),
            zap.String("template_id", templateID),
            zap.Error(err),
        )
        respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "failed to create project from template")
        return
    }
    log.Info("project created from template",
        zap.String("user_id", userID.String()),
        zap.String("template_id", templateID),
        zap.String("project_id", project.ID.String()),
    )
    respondJSON(c, http.StatusCreated, convert.ToProject(*project))
})
```

- [ ] **Step 4.4: Add logs to `handler/settings.go` — GetSettings**

Replace lines 18-25:
```go
api.GET("/settings", func(c *gin.Context) {
    userID := getUserID(c)
    settings, err := svc.GetSettings(userID)
    if err != nil {
        log.Error("get settings failed",
            zap.String("user_id", userID.String()),
            zap.Error(err),
        )
        respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch settings")
        return
    }
    respondJSON(c, http.StatusOK, convert.ToSettings(*settings))
})
```

- [ ] **Step 4.5: Add logs to `handler/settings.go` — UpdateSettings**

Replace lines 28-55:
```go
api.PUT("/settings", func(c *gin.Context) {
    userID := getUserID(c)
    var req types.PutSettingsJSONBody
    if err := c.ShouldBindJSON(&req); err != nil {
        log.Warn("update settings: invalid request body",
            zap.String("user_id", userID.String()),
            zap.Error(err),
        )
        respondError(c, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
        return
    }
    svcReq := service.UpdateSettingsRequest{
        ModelTier: (*string)(req.ModelTier),
        Locale:    (*string)(req.Locale),
        Theme:     (*string)(req.Theme),
    }
    settings, err := svc.CompleteUpdateSettings(
        c.Request.Context(),
        userID,
        svcReq,
        wsManager.NextSeq,
        func(userID uuid.UUID, update model.UserUpdate) {
            wsUpdate := convert.ToUpdate(update)
            wsManager.PushToUserConnections(userID, wsUpdate)
        },
    )
    if err != nil {
        log.Error("update settings failed",
            zap.String("user_id", userID.String()),
            zap.Error(err),
        )
        respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update settings")
        return
    }
    log.Info("settings updated",
        zap.String("user_id", userID.String()),
    )
    respondJSON(c, http.StatusOK, convert.ToSettings(*settings))
})
```

- [ ] **Step 4.6: Compile check and commit**

Run: `cd /Users/leichujun/go/src/github.com/PineappleBond/eino-demo-dev/server && go build ./...`
Expected: zero errors.

```bash
git add server/internal/handler/template.go server/internal/handler/settings.go
git commit -m "feat: add logging to template and settings handler endpoints"
```

---

### Task 5: Add logging to `handler/user.go` and `handler/chat.go`

**Files:**
- Modify: `server/internal/handler/user.go`
- Modify: `server/internal/handler/chat.go` (already has some logs, check for gaps)

- [ ] **Step 5.1: Add logs to `handler/user.go` — GetMe**

Replace lines 20-36:
```go
api.GET("/users/me", func(c *gin.Context) {
    userID := getUserID(c)
    user, settings, err := svc.GetMe(userID)
    if err != nil {
        log.Error("get me failed",
            zap.String("user_id", userID.String()),
            zap.Error(err),
        )
        respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch user")
        return
    }
    createdAt := user.CreatedAt
    resp := convert.MeResponse{
        ID:       user.ID,
        Settings: convert.ToSettings(*settings),
    }
    if user.Name != "" {
        resp.Name = &user.Name
    }
    resp.CreatedAt = &createdAt
    respondJSON(c, http.StatusOK, resp)
})
```

- [ ] **Step 5.2: Add log to `handler/user.go` — Updates endpoint entry**

Before line 51 (`var updates []model.UserUpdate`), add:
```go
log.Debug("fetch updates",
    zap.String("user_id", userID.String()),
    zap.Int64("last_seq", lastSeq),
)
```

- [ ] **Step 5.3: Compile check and commit**

Run: `cd /Users/leichujun/go/src/github.com/PineappleBond/eino-demo-dev/server && go build ./...`
Expected: zero errors.

```bash
git add server/internal/handler/user.go
git commit -m "feat: add logging to user handler endpoints"
```

---

### Task 6: Add logging to `service/conversation.go`

**Files:**
- Modify: `server/internal/service/conversation.go`

This file already has `log *zap.Logger` (1 existing log at line 186). Add ~12 more.

- [ ] **Step 6.1: Add logs to `ListConversations`**

Replace lines 27-33:
```go
func (s *ConversationService) ListConversations(userID, projectID uuid.UUID) ([]model.Conversation, error) {
    var conversations []model.Conversation
    if err := s.db.Where("user_id = ? AND project_id = ?", userID, projectID).
        Order("updated_at DESC").Find(&conversations).Error; err != nil {
        s.log.Error("list conversations: query failed",
            zap.String("user_id", userID.String()),
            zap.String("project_id", projectID.String()),
            zap.Error(err),
        )
        return nil, err
    }
    return conversations, nil
}
```

- [ ] **Step 6.2: Add logs to `CompleteCreateConversation`**

Add after the project verification failure (line 63):
```go
if err := tx.Where("id = ? AND user_id = ?", projectID, userID).First(&project).Error; err != nil {
    s.log.Error("create conversation: project not found",
        zap.String("user_id", userID.String()),
        zap.String("project_id", projectID.String()),
    )
    return fmt.Errorf("project not found")
}
```

Add after the conversation creation error (line 77):
```go
if err := tx.Create(&conversation).Error; err != nil {
    s.log.Error("create conversation: failed to create conversation record",
        zap.String("user_id", userID.String()),
        zap.String("project_id", projectID.String()),
        zap.Error(err),
    )
    return err
}
```

- [ ] **Step 6.3: Add logs to `CompleteDeleteConversation`**

After the ownership verification (line 481):
```go
if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
    s.log.Error("delete conversation: not found",
        zap.String("user_id", userID.String()),
        zap.String("conv_id", conversationID.String()),
    )
    return fmt.Errorf("get conversation: %w", ErrConversationNotFound)
}
```

Add after the seq assignment (line 486):
```go
seq, err := nextSeq(ctx, userID)
if err != nil {
    s.log.Error("delete conversation: seq assignment failed",
        zap.String("user_id", userID.String()),
        zap.String("conv_id", conversationID.String()),
        zap.Error(err),
    )
    return fmt.Errorf("seq assignment failed: %w", err)
}
```

Add before the pushUpdate at the end (line 518):
```go
s.log.Info("conversation deleted",
    zap.String("user_id", userID.String()),
    zap.String("conv_id", conversationID.String()),
)
pushUpdate(userID, model.UserUpdate{
```

- [ ] **Step 6.4: Add logs to `CompleteRenameConversation`**

After ownership verification (line 298):
```go
if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
    s.log.Error("rename conversation: not found",
        zap.String("user_id", userID.String()),
        zap.String("conv_id", conversationID.String()),
    )
    return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
}
```

Add after the transaction error (line 331):
```go
if err != nil {
    s.log.Error("rename conversation: transaction failed",
        zap.String("user_id", userID.String()),
        zap.String("conv_id", conversationID.String()),
        zap.Error(err),
    )
    return nil, err
}
s.log.Info("conversation renamed",
    zap.String("user_id", userID.String()),
    zap.String("conv_id", conversationID.String()),
    zap.String("title", conv.Title),
)
```

- [ ] **Step 6.5: Add logs to `UpdateStatus`**

After ownership verification (line 365):
```go
if err := s.db.Where("id = ? AND user_id = ?", conversationID, userID).First(&conv).Error; err != nil {
    s.log.Error("update conversation status: not found",
        zap.String("user_id", userID.String()),
        zap.String("conv_id", conversationID.String()),
    )
    return nil, fmt.Errorf("get conversation: %w", ErrConversationNotFound)
}
```

Add after successful update (line 455, before `return &conv`):
```go
s.log.Info("conversation status updated",
    zap.String("user_id", userID.String()),
    zap.String("conv_id", conversationID.String()),
    zap.String("old_status", conv.Status),
    zap.String("new_status", req.Status),
)
```

- [ ] **Step 6.6: Compile check and commit**

Run: `cd /Users/leichujun/go/src/github.com/PineappleBond/eino-demo-dev/server && go build ./...`
Expected: zero errors.

```bash
git add server/internal/service/conversation.go
git commit -m "feat: add logging to conversation service operations"
```

---

### Task 7: Add logging to `service/project.go` and `service/template.go`

**Files:**
- Modify: `server/internal/service/project.go`
- Modify: `server/internal/service/template.go`

- [ ] **Step 7.1: Add logs to `service/project.go` — ListProjects**

Replace lines 26-31:
```go
func (s *ProjectService) ListProjects(userID uuid.UUID) ([]model.Project, error) {
    var projects []model.Project
    if err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&projects).Error; err != nil {
        s.log.Error("list projects: query failed",
            zap.String("user_id", userID.String()),
            zap.Error(err),
        )
        return nil, err
    }
    return projects, nil
}
```

- [ ] **Step 7.2: Add logs to `service/project.go` — GetProject**

Replace lines 35-40:
```go
func (s *ProjectService) GetProject(userID, projectID uuid.UUID) (*model.Project, error) {
    var project model.Project
    if err := s.db.Where("id = ? AND user_id = ?", projectID, userID).First(&project).Error; err != nil {
        s.log.Error("get project: not found",
            zap.String("user_id", userID.String()),
            zap.String("project_id", projectID.String()),
        )
        return nil, fmt.Errorf("project not found")
    }
    return &project, nil
}
```

- [ ] **Step 7.3: Add logs to `service/project.go` — UpdateProject**

Replace lines 44-62:
```go
func (s *ProjectService) UpdateProject(userID, projectID uuid.UUID, name string, config model.JSONMap) (*model.Project, error) {
    updates := map[string]any{}
    if name != "" {
        updates["name"] = name
    }
    if config != nil {
        updates["config"] = config
    }
    if len(updates) == 0 {
        s.log.Warn("update project: no fields to update",
            zap.String("user_id", userID.String()),
            zap.String("project_id", projectID.String()),
        )
        return nil, fmt.Errorf("no fields to update")
    }
    result := s.db.Model(&model.Project{}).Where("id = ? AND user_id = ?", projectID, userID).Updates(updates)
    if result.Error != nil {
        s.log.Error("update project: database error",
            zap.String("user_id", userID.String()),
            zap.String("project_id", projectID.String()),
            zap.Error(result.Error),
        )
        return nil, result.Error
    }
    if result.RowsAffected == 0 {
        s.log.Error("update project: not found",
            zap.String("user_id", userID.String()),
            zap.String("project_id", projectID.String()),
        )
        return nil, fmt.Errorf("project not found")
    }
    s.log.Info("project updated",
        zap.String("user_id", userID.String()),
        zap.String("project_id", projectID.String()),
    )
    return s.GetProject(userID, projectID)
}
```

- [ ] **Step 7.4: Add logs to `service/project.go` — CompleteDeleteProject**

Add after ownership verification (line 88):
```go
if err := s.db.Where("id = ? AND user_id = ?", projectID, userID).First(&project).Error; err != nil {
    s.log.Error("delete project: not found",
        zap.String("user_id", userID.String()),
        zap.String("project_id", projectID.String()),
    )
    return fmt.Errorf("project not found")
}
```

Add after seq assignment (line 93):
```go
seq, err := nextSeq(ctx, userID)
if err != nil {
    s.log.Error("delete project: seq assignment failed",
        zap.String("user_id", userID.String()),
        zap.String("project_id", projectID.String()),
        zap.Error(err),
    )
    return fmt.Errorf("seq assignment failed: %w", err)
}
```

Add before final pushUpdate (line 125):
```go
s.log.Info("project deleted",
    zap.String("user_id", userID.String()),
    zap.String("project_id", projectID.String()),
)
pushUpdate(userID, model.UserUpdate{
```

- [ ] **Step 7.5: Add logs to `service/template.go` — GetTemplate**

Replace lines 32-37:
```go
func (s *TemplateService) GetTemplate(id string) (templates.TemplateDetail, error) {
    t, ok := templates.Get(id)
    if !ok {
        s.log.Warn("get template: not found", zap.String("template_id", id))
        return t, fmt.Errorf("template %q not found", id)
    }
    return t, nil
}
```

- [ ] **Step 7.6: Add logs to `service/template.go` — CompleteCreateProjectFromTemplate**

Add after template not found (line 52):
```go
if !ok {
    s.log.Warn("create project from template: template not found", zap.String("template_id", templateID))
    return nil, fmt.Errorf("template not found")
}
```

Add after seq assignment failure (line 65):
```go
seq, err := nextSeq(ctx, userID)
if err != nil {
    s.log.Error("create project from template: seq assignment failed",
        zap.String("user_id", userID.String()),
        zap.String("template_id", templateID),
        zap.Error(err),
    )
    return nil, fmt.Errorf("seq assignment failed: %w", err)
}
```

Add before final pushUpdate (line 121):
```go
s.log.Info("project created from template",
    zap.String("user_id", userID.String()),
    zap.String("template_id", templateID),
    zap.String("project_id", project.ID.String()),
)
pushUpdate(userID, model.UserUpdate{
```

- [ ] **Step 7.7: Compile check and commit**

Run: `cd /Users/leichujun/go/src/github.com/PineappleBond/eino-demo-dev/server && go build ./...`
Expected: zero errors.

```bash
git add server/internal/service/project.go server/internal/service/template.go
git commit -m "feat: add logging to project and template service operations"
```

---

### Task 8: Add logging to `service/user.go`

**Files:**
- Modify: `server/internal/service/user.go`

- [ ] **Step 8.1: Add logs to `GetMe`**

Replace lines 23-37:
```go
func (s *UserService) GetMe(userID uuid.UUID) (*model.User, *model.Settings, error) {
    var user model.User
    if err := s.db.First(&user, "id = ?", userID).Error; err != nil {
        s.log.Error("get user: not found",
            zap.String("user_id", userID.String()),
            zap.Error(err),
        )
        return nil, nil, err
    }

    var settings model.Settings
    s.db.Where("user_id = ?", userID).FirstOrCreate(&settings, model.Settings{
        UserID:    userID,
        ModelTier: "sonnet",
        Locale:    "en",
        Theme:     "light",
    })

    return &user, &settings, nil
}
```

- [ ] **Step 8.2: Compile check and commit**

Run: `cd /Users/leichujun/go/src/github.com/PineappleBond/eino-demo-dev/server && go build ./...`
Expected: zero errors.

```bash
git add server/internal/service/user.go
git commit -m "feat: add logging to user service"
```

---

### Task 9: Add logging to `eino/runner/rootrunner.go`

**Files:**
- Modify: `server/internal/eino/runner/rootrunner.go`

- [ ] **Step 9.1: Add `Log` field to `RootRunnerConfig`**

After line 141 (`ModelHub skill.ModelHub`), add:
```go
// Log is the structured logger for this runner instance.
Log *zap.Logger
```

Add `"go.uber.org/zap"` to imports.

- [ ] **Step 9.2: Add logging to `NewRootRunner` — model creation**

Replace lines 160-169:
```go
// 1. Create ChatModel from ModelProvider.
mc := cfg.ModelProvider.GetModel(cfg.ModelTier)
chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
    BaseURL: mc.BaseURL,
    APIKey:  mc.APIKey,
    Model:   mc.Model,
})
if err != nil {
    if cfg.Log != nil {
        cfg.Log.Error("root runner: failed to create chat model",
            zap.String("tier", cfg.ModelTier),
            zap.Error(err),
        )
    }
    return nil, err
}
```

- [ ] **Step 9.3: Add logging to `NewRootRunner` — prompt rendering**

Replace lines 176-179:
```go
// 2. Render instruction — template includes SystemPrompt via {{ .SystemPrompt }}.
instruction, err := renderRootPrompt(cfg.Tools, cfg.SubAgents, cfg.SystemPrompt, cfg.WorkspaceDir, cfg.IsGitRepo)
if err != nil {
    if cfg.Log != nil {
        cfg.Log.Error("root runner: failed to render prompt",
            zap.String("conv", cfg.ConversationID.String()),
            zap.Error(err),
        )
    }
    return nil, err
}
```

- [ ] **Step 9.4: Add summary log at end of `NewRootRunner`**

Before the final `return` (line 282):
```go
if cfg.Log != nil {
    cfg.Log.Info("root runner: created",
        zap.String("conv", cfg.ConversationID.String()),
        zap.String("model_tier", cfg.ModelTier),
        zap.Int("tool_count", len(cfg.Tools)),
        zap.Int("sub_agent_count", len(cfg.SubAgents)),
        zap.Bool("summarization", cfg.SummarizationCallback != nil),
        zap.Bool("skills", cfg.SkillBackend != nil),
        zap.Bool("permissions", cfg.PermissionMW != nil),
    )
}
return &RootRunner{runner: runner}, nil
```

- [ ] **Step 9.5: Wire `Log` in `service/chat.go` where RootRunnerConfig is constructed**

Find where `runner.RootRunnerConfig{}` is constructed in `service/chat.go` and add the `Log` field.

Search for the construction:
```go
// Find the RootRunnerConfig literal and add:
Log: s.log,
```

- [ ] **Step 9.6: Compile check and commit**

Run: `cd /Users/leichujun/go/src/github.com/PineappleBond/eino-demo-dev/server && go build ./...`
Expected: zero errors.

```bash
git add server/internal/eino/runner/rootrunner.go server/internal/service/chat.go
git commit -m "feat: add logging to root runner creation"
```

---

### Task 10: Full build and smoke test

**Files:** None (verification only)

- [ ] **Step 10.1: Full build**

Run: `cd /Users/leichujun/go/src/github.com/PineappleBond/eino-demo-dev/server && go build ./...`
Expected: zero errors, zero warnings.

- [ ] **Step 10.2: Run `go vet`**

Run: `cd /Users/leichujun/go/src/github.com/PineappleBond/eino-demo-dev/server && go vet ./...`
Expected: zero warnings.

- [ ] **Step 10.3: Run existing tests**

Run: `cd /Users/leichujun/go/src/github.com/PineappleBond/eino-demo-dev/server && go test ./...`
Expected: all existing tests pass.

- [ ] **Step 10.4: Verify log format**

Start the server: `go run ./server/cmd/server/main.go` (with dependencies running via docker-compose)
Hit any endpoint: `curl -H "Authorization: Bearer test-token" http://localhost:8080/api/v1/templates`
Verify: stdout shows structured JSON logs with the new log messages.

- [ ] **Step 10.5: Final commit (if any changes)**

```bash
git add -A
git commit -m "chore: verify logging compilation and smoke test"
```

---

## Self-Review Checklist

**1. Spec coverage:**
- Handler layer: conversation (Task 2), project (Task 3), template (Task 4), settings (Task 4), user (Task 5) — all covered
- Service layer: conversation (Task 6), project (Task 7), template (Task 7), user (Task 8) — all covered
- Eino layer: rootrunner (Task 9) — covered
- DI wiring (Task 1) — covered
- Compile/test (Task 10) — covered

**2. Placeholder scan:** No TBD, TODO, or "similar to" references found. All code blocks contain actual implementation code.

**3. Type consistency:**
- All `log` parameters are `*zap.Logger` — matches existing DI provision
- All zap fields use correct types: `zap.String()`, `zap.Error()`, `zap.Int()`, `zap.Int64()`, `zap.Bool()`
- `user_id` consistently uses `.String()` on UUID
- `RootRunnerConfig.Log` is `*zap.Logger`, nil-checked before use
