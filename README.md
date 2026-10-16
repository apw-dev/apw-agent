# apw — Apiwat Agent

**ระบบปฏิบัติการ AI สำหรับนักพัฒนาซอฟต์แวร์ (Ground-Up AI Developer OS)** ที่สร้างขึ้นใหม่ตั้งแต่ศูนย์ด้วยภาษา Go โดยไม่พึ่งพาเฟรมเวิร์กสำเร็จรูปที่เทอะทะ ผสานสมองสองระบบ (**System 1: Reflex** และ **System 2: Architect**), ศัลยแพทย์โค้ดความแม่นยำสูง (**Surgeon**), บริการเบื้องหลัง 24/7 (**`apwd` Daemon**) และระบบความจำ/ทักษะอัตโนมัติเข้าเป็นเนื้อเดียวกัน

---

## 1. วิสัยทัศน์และปรัชญาการออกแบบ (Core Philosophy — 100% Native APW)

| แกนปรัชญาของ APW | แก่นหลักที่สร้างขึ้นใน `apw` | โมดูลในระบบ |
| --- | --- | --- |
| **1. System 1 — Fast Reflex Principle** | ตอบสนองระดับเสี้ยววินาที (<10ms routing), บังคับโครงสร้าง Type-safe, วิเคราะห์โค้ดผ่าน AST และตรวจจับข้อผิดพลาดอัตโนมัติ | `internal/reflex` |
| **2. System 2 — Spec-First Architecture** | วางแผนสถาปัตยกรรมเป็นขั้นตอน (`ExecutionPlan` / `plan.json`) และตรวจสอบขอบเขตความปลอดภัยก่อนอนุญาตให้แตะไฟล์โค้ด | `internal/architect` |
| **3. Precision Code Surgery** | ผ่าตัดแก้ไขไฟล์เฉพาะจุด (Read-Before-Write + SHA-256 Base Hash + Surgical Diff) พร้อมรัน Compiler/Test วนลูปอัตโนมัติ | `internal/surgeon` |
| **4. 24/7 Autonomous Daemon & Skills** | รันเบื้องหลังตลอดเวลา (`apwd`), จัดการคิวงาน, ความจำระยะยาว (SQLite) และระบบขยายทักษะ (Skill Hub) | `internal/daemon`, `internal/store`, `skills/` |
| **5. Headless Core & Multi-Client Protocol** | แยกส่วนสมองออกจากหน้าจอ เชื่อมต่อได้ทั้ง Interactive CLI (`apw`), Desktop UI (`apw desktop`), Editor (RPC) และ Chat (`Telegram`) | `pkg/protocol`, `internal/desktop`, `internal/channels` |

---

## 2. สถาปัตยกรรมระบบ (System Architecture)

```text
┌────────────────────────────────────────────────────────────────────────────┐
│                        Surface Layer (Multi-Client)                        │
│     [ $ apw CLI ]      [ $ apw desktop UI ]       [ Telegram / Chat ]      │
└─────────────────────────────────────┬──────────────────────────────────────┘
                                      │ JSON-RPC 2.0 / HTTP / Local Socket
┌─────────────────────────────────────▼──────────────────────────────────────┐
│                       APW Core Daemon ($ apwd - 24/7)                      │
│                                                                            │
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │                    1. Reflex Engine (System 1)                       │  │
│  │  • Intent Router (<10ms)         • AST Symbol & Syntax Inspector     │  │
│  │  • Fast Constrained LLM (Local)  • Deterministic Type-Safe Schema    │  │
│  └──────────────────┬───────────────────────────────┬───────────────────┘  │
│                     │ (Fast Path: 75% ของงาน)        │ (Slow Path: 25% ของงาน)│
│                     │                               ▼                      │
│                     │              ┌────────────────────────────────────┐  │
│                     │              │   2. Architect Engine (System 2)   │  │
│                     │              │  • Deep Reasoning LLM              │  │
│                     │              │  • Dependency Graph Analyzer       │  │
│                     │              │  • Task Decomposition (plan.json)  │  │
│                     │              └────────────────┬───────────────────┘  │
│                     │                               │ (ส่ง Sub-tasks กลับ)  │
│                     ▼                               ▼                      │
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │                3. Surgeon & Verification Sandbox                     │  │
│  │  • Surgical Block Patcher        • Auto Compiler/Linter Gatekeeper   │  │
│  │  • Git Worktree Isolation        • Self-Healing Retry Loop (Max 3)   │  │
│  └──────────────────────────────────┬───────────────────────────────────┘  │
│                                     │                                      │
│  ┌──────────────────────────────────▼───────────────────────────────────┐  │
│  │                4. Memory & Self-Evolving Skill Hub                   │  │
│  │  • Project Context Graph         • Auto Skill Extractor              │  │
│  │  • Episodic Error Memory (SQLite)• Sandboxed Script Runner           │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. แผนผังโครงสร้างโมดูลในโปรเจกต์ (Repository Topology)

```text
apw-agent/
├── cmd/
│   ├── apw/                        # Surface CLI & Interactive Shell (apw > _)
│   └── apwd/                       # 24/7 Background Core Daemon
├── internal/
│   ├── core/                       # Core Domain Models (Task, Request, Response, Capabilities)
│   ├── config/                     # ระบบตั้งค่า apw.json + Validation
│   ├── reflex/                     # [Brain 1] Verification Pipeline (fmt/vet/build/test) + Self-Healing Loop
│   ├── architect/                  # [Brain 2] Spec-First Planner + DAG Subtask Decomposition + Plan Validator
│   ├── surgeon/                    # [Surgeon] Read-Before-Write Snapshot, SHA-256 Drift Guard, AST Symbol Parser
│   ├── executor/                   # Triad Orchestrator (เชื่อม Architect -> Surgeon -> Reflex ครบวงจร)
│   ├── daemon/                     # Auto-start & Lifecycle Manager สำหรับ apwd + Skill Discovery
│   ├── desktop/                    # Native Desktop Web UI Server (apw desktop / apw ui)
│   ├── channels/
│   │   └── telegram/               # Messaging Gateway (Long-polling, Allowlist, Task Commands)
│   ├── provider/
│   │   ├── api/                    # Deep Reasoning Cloud API Provider
│   │   ├── local/                  # Fast Local LLM Provider (OpenAI-compatible endpoint)
│   │   ├── mock/                   # Deterministic Mock Provider สำหรับ Test & CI
│   │   └── factory/                # Provider Router & Built-in APW Engine
│   └── store/                      # SQLite Persistent Memory (Sessions, Tasks, Inbox Dedup, Episodic Logs)
└── docs/                           # เอกสารสถาปัตยกรรม, ADR และแผนพัฒนา
```

---

## 4. การเริ่มต้นใช้งาน (Quick Start)

### ติดตั้งไบนารี (`apw` และ `apwd`)
```bash
go install ./cmd/...
```

### 1) โหมด Interactive CLI (`apw > _`)
พิมพ์ `apw` จากโฟลเดอร์โปรเจกต์ใดก็ได้ ระบบจะปลุก `apwd` เบื้องหลังอัตโนมัติและเข้าสู่หน้าต่างโต้ตอบทันที:
```text
  ▄▀█ █▀█ █░█░█
  █▀█ █▀▀ ▀▄▀▄▀   v0.1.0-alpha (Apiwat Agent)
 ───────────────────────────────────────────────────────
  Brain 1 (Reflex) : Reflex Engine [Active - 1ms]
  Brain 2 (Deep)   : Architect     [Standby]
  Daemon (apwd)    : Running (PID: 48760) | 17 Skills Loaded
 ───────────────────────────────────────────────────────
  Workspace        : D:/APW_Project/apw-agent (Go)

apw > _
```

### 2) โหมดหน้าต่าง Desktop UI (`apw desktop`)
```bash
apw desktop
```

### 3) คำสั่งบรรทัดคำสั่งโดยตรง (Direct CLI Commands)
```bash
apw run "อธิบายหรือสั่งงานที่ต้องการ"      # รันวงจร Triad (Architect -> Surgeon -> Reflex)
apw plan "วางแผนงานก่อนแก้ไขจริง"        # สร้างแผน ExecutionPlan (.apw/plans/<id>.json)
apw verify .                           # รัน Reflex ตรวจสอบโค้ด (fmt -> vet -> build -> test)
apw symbols internal/surgeon/surgeon.go # สกัดโครงสร้าง AST Symbols ในไฟล์
apw daemon status                      # ตรวจสอบสถานะบริการเบื้องหลัง apwd
```
