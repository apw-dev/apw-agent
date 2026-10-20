# apw — Apiwat Agent

ลำดับงานปัจจุบัน: **Core CLI → ทดสอบ provider จริงและเตรียม release → Native Windows Desktop (`APW.exe` + Svelte 5)** ดู [roadmap](docs/roadmap.md).

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
| **5. Headless Core & Multi-Client Protocol** | แยกส่วนสมองออกจากหน้าจอ เชื่อมต่อได้ทั้ง Interactive CLI (`apw`), Native Desktop (`APW.exe` / `apw desktop`), Editor (ACP / JSON-RPC) และ Chat (`Telegram`) | `internal/acp`, `internal/desktop`, `cmd/apw-desktop`, `internal/channels`, `internal/transport` |

---

## 2. สถาปัตยกรรมระบบ (System Architecture)

```text
┌────────────────────────────────────────────────────────────────────────────┐
│                        Surface Layer (Multi-Client)                        │
│     [ $ apw CLI ]   [ APW.exe / apw desktop ]    [ Telegram / Chat ]       │
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
│   ├── apwd/                       # 24/7 Background Core Daemon
│   └── apw-desktop/                # Native Windows Desktop Shell (APW.exe via Wails v2)
├── internal/
│   ├── core/                       # Core Domain Models (Task, Request, Response, Capabilities)
│   ├── config/                     # ระบบตั้งค่า apw.json + Validation
│   ├── reflex/                     # [Brain 1] Verification Pipeline (fmt/vet/build/test) + Self-Healing Loop
│   ├── architect/                  # [Brain 2] Spec-First Planner + DAG Subtask Decomposition + Plan Validator
│   ├── surgeon/                    # [Surgeon] Read-Before-Write Snapshot, SHA-256 Drift Guard, AST Symbol Parser
│   ├── executor/                   # Triad Orchestrator (เชื่อม Architect -> Surgeon -> Reflex ครบวงจร)
│   ├── daemon/                     # Auto-start & Lifecycle Manager สำหรับ apwd + Skill Discovery
│   ├── desktop/                    # Desktop Backend & Embedded Svelte 5 Frontend (internal/desktop/web/)
│   ├── acp/                        # Agent Client Protocol (ACP) stdio server
│   ├── eval/                       # Benchmarking & Hidden Test Fixture Runner (apw eval)
│   ├── retention/                  # นโยบายล้างงานเก่าและ Prune tasks (apw cleanup)
│   ├── workspace/                  # Exclusive & shared workspace file leases (lock)
│   ├── sandbox/                    # Process isolation (Windows Job Object restricted & Docker)
│   ├── browser/                    # Headless browser runner สำหรับตรวจสอบหน้าเว็บ
│   ├── research/                   # Safe documentation fetcher & Web Search ก่อนวางแผน
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

### ติดตั้งไบนารี (`apw`, `apwd`, และ `APW.exe`)
ต้องใช้ Go 1.27 ขึ้นไป (`go.mod` ระบุ `go 1.27.0`) จากนั้น clone repository และเข้าโฟลเดอร์ก่อนติดตั้ง:
```bash
git clone https://github.com/apiwat/apw-agent.git
cd apw-agent
go install ./cmd/...
```
หรือใช้ตัวติดตั้ง NSIS (`APW-Setup-<version>.exe`) หรือชุด portable (`APW.exe` + `apw.exe` + `apwd.exe`) ตาม [docs/release.md](docs/release.md) โดยวางไฟล์ทั้งหมดไว้ในโฟลเดอร์เดียวกัน

### 1) โหมด Interactive CLI (`apw > _`)
พิมพ์ `apw` จากโฟลเดอร์โปรเจกต์ใดก็ได้ ระบบจะปลุก `apwd` เบื้องหลังอัตโนมัติและเข้าสู่หน้าต่างโต้ตอบทันที:
```text
  ▄▀█ █▀█ █░█░█
  █▀█ █▀▀ ▀▄▀▄▀   v0.1.0-alpha (Apiwat Agent)
 ───────────────────────────────────────────────────────
  Brain 1 (Reflex) : Reflex Engine [Available]
  Brain 2 (Deep)   : Architect     [Standby]
  Daemon (apwd)    : Running (PID: 14820) | 17 Skills Loaded
 ───────────────────────────────────────────────────────
  Workspace        : D:/APW_Project/apw-agent (Go)

apw > _
```

### 2) โหมดหน้าต่าง Native Desktop UI (`APW.exe` / `apw desktop`)
```bash
apw desktop
```
บน Windows หากมีไฟล์ `APW.exe` อยู่ในโฟลเดอร์เดียวกับ `apw.exe` คำสั่ง `apw desktop` จะเปิดหน้าต่าง Native Windows (`APW.exe` ที่สร้างด้วย Wails v2 + Svelte 5 แบบ in-process โดยไม่เปิดพอร์ต localhost) ให้โดยอัตโนมัติ หรือสามารถดับเบิลคลิกเปิด `APW.exe` โดยตรงก็ได้ (หากไม่มี `APW.exe` จะ fallback ไปเปิดผ่าน Edge `--app`)

### 3) คำสั่งบรรทัดคำสั่งโดยตรง (Direct CLI Commands)
```bash
apw run "อธิบายหรือสั่งงานที่ต้องการ"       # ต้อง trust: เตรียมและ verify จากนั้นรอ apply
apw apply <task-id>                     # นำ patch ที่ผ่านการ verify แล้วไป apply ลง workspace จริง
apw review "ตรวจโค้ดและรายงานปัญหา"       # อ่าน source เพื่อ review วิเคราะห์หาจุดผิดพลาด
apw preview "เตรียมการเปลี่ยนแปลง"         # สร้าง diff ใน worktree โดยไม่รันโค้ด repository
apw plan "วางแผนงานก่อนแก้ไขจริง"        # สร้างแผน ExecutionPlan (.apw/plans/<id>.json)
apw tasks | apw task <task-id>          # ดูรายการงานทั้งหมด หรือตรวจสอบรายงาน JSON ของงานนั้น
apw diff <task-id>                      # แสดง source diff ของ task ที่เตรียมไว้
apw watch <task-id>                     # Reattach เกาะติดความคืบหน้าของ task ที่กำลังรัน
apw resume [task-id]                    # สานต่องานเดิมจาก checkpoint
apw cancel <task-id>                    # ยกเลิกงานและหยุด subprocess tree ทันที
apw trust                               # อนุญาต build/test สำหรับ repo นี้ (จำเป็นก่อน run และ verify)
apw verify .                           # ต้อง trust: รัน Reflex ตาม pipeline ของโปรเจกต์ (ดูหัวข้อภาษาที่รองรับ)
apw cleanup [--dry-run]                 # ล้างงานเก่าตาม retention (ถ้าไม่ใส่ --dry-run ต้องสั่ง apw daemon stop ก่อน)
apw recover <legacy-task-id>            # Export กู้คืนงานเก่าที่ค้างใน worktree แบบปลอดภัยสำหรับตรวจสอบ
apw model [status|refresh|set ...]      # ตรวจสอบหรือสลับโมเดล AI ของ Architect / Reflex
apw auth                                # จัดการการเชื่อมต่อโมเดล (AGY, Claude CLI, Codex, Local)
apw skills                              # ดูรายการทักษะ (Skills) ที่โหลดอยู่ในระบบ
apw symbols internal/surgeon/surgeon.go # สกัดโครงสร้าง AST Symbols ในไฟล์
apw daemon [status|start|stop]          # ตรวจสอบหรือจัดการบริการเบื้องหลัง apwd
apw version                             # แสดงข้อมูลเวอร์ชันและ architecture
```
`review`, `preview` และ `plan` ใช้ได้โดยไม่ต้อง trust เพราะไม่รันโค้ดของ repository ส่วน `run` และ `verify` จะรัน build/test จึงถูกปฏิเสธจนกว่าจะ `apw trust` (Git worktree แยกไฟล์ออกจาก workspace แต่ไม่ได้ sandbox โค้ดที่ถูกรัน เว้นแต่เปิด `sandbox`)
*หมายเหตุสำหรับการทำความสะอาด (`apw cleanup`):* การรันแบบตัดทิ้งจริง (ไม่ใส่ `--dry-run`) ต้องหยุด daemon ก่อนด้วย `apw daemon stop` เพื่อปล่อยสิทธิ์ service lease มิฉะนั้นคำสั่งจะปฏิเสธการลบงานเก่าเพื่อป้องกัน race condition กับ daemon ที่กำลังทำงานอยู่

```bash
apw run --auto "..."        # verify ผ่านแล้ว apply ให้ทันที ไม่ต้องสั่ง apply เอง
apw run --no-verify "..."   # ไม่รัน install/build/test (ไม่ต้อง trust) ตรวจเฉพาะ check ระดับไฟล์ แล้วรอ apply
```

**โหมดใน CLI แบบ interactive:** พิมพ์ `/mode` หรือกด shift+tab เพื่อเปลี่ยนว่าคำสั่งที่พิมพ์จะทำอะไร (ค่าเริ่มต้นตั้งได้ใน `apw.json` ที่ `"approval"`):
- `ask` (ค่าเริ่มต้น): วางแผน แก้ และ verify เสร็จแล้วถาม `Apply ... [y/N]` ก่อนนำผลไปใส่ในโปรเจกต์
- `auto`: verify ผ่านแล้ว apply ให้เลยโดยไม่ถาม
- `plan`: วางแผนอย่างเดียว ไม่แก้และไม่รันอะไร
- `unverified`: แก้โดยไม่รัน install/build/test (ไม่ต้อง trust) ตรวจเฉพาะ check ระดับไฟล์ แล้วถามก่อน apply รายงานจะเขียนว่า UNVERIFIED ชัดเจน

**Trust ตอนเข้า CLI:** ถ้าโฟลเดอร์ยังไม่ได้ trust ตอนเปิด `apw` จะอธิบายว่า trust อนุญาตอะไร (ให้รัน install/build/test ของโปรเจกต์ใน worktree ด้วยสิทธิ์ของผู้ใช้) แล้วถาม `Trust this folder? [y/N]` ถ้าตอบไม่ ยังใช้ `plan`, `review`, `preview` และโหมด `unverified` ได้

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
- **worktree อยู่นอกโปรเจกต์:** APW สร้าง worktree ของแต่ละงานไว้ที่ `%LOCALAPPDATA%\apw\wt\<workspace>\<task>` (Linux: `~/.cache/apw/wt/...`) ไม่ใช่ในโฟลเดอร์โปรเจกต์ Vitest, Jest และ `eslint .` จึงไม่ค้นเจอสำเนาของ APW และ path ไม่ยาวขึ้นตามโปรเจกต์ ถ้ายังติดข้อจำกัด 260 ตัวอักษรของ Windows (dependency ของ pnpm ซ้อนลึกมาก) ให้ตั้ง `"worktree_root": "D:\\apw-wt"` ใน `apw.json` (ต้องเป็น absolute path นอกโปรเจกต์) หรือเปิด Windows long paths ซึ่ง APW จะแจ้งไว้ใน error
- **งานเก่าใน `.worktrees/`:** งานที่สร้างก่อน rc.12 ยังอยู่ใน `.worktrees/` ของโปรเจกต์และยัง resume/apply/cleanup ได้ตามเดิม (APW ไม่ย้ายหรือลบให้เอง) ระหว่างนั้นถ้า test runner ค้นเจอ ให้ exclude `.worktrees/**` ชั่วคราว แล้วลบงานเก่าด้วย `apw cleanup`

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

### Sandbox สำหรับคำสั่งตรวจสอบ

ขั้นตรวจสอบจะรันโค้ดของโปรเจกต์ (install script, build, test) ค่าเริ่มต้น (`off`) รันด้วยสิทธิ์ของผู้ใช้และ environment ทั้งหมด ถ้าไม่ไว้ใจโค้ดในโปรเจกต์ ให้เปิด sandbox ใน `apw.json`:

```json
"sandbox": { "mode": "restricted", "network": "install", "memory_mb": 4096, "pass_env": ["NPM_TOKEN"] }
```

- **`restricted` บน Windows:** รันด้วย token ระดับ low integrity ใน job object ที่จำกัด UI และหน่วยความจำ เขียนได้เฉพาะใน worktree เขียนโปรเจกต์ต้นทาง, `.git` ของ repo หลัก (จึงฝัง hook ไม่ได้), manifest ของงาน และไฟล์อื่นของผู้ใช้ไม่ได้ แต่ยังอ่านไฟล์ของผู้ใช้ได้และยังออก network ได้
- **`restricted` บน Linux:** ใช้ bubblewrap (`bwrap`) โดย root เป็น read-only, worktree เขียนได้, มี `/tmp` แยก และไม่มี network ยกเว้นขั้น install
- **`docker`:** แต่ละขั้นรันใน container ใหม่จาก `sandbox.image` (ต้องมี toolchain ของโปรเจกต์ เช่น `node:22`) โดย mount เฉพาะ worktree, ตัด capability ทั้งหมด และไม่มี network ยกเว้นขั้น install เหมาะกับ repo ที่ไม่ไว้ใจ
- โหมด `restricted` และ `docker` จะลบตัวแปรที่ดูเป็นความลับ (ชื่อมี KEY, TOKEN, SECRET, PASSWORD, ... และ `APW_*`) ออกจาก environment ยกเว้นที่ระบุใน `pass_env` ส่วน cache และไฟล์ชั่วคราวของเครื่องมือจะย้ายไปอยู่ใน `<worktree>/.apw-sandbox`
- cache เริ่มใหม่ทุกงาน: Go รอบแรกช้าขึ้นราว 10-20 วินาที และ pnpm/npm จะดาวน์โหลดแพ็กเกจใหม่ รายละเอียดและข้อจำกัดดูที่ `docs/task-lifecycle.md` หัวข้อ "Verification sandbox"

### ตรวจหน้าเว็บด้วย browser

ถ้าเครื่องมี Microsoft Edge, Google Chrome หรือ Chromium (หรือระบุไฟล์ด้วย `APW_BROWSER`) โปรเจกต์เว็บฝั่ง client (Vite, React, Vue, Svelte, Preact, Solid ที่มี script `build`; ไม่รวม Next.js) และเว็บ HTML ล้วนจะมีขั้น `browser` เพิ่มหลัง build: APW เสิร์ฟไฟล์ใน `dist/` เองผ่าน server ที่ฟังเฉพาะ 127.0.0.1 แล้วเปิด `index.html` แบบ headless
- ไม่ผ่านถ้ามี error ที่ไม่ถูกจับในหน้า หรือหน้าว่างเปล่า (อาการจอขาวหลัง runtime crash) และส่งให้รอบซ่อมเหมือน error จาก compiler
- promise rejection ที่ไม่ถูกจับ (เช่น API ไม่ออนไลน์ระหว่างตรวจ) จะรายงานไว้แต่ไม่นับว่าไม่ผ่าน และเก็บ screenshot ไว้ที่ `<worktree>/.apw/browser/index.png`
- แผนใส่ acceptance check ชนิด `browser` ได้ เช่น `{"criterion":"ตัวนับแสดง 10","kind":"browser","file":"/","value":"count is 10"}`
- ปิดขั้นนี้ด้วย `"verify": { "browser": "off" }`

### อ่านเอกสารก่อนวางแผน (research)

เปิดด้วย `"research": { "enabled": true }` แล้ว Architect จะขอหน้าเอกสารที่ต้องใช้ก่อนวางแผน และ APW จะดึงมาให้เอง:
- ดึงเฉพาะ HTTPS GET ไปยังเว็บเอกสารใน allowlist (เช่น pkg.go.dev, docs.rs, developer.mozilla.org, react.dev, vite.dev, registry.npmjs.org) และ host ที่เพิ่มเองใน `research.hosts`; ไม่รับ query string, credential, cookie หรือ IP ภายในเครือข่าย
- เนื้อหาที่ดึงมาถูกใส่ใน prompt โดยระบุว่าเป็นข้อมูลอ้างอิงที่ไม่น่าไว้ใจ และห้ามทำตามคำสั่งในนั้น
- ค้นเว็บได้ด้วย `"search": "brave"` และ key ใน `BRAVE_API_KEY`
- ถ้าดึงไม่สำเร็จ งานยังวางแผนต่อได้ตามปกติ และรายงานจะบอกว่าใช้แหล่งไหนบ้าง

### คำสั่งประจำโปรเจกต์และบริบทก่อนวางแผน

- **`APW.md` (หรือ `AGENTS.md`):** วางไว้ที่ root ของโปรเจกต์เพื่อบอกข้อกำหนด เช่น สไตล์โค้ด โครงสร้างโฟลเดอร์ หรือภาษาที่ใช้ใน UI APW จะใส่ไว้ใน prompt ของการวางแผน, review และการสร้าง patch (สูงสุด 12 KiB) แต่ข้อความในไฟล์นี้เปลี่ยนกฎของ APW ไม่ได้
- **ค้นโค้ดก่อนวางแผน:** ถ้าบริบทโค้ดเกิน 96 KiB Architect ขอ `grep`, `glob` หรือ `read` ช่วงบรรทัดได้ก่อนวางแผน เป็นการอ่านอย่างเดียว ไม่รันโค้ด และมองไม่เห็น `.git`, `.apw`, `.env` หรือไฟล์ key ตั้ง `"planning": {"lookup": "always"}` ให้ค้นได้ทุกงาน หรือ `"off"` เพื่อปิด
- **ความจำของ session:** เก็บ 3 turn ล่าสุดแบบละเอียด ส่วน turn เก่าย่อเหลือบรรทัดเดียว จึงไม่บวมแม้คุยกันนาน

### ให้หลายโมเดลเสนอแผน

```json
"planning": { "proposals": 3, "judge": true }
```

ถ้ามี Architect มากกว่าหนึ่งตัว (`roles.architect` และ `failover.architect`) APW จะให้แต่ละตัววางแผนพร้อมกัน แล้วให้คะแนนแต่ละแผนตามกฎ: ทุกเกณฑ์ต้องมี check ที่รันได้จริง, check อ้างถึงขั้นที่มีอยู่จริง, ระบุขอบเขตไฟล์และการตรวจสอบ, แก้เฉพาะไฟล์ที่ประกาศไว้ และแผนที่เล็กกว่าได้คะแนนดีกว่า ถ้า `judge` เป็น `true` จะให้ Architect หลักให้คะแนน 0-10 เพิ่ม แล้วเฉลี่ยกับคะแนนตามกฎ แผนที่คะแนนสูงสุดจะถูกใช้ และ `plan.ranking` จะเก็บทุกข้อเสนอพร้อมคะแนนและเหตุผล (`apw plan` แสดงให้ดู)

### วัดผล, GitHub issue และ ACP

```bash
apw eval                         # วัดผล APW กับชุดงานใน eval/testdata (มี hidden test) แล้วสรุป solved / false positive
apw eval --baseline .apw/eval/<ไฟล์เก่า>.json   # เทียบกับรอบก่อน
apw issue 42                     # เปลี่ยน GitHub issue เป็นงานที่ verify แล้ว เปิด draft PR (ใช้ gh) โดยไม่แตะไฟล์ในโปรเจกต์
apw acp                          # ให้ editor หรือ agent hub ที่รองรับ Agent Client Protocol เรียกใช้ APW ผ่าน stdio
```

- **`apw eval`:** แต่ละงานเริ่มจาก repo ตัวอย่าง ให้ APW วางแผน แก้ verify และ apply ตามปกติ แล้วค่อยใส่ hidden test ที่ APW ไม่เคยเห็นเข้าไปตรวจ ตัวเลขที่สำคัญที่สุดคือ false positive (APW บอกว่าผ่านแต่ hidden test ไม่ผ่าน) รายละเอียดดู `docs/eval.md`
- **`apw issue`:** อ่าน issue ด้วย `gh` รันเป็นงานปกติ (ต้อง trust) ถามก่อนเผยแพร่ แล้ว commit เฉพาะไฟล์ที่ verify แล้วบน branch ของงาน push เป็น `apw/issue-<n>-...` และเปิด draft PR ถ้างานไม่ผ่านหรือไม่ได้แก้ไฟล์ จะไม่ push อะไรเลย
- **`apw acp`:** โหมด ask/auto/plan/unverified กลายเป็น session mode ของ ACP ความคืบหน้าแสดงเป็น tool call และจะขออนุญาตผ่าน ACP ก่อน apply

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
