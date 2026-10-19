# AGENT.md — Workspace Operating Rules

## Project Context
- **Workspace**: apw-agent
- **Detected Stack**: Go

## Core Guidelines
1. Always inspect existing symbols and tests before proposing changes.
2. Isolate mutations inside Git worktrees and pass Reflex verification before applying.
3. Require Human-in-the-Loop confirmation before executing destructive or external system commands.
