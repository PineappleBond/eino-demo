# Frontend Layout & UX Design

**Date**: 2026-04-10
**Status**: Approved
**Prototype**: [prototype.html](prototype.html) — 可交互 HTML 原型，浏览器直接打开

## Overview

Ant Design v6 + Next.js frontend for Eino educational demo. Three core states: home (template grid), project details, and chat with multi-conversation tabs.

## Global Layout

```
┌─────────────────────────────────────────────────────────────────┐
│  TopBar (52px): [📦 Project-A ▼]  ...  [⚙ Project 🌙 🌐 中]    │
├────────────┬──────────────────────────────────────────┬──────────┤
│ Left Sider │  Tabs 标签区                             │ ConvInfo │
│ (240px)    │  [💬 Conv 1] [💬 Conv 2] [+]             │ (300px)  │
│            ├──────────────────────────────────────────┤ 可折叠    │
│ ▶ Convers  │  聊天记录 (flex:1)                       │          │
│   💬 C-1   │  输入框 + 操作栏                          │          │
│   💬 C-2   ├──────────────────────────────────────────┤          │
│            │  [无活跃对话时显示提示]                    │          │
│ ⚙ Settings │                                          │          │
│ 📋 Templates│                                         │          │
└────────────┴──────────────────────────────────────────┴──────────┘
```

- **TopBar ⚙** → 覆盖式 Project Info Drawer（420px，右侧滑入，可编辑/删除）
- **聊天区 ℹ 按钮** → 展开/折叠 ConvInfo 面板（300px，聊天区右侧内嵌）
- 两个面板完全独立

## Route States

| Route | TopBar | Sider | Content | ConvInfo Panel |
|-------|--------|-------|---------|----------------|
| `/` (home) | No project dropdown | Hidden | Template grid cards | Hidden |
| `/project/[id]` | Project dropdown | Auto-expanded | Project details + Agent tree | Hidden |
| `/project/[id]/chat` | Project dropdown | Conversation list | Tabbed chat | Toggleable (ℹ button) |
| `/project/[id]/chat?branch=[convId]` | Project dropdown | Conversation list | Branch chat + breadcrumb | Toggleable |

## Chat Area

- Messages: flex-grow, scrollable, linear message flow
- Branch chats: inline collapsible cards with link to branch page (reuses chat Page component)
- Right-click messages: Copy | Reply | Create Branch Conversation
- Right-click members: @mention (inserts into input)
- Input area: textarea + Stop/Send buttons, auto-save draft to IndexedDB

## Left Sider Actions

| Action | Interaction |
|--------|-------------|
| Rename conversation | Double-click name → inline edit |
| Delete conversation | Right-click → Popconfirm |
| Archive conversation | Right-click → moves to "Archived" section |
| Manual context compression | Right-click → triggers `conversation.compacting` event |

## TopBar Settings Button

Click → right Drawer slides in with:
- Project info (editable)
- Delete Project (with confirmation)
- Settings: model tier (haiku/sonnet/opus), locale (en/zh), theme (light/dark)

## Branch Conversations

- URL: `/project/[id]/chat?branch=[convId]`
- Breadcrumb: `← 返回上级 · Supervisor → Coder`
- Same Page component, same UX as main chat
- Unlimited nesting via query params

## Responsive Breakpoints

| Breakpoint | Behavior |
|------------|----------|
| ≥ 1024px | Full layout: Sider 240px + Content + Right Drawer 320px |
| 768px–1023px | Sider becomes Drawer (triggered by toggle button) |
| < 768px | Sider hidden, TopBar simplified, chat takes full width |
