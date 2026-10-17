# apw — Apiwat Agent

ลำดับงานปัจจุบัน: **Core CLI → ทดสอบ provider จริงและเตรียม release → Desktop ตามภาพอ้างอิงเป็นเฟสสุดท้าย** ดู [roadmap](docs/roadmap.md).

**ระบบปฏิบัติการ AI สำหรับนักพัฒนาซอฟต์แวร์ (Ground-Up AI Developer OS)** ที่สร้างขึ้นใหม่ตั้งแต่ศูนย์ด้วยภาษา Go โดยไม่พึ่งพาเฟรมเวิร์กสำเร็จรูปที่เทอะทะ ผสานสมองสองระบบ (**System 1: Reflex** และ **System 2: Architect**), ศัลยแพทย์โค้ดความแม่นยำสูง (**Surgeon**), บริการเบื้องหลัง 24/7 (**`apwd` Daemon**) และระบบความจำ/ทักษะอัตโนมัติเข้าเป็นเนื้อเดียวกัน


คู่มือขั้นตอนทำงานและข้อจำกัดปัจจุบัน: [Task lifecycle and operations](docs/task-lifecycle.md).


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
ต้องใช้ Go 1.27 ขึ้นไป (`go.mod` ระบุ `go 1.27.0`) จากนั้น clone repository และเข้าโฟลเดอร์ก่อนติดตั้ง:
```bash
git clone https://github.com/apw-dev/apw-agent.git
cd apw-agent
go install ./cmd/...
```
หรือใช้ชุด portable (`apw.exe` + `apwd.exe`) ตาม [docs/release.md](docs/release.md) โดยวางไฟล์ทั้งสองไว้ในโฟลเดอร์เดียวกัน

### 1) โหมด Interactive CLI (`apw > _`)
พิมพ์ `apw` จากโฟลเดอร์โปรเจกต์ใดก็ได้ ระบบจะปลุก `apwd` เบื้องหลังอัตโนมัติและเข้าสู่หน้าต่างโต้ตอบทันที:
```text
  ▄▀█ █▀█ █░█░█
  █▀█ █▀▀ ▀▄▀▄▀   v0.1.0-alpha (Apiwat Agent)
 ───────────────────────────────────────────────────────
  Brain 1 (Reflex) : Reflex Engine [Available]
  Brain 2 (Deep)   : Architect     [Standby]
  Daemon (apwd)    : Stopped (PID: 0) | 17 Skills Loaded
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
apw review "ตรวจโค้ดและรายงานปัญหา"       # อ่าน source เพื่อ review
apw preview "เตรียมการเปลี่ยนแปลง"         # สร้าง diff ใน worktree
apw trust                               # อนุญาต build/test สำหรับ repo นี้ (จำเป็นก่อน run และ verify)
apw run "อธิบายหรือสั่งงานที่ต้องการ"       # ต้อง trust: เตรียมและ verify จากนั้นรอ apply
apw plan "วางแผนงานก่อนแก้ไขจริง"        # สร้างแผน ExecutionPlan (.apw/plans/<id>.json)
apw verify .                           # ต้อง trust: รัน Reflex ตาม pipeline ของโปรเจกต์ (ดูหัวข้อภาษาที่รองรับ)
apw cleanup --dry-run                  # ดูว่า retention policy จะลบ task เก่าอะไรบ้าง (ไม่เปลี่ยนอะไร)
apw symbols internal/surgeon/surgeon.go # สกัดโครงสร้าง AST Symbols ในไฟล์
apw daemon status                      # ตรวจสอบสถานะบริการเบื้องหลัง apwd
```
`review`, `preview` และ `plan` ใช้ได้โดยไม่ต้อง trust เพราะไม่รันโค้ดของ repository ส่วน `run` และ `verify` จะรัน build/test จึงถูกปฏิเสธจนกว่าจะ `apw trust` (Git worktree แยกไฟล์ออกจาก workspace แต่ไม่ได้ sandbox โค้ดที่ถูกรัน)

### ภาษาและเครื่องมือที่รองรับ
APW ตรวจชนิดโปรเจกต์จากไฟล์ที่ root แล้วเลือก pipeline ตรวจสอบ (Reflex) ให้เอง ทุกขั้นรันใน worktree ที่แยกออกมา และต้อง `apw trust` ก่อน:

| โปรเจกต์ (ไฟล์ที่ใช้ตรวจ) | Pipeline |
| --- | --- |
| Go (`go.mod`) | `gofmt` → `go vet` → `go build` → `go test` |
| Node.js / TypeScript (`package.json`) | install → `format:check` → `lint` → typecheck → `build` → `test` (เฉพาะ script ที่มีจริง) |
| Rust (`Cargo.toml`) | `cargo fmt --check` → `cargo build` → `cargo test` |
| Python (`pyproject.toml`, `requirements.txt`) | `ruff check` (ถ้าติดตั้ง) → `pytest` (ถ้ามี tests) |
| HTML/CSS ล้วน (`index.html`) | ตรวจว่า `src`/`href` ที่ชี้ไฟล์ในเครื่องมีอยู่จริง |

**Node.js:** เลือก package manager จาก `packageManager` ใน `package.json` หรือจาก lockfile (`bun.lock` → bun, `pnpm-lock.yaml` → pnpm, `package-lock.json` → npm, `yarn.lock` → yarn)
- **install:** มี lockfile จะติดตั้งตาม lock เท่านั้น (`npm ci`, `pnpm install --frozen-lockfile`, `bun install --frozen-lockfile`) ไม่มี lockfile จะไม่สร้าง lockfile ใหม่ และรอบซ่อมถัดไปจะข้าม install ถ้า `package.json` กับ lockfile ไม่เปลี่ยน
- **typecheck:** ใช้ script `typecheck` ถ้ามี ไม่มีแต่มี `tsconfig.json` กับ `typescript` จะรัน `tsc --noEmit` (ข้ามถ้า script `build` รัน `tsc` อยู่แล้ว เช่น template ของ Vite)
- **test:** รันด้วย `CI=true` เพื่อให้ Vitest และ Jest รันครั้งเดียว ไม่ค้างใน watch mode
- **ข้อกำหนด:** `node_modules/` และไฟล์ output ของ build (`dist/`) ต้องอยู่ใน `.gitignore` ไม่อย่างนั้น APW จะนับเป็นไฟล์ที่เปลี่ยนโดยไม่ได้วางแผน
- **pnpm 10 ขึ้นไป:** pnpm บล็อก build script ของ dependency (เช่น esbuild ที่ Vite ใช้) และ install จะล้มด้วย `ERR_PNPM_IGNORED_BUILDS` ให้รัน `pnpm approve-builds esbuild` ครั้งเดียว แล้ว commit `pnpm-workspace.yaml` ที่ได้ (`allowBuilds: esbuild: true`)
- **ไม่ให้ test runner ไปรันสำเนาใน `.worktrees/`:** APW เก็บ worktree ของแต่ละงานไว้ใน `.worktrees/` ของโปรเจกต์ ซึ่ง Vitest, Jest และ `eslint .` จะค้นเจอด้วย ให้ exclude โฟลเดอร์นี้ เช่นใน `vite.config.ts`: `test: { exclude: [...configDefaults.exclude, '.worktrees/**'] }` (import `configDefaults` จาก `vitest/config`) และลบงานเก่าด้วย `apw cleanup`
- **Path บน Windows:** worktree อยู่ลึกกว่าโฟลเดอร์โปรเจกต์ราว 50 ตัวอักษร และ dependency ของ pnpm ซ้อนลึกมาก ควรวางโปรเจกต์ไว้ใน path สั้น (เช่น `D:\src\app`) ไม่อย่างนั้นอาจติดข้อจำกัด 260 ตัวอักษร ซึ่ง APW จะแจ้งไว้ใน error

**ภาษาอื่นหรือ framework ที่เขียนเอง:** กำหนด pipeline เองใน `apw.json` ได้ ค่านี้จะแทน pipeline ที่ตรวจเจอ แต่ละขั้นเป็นคำสั่งแบบ argument vector (ไม่ผ่าน shell) และชื่อขั้นใช้ใน acceptance check ได้ เช่น `{"kind":"verification","value":"test"}`:

```json
"verify": {
  "stages": [
    { "name": "lint", "run": ["pnpm", "lint"] },
    { "name": "typecheck", "run": ["pnpm", "exec", "tsc", "--noEmit"] },
    { "name": "test", "run": ["pnpm", "test"], "env": { "CI": "true" }, "timeout_sec": 600 }
  ]
}
```
ถ้าตรวจชนิดโปรเจกต์ไม่ได้และไม่ได้กำหนด `verify.stages` ไว้ `apw run` และ `apw verify` จะไม่ผ่าน พร้อมบอกให้กำหนด pipeline (APW ไม่ถือว่างานผ่านถ้าไม่มีอะไรตรวจ)

### การสลับโมเดลและโควตา

การค้นพบโมเดลทำให้เลือกใช้ได้ แต่ไม่ได้เพิ่มทุกโมเดลเป็น fallback อัตโนมัติ
ค่าเริ่มต้นของการสร้าง patch คือ `roles.reflex` ตามด้วย `roles.architect`
ถ้าต้องการตัวสำรองเพิ่มเติม ให้ระบุชื่อ provider จาก `providers` ใน config ตามลำดับ:

```json
"failover": {
  "reflex": [],
  "architect": ["claude-sub:claude-haiku-5-5"],
  "directive_timeout_sec": 90
}
```

ตัวอย่างนี้ต้องมี `claude-sub:claude-haiku-5-5` อยู่ใน `providers` ก่อน
ลำดับคือ Reflex ที่เลือก → Reflex สำรองที่ระบุ → Architect ที่เลือก → Architect สำรองที่ระบุ
หากเลือก Gemini Flash ระบบจะไม่ลอง Argon เอง เว้นแต่เลือกเป็น role หรือระบุเป็น fallback
เวลารอ patch ต่อครั้งมีค่าเริ่มต้น 90 วินาที แยกจากเวลารวมของงานและ timeout ภายใน provider
เมื่อ fallback สำเร็จ จะใช้ตัวนั้นก่อนในขั้นถัดไปของงานเดียวกัน

AGY แบ่ง cooldown เป็นสามกลุ่ม: Gemini Flash/Pro, Gemini Argon และ Claude/GPT-OSS
เมื่อพบ quota error จะพักเฉพาะกลุ่มนั้นตาม `Resets in ...` ที่บริการแจ้ง
หากไม่แจ้งเวลาจะพัก 5 นาที ส่วน timeout จะพักเฉพาะโมเดล 5 นาที และไม่ถือว่าโควตาทั้งกลุ่มหมด
การเชื่อมต่อ Claude CLI, Codex CLI และ API ใช้ cooldown แยกจาก AGY

งานใน Git worktree บันทึกแผน ขั้นที่สำเร็จ และ cooldown หลังแต่ละขั้นเพื่อใช้ `apw resume`
เมื่อ verification หรือนำผลกลับไม่สำเร็จ จะเก็บ worktree ไว้
checkpoint ต้องตรงกับ task, plan และ snapshot manifest ของ worktree; งานเก่าที่ไม่มี manifest จะหยุดเพื่อป้องกันการเขียนทับ ให้เก็บ worktree เดิมไว้ตรวจและนำ diff มาทบทวนก่อนเริ่มงานใหม่

## License

MIT, see [LICENSE](LICENSE).
