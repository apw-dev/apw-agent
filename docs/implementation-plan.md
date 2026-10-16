# แผนพัฒนา apw — Apiwat Agent

สถานะ: แผนเสนอ revision 6 — ตัด browser model provider (Gemini/Argon, go-rod/CDP) ออกจากขอบเขตทั้งหมด เหลือ provider แบบ API และ local; รับคำสั่งผ่าน Telegram/Discord Gateway; แกน Triad Engine (Architect, Surgeon, Reflex) มีโค้ดขั้นต้นแล้วใน M0

## 1. เป้าหมายและขอบเขต

สร้าง agent ด้วย Go ที่ใช้งานผ่าน Telegram หรือ Discord ได้คล้ายแนวทาง Hermes รับงานต่อเนื่องผ่านบริการเบื้องหลัง เข้าใจบริบท สร้างแผนหรือ patch แก้ไฟล์ในพื้นที่แยก และส่งผลตรวจสอบกลับแชตเดิม โดยเลือกใช้โมเดลได้ทั้งผ่าน API และโมเดล local

สถานะปัจจุบันของ repository: มีโครง `cmd/apw`, `cmd/apwd`, `internal/core`, `internal/provider` (interface) และแกน Triad ใน `internal/architect`, `internal/surgeon`, `internal/reflex` พร้อมเทสต์; คำสั่ง `apw verify` และ `apw symbols` ใช้งานได้ ยังไม่มี provider จริง, gateway, session หรือ state

**ข้อกำหนดหลักที่ห้ามเลื่อนไปเป็นของเสริม**

1. สั่งงาน สนทนาต่อเนื่อง ดูสถานะ ยกเลิก และรับผลกลับผ่าน Telegram หรือ Discord ได้ โดย MVP ส่งมอบอย่างน้อยหนึ่งช่องทาง และ Beta รองรับทั้งสองช่องทาง
2. เลือก provider ได้จาก config อย่างน้อยหนึ่งแบบที่ใช้งานจริงได้ (API หรือ local) พร้อม mock provider แบบ deterministic สำหรับเทสต์
3. คำตอบจากโมเดลต้องนำไปวางแผน สร้าง patch และตรวจผลได้ครบวงจรอัตโนมัติ การคัดลอกคำตอบมาวางด้วยมือไม่ถือว่าผ่าน

**สมมติฐานสำหรับประมาณเวลา**

- นักพัฒนาเต็มเวลา 1 คน ใช้กรอบ 12 สัปดาห์สำหรับเส้นทางหลัก และเผื่ออีก 2 สัปดาห์สำหรับรวมระบบและแก้ปัญหา การตัด browser provider ลดงานใน M0–M2 จึงต้องประเมินใหม่หลังจบ M0 และทุก milestone
- รุ่นแรกพัฒนาและใช้งานบน Windows เป็นหลัก รองรับ repository ภาษา Go ก่อน
- ใช้ Telegram เป็นช่องทางแรกตามแผนเสนอ แล้วเพิ่ม Discord ใน Beta; CLI เป็นเครื่องมือ setup/debug ที่เรียก core เดียวกัน
- provider แบบ API ต้องมี API key ที่ผู้ใช้ตั้งเอง ส่วน local provider เป็นทางเลือกที่ไม่ต้องใช้ key; CI และเทสต์หลักใช้ mock provider เท่านั้น
- ใช้โปรเจกต์ Go ตัวอย่างเป็นพื้นที่ทดสอบ รวมทั้งกรณี build ไม่ผ่าน ไฟล์ถูกแก้ระหว่างทำงาน และผู้ใช้ยกเลิกงาน
- ไม่มีข้อกำหนดเรื่องงบโมเดลหรือเครื่อง local ในเอกสาร จึงต้องตั้งเพดาน request, token และเวลาต่อ task ผ่าน config และเก็บค่าที่ใช้จริงก่อนเลือกค่าเริ่มต้นสำหรับ release

## 2. ผลลัพธ์ที่ต้องใช้งานได้

### MVP: จบ milestone M2

ผู้ใช้ส่งงานใน Telegram ระบบใช้ provider ตาม config (API หรือ local) ส่งบริบทที่จำเป็นให้โมเดล รับคำตอบกลับมาสร้างแผน/patch สำหรับ Go หนึ่งจุด ทดลองใน Git worktree รัน fmt/vet/build/test และส่งสรุป diff/report กลับแชตเดิม ผู้ใช้ดูสถานะและยกเลิกจากแชตได้ หากล้มเหลวต้องไม่ทำให้ working tree เดิมเสียหาย

ต้องมีหลักฐานทดสอบกับ provider จริงอย่างน้อยหนึ่งตัวแบบ opt-in นอกเหนือจาก mock

### รุ่น Beta: จบ milestone M4

รองรับ Telegram และ Discord ผ่าน gateway เดียวกัน มี router สองระดับและ planner สำหรับงานหลายไฟล์ พร้อมการยกเลิกงาน การกู้งานหลัง daemon restart และประวัติที่ยังอ่านได้

### รุ่น v1: จบ milestone M5

เพิ่มความจำการแก้ปัญหาและ skill registry ที่บันทึกแหล่งที่มา ตรวจสอบได้ และมีขอบเขตสิทธิ์ชัดเจน พร้อมแพ็กเกจติดตั้งและเอกสารใช้งาน

**เลื่อนออกจากเส้นทางหลัก:** browser/web-UI model provider, VS Code extension, Web UI ของ apw, public remote RPC, semantic rename ข้ามภาษา, voice/video และการรัน skill ที่สร้างเองโดยไม่ผ่านการเปิดใช้ การเชื่อม Telegram/Discord ใช้ outbound connection จากเครื่องที่รัน agent ได้ตาม transport ที่เลือก จึงแยกจากการเปิด public RPC

## 3. ข้อตกลงทางสถาปัตยกรรม

| ประเด็น | แนวทางในแผนนี้ | เหตุผลและเงื่อนไขขยาย |
| --- | --- | --- |
| Core | ให้ application service เป็นศูนย์กลางตั้งแต่ต้น | CLI, chat gateway และ daemon ใช้ task lifecycle และ policy เดียวกัน |
| Parser | เริ่ม Go ด้วย `go/parser` และ `go/ast` หลัง interface ของ indexer | ลดขอบเขตการ build รุ่นแรก; เพิ่ม tree-sitter adapter เมื่อเริ่มภาษาอื่นและผ่าน spike บน Windows |
| Model | provider interface (`internal/provider`) รองรับ `api`, `local`, `mock` และ capability ของแต่ละแบบ | ไม่สมมติว่าทุก provider มี native tool calling หรือ constrained JSON; ตรวจ output และ normalize ก่อนเข้า core |
| Gateway | platform adapter → session/task service → event/outbox → platform adapter | แยก platform จาก agent; แยก conversation, owner และ workspace อย่างชัดเจน |
| Patch | ระบุไฟล์, symbol, base hash, expected text และ replacement | ป้องกันการนำ patch เก่าไปทับไฟล์ที่เปลี่ยนแล้ว และไม่อาศัย line number เพียงอย่างเดียว |
| Isolation | งานแก้โค้ดทำใน Git worktree แยก มี lock ต่อ task | รักษาไฟล์งานของผู้ใช้; worktree ไม่ใช่ security sandbox สำหรับรันโค้ด |
| Verification | runner ใช้ executable + argv + cwd + timeout | ไม่รัน shell string ที่โมเดลส่งมาตรง ๆ; ตรวจสิทธิ์ก่อนเริ่ม process |
| State | SQLite สำหรับ task/session, inbox dedup, outbox และประวัติ | เลือก driver หลัง spike บน Windows ใน M0; gateway ต้องบันทึก event และ task mapping ก่อนรับ message ถัดไป |
| Protocol | ใช้ชื่อ `pkg/protocol` เป็นสัญญากลาง | แก้ความไม่ตรงกันของชื่อ `pkg/rpc` และ `pkg/protocol` ในเอกสารต้นฉบับ |
| Transport | local-only ก่อน; ประเมิน named pipe หรือ loopback ที่มี token บน Windows | ไม่สมมติว่า Unix socket เป็นทางเดียว และยังไม่เปิด remote listener |

JSON Schema ตรวจได้ว่า response มีโครงสร้างถูกต้อง แต่ไม่ได้พิสูจน์ว่าโค้ดถูก type หรือทำงานถูก ต้องอาศัย compiler, static analysis และ test เพิ่มเติม ระบบรายงานว่า “ผ่านคำสั่งตรวจสอบใดบ้าง” แทนการรับประกันความถูกต้องทั้งหมด

AST ใช้หาขอบเขตและ symbol ได้ แต่ semantic rename ต้องมีข้อมูลการอ้างอิงและชนิดข้อมูลเพิ่มเติม จึงไม่จัดเป็นคำสั่ง rename ที่พร้อมใช้เพียงเพราะมี AST

### 3.0 ปรัชญาแกนกลาง: Triad of Next-Gen Agent (Architect, Surgeon, Reflex)

apw ออกแบบขึ้นภายใต้แกนปรัชญาสามประสาน (Triad Engine) เพื่อยกระดับความแม่นยำและความปลอดภัยในการดัดแปลงโค้ดเทียบเท่ามาตรฐานของ **Claude Code** และโมเดลเอเจนต์ชั้นนำยุคใหม่:

1. **The Architect (การคิดเชิงสถาปัตยกรรมและวางผังงาน)** — `internal/architect`
   - **หน้าที่:** ถอดรหัสความต้องการของผู้ใช้ (intent analysis), ทำความเข้าใจโครงสร้างโปรเจกต์เชิงลึก (system & symbol topology), และแตกงานเป็นขั้นตอนย่อยที่มีลำดับขึ้นต่อกัน (DAG of atomic subtasks)
   - **กลยุทธ์โมเดล:** มอบหมายให้โมเดลระดับสูงที่มี reasoning capacity กว้างขวางผ่าน API provider รับหน้าที่คิด ออกแบบโครงสร้าง และตรวจสอบ architecture tradeoffs ชื่อโมเดลกำหนดผ่าน config (`Architect.Model`) ไม่ hard-code
   - **สัญญาการทำงาน:** สถาปัตย์จะไม่ลงมือแก้ไขไฟล์โดยตรง แต่ส่งมอบ `ExecutionPlan` ที่ระบุเป้าหมาย, ไฟล์ที่เกี่ยวข้อง, symbol ที่กระทบ และ acceptance criteria; `ExecutionPlan.Validate()` ปฏิเสธ action ที่ไม่รู้จักและ `target_file` ที่ออกนอก workspace ก่อนส่งต่อ
   - **ทำแล้ว:** planning prompt, `ParsePlan` + `Validate` · **ยังไม่ทำ:** DAG dependencies, เชื่อม provider จริง

2. **The Surgeon (ศัลยแพทย์โค้ดตามหลักการ Claude Code)** — `internal/surgeon`
   - **หลักการ Read-Before-Write:** ห้ามแก้ไขไฟล์ใดๆ โดยไม่อ่านบริบทจริงล่าสุด (snapshot อ้างอิงด้วย absolute path)
   - **Precision Surgical Patching:** ไม่เขียนทับทั้งไฟล์หากไม่จำเป็น ใช้การแทนที่เฉพาะจุดที่ต้องตรงแบบ unique และรักษาชนิดขึ้นบรรทัด (CRLF/LF) เดิมของไฟล์
   - **Base Hash & Integrity Fingerprint:** ทุก patch ตรวจ SHA-256 ของไฟล์ตั้งต้น หากไฟล์บนดิสก์ถูกแก้จากภายนอกระหว่างทำงาน ปฏิเสธการแก้ทับทันที (Refuse on drift)
   - **Safe Workspace Isolation:** การผ่าตัดโค้ดทุกครั้งต้องทำใน **Git Worktree** แยก (ยังไม่ทำ — M1)

3. **The Reflex (การสะท้อนคิด แก้ไขตัวเอง และทดสอบอัตโนมัติ — Reflexion Loop)** — `internal/reflex`
   - **Closed-Loop Verification:** หลัง Surgeon แก้เสร็จ รันวงจรตรวจแบบ multi-stage: `gofmt` → `go vet` → `go build` → `go test` แล้วแปลง output เป็น `Diagnostic` ที่มีไฟล์/บรรทัด/คอลัมน์ (ทำแล้ว ใช้ผ่าน `apw verify`)
   - **Autonomous Self-Healing:** หากคอมไพเลอร์หรือเทสต์ล้มเหลว นำ diagnostic กลับไปให้โมเดลสร้าง surgical fix วนซ้ำแบบ bounded retry สูงสุด 3 ครั้ง (`ReflexEngine.MaxRetries` มีแล้ว ลูปยังไม่ทำ)
   - **Fast-Path Reflex:** ข้อผิดพลาดเชิงไวยากรณ์หรือ type เล็กๆ ใช้โมเดลขนาดเล็ก/เร็วผ่าน API หรือ local model แก้ โดยไม่ต้องย้อนกลับไปที่ Architect เว้นแต่เป็นปัญหาเชิงสถาปัตยกรรม

### 3.1 Messaging Gateway แบบ Hermes

ยึดแนวทางแยก platform adapter ออกจาก gateway/session และตัว agent ตาม [Hermes platform adapter architecture](https://hermes-agent.nousresearch.com/docs/developer-guide/adding-platform-adapters) และ [Messaging Gateway](https://hermes-agent.nousresearch.com/docs/user-guide/messaging/) โดยสร้าง implementation ของ apw เอง

- **Telegram:** เริ่มจาก bot ใน private chat ใช้ long polling; เก็บ update ID และ offset หลังรับลง durable inbox เพื่อป้องกันการสร้าง task ซ้ำ ตามรูปแบบ [Telegram Bot API](https://core.telegram.org/bots/api#getupdates)
- **Discord:** เพิ่ม bot ใน DM และ channel/thread ที่กำหนด ใช้ Gateway สำหรับรับ event พร้อม reconnect/heartbeat ตาม [Discord Gateway](https://docs.discord.com/developers/events/gateway); เริ่มรับงานจาก mention หรือคำสั่งที่กำหนดและจัด intents ให้สอดคล้อง
- ใช้ bot credentials ของ platform และ allowlist ของ user ID + chat/guild/channel ID; bind workspace จาก configuration ที่เจ้าของตั้งไว้ ไม่รับ absolute path ตามข้อความใด ๆ โดยตรง
- session key ต้องรวม platform, account/bot, chat/channel/thread และ owner; การต่อบริบทข้ามช่องทางต้องเป็นการ link โดยเจ้าของ ไม่จับคู่จากชื่อแสดงผล
- รองรับรับงานภาษาธรรมชาติและคำสั่งเชิงตรรกะ `new`, `project`, `model`, `plan`, `run`, `status`, `cancel`, `result`; แต่ละ adapter แปลงเป็นรูปแบบคำสั่งของ platform
- ตอบรับทันทีพร้อม task ID แล้วส่ง progress เฉพาะสถานะสำคัญ ไม่ส่ง log ทุกบรรทัด; งานต่อเนื่องได้แม้ผู้ใช้ปิดแอปแชต และผลอยู่ใน outbox รอส่งใหม่เมื่อเน็ตกลับ
- แบ่งข้อความยาว/แนบไฟล์ report ตามข้อจำกัด platform; escape markup และไม่ส่ง secret หรือ source ทั้ง repo กลับแชต
- inbox dedup ป้องกัน task ซ้ำ; outbox มี delivery status/retry ไม่อ้างว่าส่งข้อความได้ exactly-once ทุกกรณีเมื่อ crash ระหว่างส่ง
- หากต้องยืนยัน action ให้ผูก token กับ task + plan hash + owner + expiry; ผู้ใช้อื่นหรือการกดซ้ำไม่สามารถอนุมัติแทนได้ และใช้ execute policy เดียวกับ CLI

### 3.2 Model Provider — API และ Local

**เป้าหมาย:** core เรียกโมเดลผ่าน `provider.Provider` (`Name`, `Ask`, `Close`) โดยไม่รู้ว่าเบื้องหลังเป็น API หรือ local และสลับได้จาก config

1. **ชนิด provider:** `api` (บริการโมเดลผ่าน HTTPS), `local` (เช่น runtime ที่เปิด HTTP endpoint ในเครื่อง) และ `mock` (คืนผลจาก fixture แบบ deterministic สำหรับเทสต์และ CI)
2. **Credential:** config เก็บเพียง credential reference (เช่นชื่อ environment variable) ไม่เก็บ API key ใน config ที่ commit ได้ ไม่ log และไม่ส่ง key กลับแชต
3. **Capability:** แต่ละ provider ประกาศ capability เช่น context window, structured output, tool calling และ streaming; core เลือก prompt/การ parse ตาม capability ไม่สมมติว่าทุกตัวรองรับเหมือนกัน
4. **Model selection:** ชื่อโมเดลแยกตามบทบาท (architect, reflex) และต่อ session ได้; บันทึก requested model กับ model ที่ response รายงานจริงใน `ModelResponse.ModelObserved`
5. **Prompt:** ส่ง instruction + context ที่คัดแล้ว + response schema โดยมีเพดานขนาด; ไม่ส่ง secret และส่ง source เฉพาะในขอบเขต project/provider policy ที่ผู้ใช้เปิดใช้
6. **Timeout & cancel:** ทุก request ผูก `context` ที่มี deadline; หลังยกเลิก task ห้ามนำ response ที่ตามมาช้าไป apply patch
7. **Rate limit & retry:** retry แบบ exponential backoff + jitter เฉพาะ error ที่ retry ได้ (เช่น 429/5xx/timeout) ภายใต้ budget ของ task; auth error หรือ quota หมดให้สถานะ `provider_unavailable` พร้อมเหตุผล
8. **Usage:** เก็บ token/cost จาก usage ที่ provider รายงาน ถ้าไม่มีข้อมูลให้เป็น unknown ไม่รายงานเป็นศูนย์
9. **Normalize:** เก็บ raw response และสร้าง `ModelResponse` ที่มี provider, request ID, model observation, completion status และ content; parse เป็น Plan/PatchSet แล้วตรวจ schema และ scope ตาม flow ปกติ
10. **Fallback:** เปลี่ยน provider ได้เฉพาะตาม fallback policy ที่เจ้าของเปิดไว้ และต้องรายงานทุกครั้งที่เปลี่ยนเส้นทาง

### 3.3 โมดูลที่เพิ่มจากสเปกเดิม

```text
internal/architect/                # ExecutionPlan, planning prompt, validation (มีแล้ว)
internal/surgeon/                  # read-before-write, hash-checked patch, AST symbols (มีแล้ว)
internal/reflex/                   # fmt/vet/build/test pipeline + diagnostics (มีแล้ว)
internal/provider/                 # Provider interface (มีแล้ว)
internal/provider/api/             # API provider
internal/provider/local/           # local model provider
internal/provider/mock/            # deterministic provider สำหรับเทสต์
internal/gateway/                  # รับ event, session routing, inbox/outbox
internal/channels/telegram/        # Telegram bot adapter
internal/channels/discord/         # Discord bot adapter
internal/session/                  # owner + conversation + workspace + provider
```

core, daemon และ provider ทั้งหมดเขียนด้วย Go ไม่มี runtime ภายนอกเพิ่ม (local model runtime เป็นสิ่งที่ผู้ใช้ติดตั้งเองและ apw เชื่อมผ่าน endpoint)

## 4. ลำดับทำงานหลักและข้อมูลกลาง

```text
Telegram / Discord / CLI
  → Gateway + ตรวจ owner/workspace + inbox dedup
  → Session / Task service + ตรวจ policy
  → จัดประเภทงาน
  → อ่าน repo map และบริบทที่จำเป็น
  → Model router: API / Local
  → ส่ง prompt → รอคำตอบ → normalize + validate
  → สร้างและตรวจ Plan/PatchSet
  → ตรวจ base hash / path / symbol
  → ทดลองใช้ใน worktree
  → format + vet + build + test
  → ซ่อมตามข้อผิดพลาดภายในเพดาน
  → บันทึกสถานะ + diff + verification report
  → Outbox ส่งผลกลับ conversation ที่เป็นเจ้าของ task
```

ข้อมูลกลางที่ต้องกำหนดก่อนเชื่อมโมดูล:

- `Task`: ID, workspace, instruction, mode, status, deadline และ budget
- `Symbol`: stable identity ภายใน index revision, path, kind, name, span และ content hash
- `Plan`: ID, schema version, repo revision, steps, dependencies และ acceptance criteria
- `PlanStep`: ID, target files/symbols, dependency IDs, declared file scope และ verification profile
- `PatchSet`: task/step ID, base hashes และรายการแก้ไขที่ไม่มีขอบเขตทับกัน
- `VerificationReport`: executable/argv, working directory, exit code, duration, output reference และสถานะครบ/ไม่ครบ
- `TaskEvent`: sequence, timestamp, state transition และข้อความที่ผ่านการปิดบัง secret
- `ConversationSession`: platform identity, owner, workspace binding, context revision และ provider/model preference
- `InboundMessage` / `OutboundDelivery`: platform event/message ID, task mapping, dedup key, delivery state และ retry metadata
- `ModelRequest` / `ModelResponse`: request ID, provider capability, model ที่ขอ/ที่สังเกตได้, content, completion status, usage และ raw response reference

ใช้ verification profile เช่น `go-package` หรือ `go-repo` แทนการให้ `verification_cmd` ในแผนเป็น shell string อิสระ profile จะแปลงเป็นคำสั่งที่ project policy อนุญาต

สถานะหลัก: `queued → analyzing → planning/patching → verifying → succeeded` พร้อม `failed`, `cancelled` และ `needs_review`; เพิ่มสถานะรอ `waiting_for_user` และ `provider_unavailable` ที่มี timeout/resume policy ทุกสถานะต้องมีเหตุผลและ artifact ที่เกี่ยวข้อง

คำว่า `succeeded` หมายถึงงานและการตรวจที่กำหนดเสร็จในพื้นที่แยก การนำผลกลับเข้า working tree ของผู้ใช้เป็นอีกขั้นที่ผู้ใช้เลือก และต้องตรวจ base hash ซ้ำ

## 5. Milestones และเกณฑ์ส่งมอบ

| Milestone | เวลาโดยประมาณ | สิ่งที่ทำ | เกณฑ์ผ่านก่อนขึ้นช่วงถัดไป |
| --- | --- | --- | --- |
| **M0: Foundation + feasibility** | สัปดาห์ 1 | Go skeleton, Triad engine ขั้นต้น, provider/session contracts, mock provider, SQLite/Windows spike, Telegram bot และ API provider proof of concept | bot รับ/ตอบข้อความทดสอบได้; ส่ง prompt ผ่าน API provider แล้ว `ParsePlan` ได้แผนที่ผ่าน `Validate`; CI ผ่าน; บันทึกข้อจำกัดจริง |
| **M1: Safe core + gateway** | สัปดาห์ 2–3 | worktree, runner, patch/rollback, `apwd` ขั้นต้น, Telegram adapter, session และ inbox/outbox | สั่ง mock task จากแชต ดูสถานะ/cancel/รับ report ได้; duplicate event ไม่สร้างงานซ้ำ; ไฟล์เดิมไม่เสียหาย |
| **M2: Chat + model MVP** | สัปดาห์ 4–5 | Go AST, API/local provider, output validation, Reflex repair loop และ flow ครบวงจร | Telegram → provider → patch Go → fmt/vet/build/test → ส่งผลกลับแชตเดิมได้; timeout/quota/cancel มีสถานะจริง |
| **M3: Discord + Dual-brain** | สัปดาห์ 6–8 | Discord adapter, router, provider ต่อบทบาท architect/reflex, repo map, DAG planner และ executor | ใช้ทั้งสองแพลตฟอร์มได้; session ไม่ปะปน; งานหลายไฟล์มีแผนและ final gate; เลือก provider/model ต่อ session ได้ |
| **M4: Reliability Beta** | สัปดาห์ 9–10 | local RPC, watcher, reconnect/restart recovery และ delivery retry | อ่าน task หลัง restart ได้; ระบุ interrupted โดยไม่ส่ง request ซ้ำแบบเดา; outbox กลับมาส่งได้; shutdown ไม่ทิ้ง process |
| **M5: Memory, skills, release** | สัปดาห์ 11–12 | episodic memory, skill registry, permissions, packaging และเอกสาร | memory ตรวจ/ลบได้; skill ที่ไม่เปิดใช้รันไม่ได้; เครื่องใหม่ตั้งค่า bot และ provider แล้วผ่าน acceptance demo |

**สัปดาห์ 13–14 เป็น buffer:** แก้ปัญหาจากใช้งานจริงและ packaging ไม่ถือว่า TypeScript/Python/extension จะเสร็จโดยอัตโนมัติใน buffer หาก provider หรือ gateway spike ไม่ผ่านต้องประเมินเวลาใหม่ทันที

ปรับจากสเปกเดิมโดยย้าย daemon ขั้นต้นและ chat gateway มา M1 และเพิ่ม Discord ใน M3 ส่วน M4 เน้นความทนทาน แทนการเพิ่งเริ่ม daemon ตอนปลายโครงการ

### M0 — Foundation

1. ~~สร้าง Git repository และโครง `cmd/apw`, `cmd/apwd`, `internal/core`, `internal/provider`~~ (ทำแล้ว); เพิ่ม `internal/config`, `internal/gateway`, `internal/session`, `internal/task` และ `testdata`
2. ~~Triad engine ขั้นต้น: Architect (`ExecutionPlan` + `Validate`), Surgeon (read-before-write, hash drift, AST symbols), Reflex (fmt/vet/build/test + diagnostics)~~ (ทำแล้ว)
3. เขียนข้อสรุปสถาปัตยกรรมสั้น ๆ เรื่อง parser, provider รายแรก, patch format, verification และ task lifecycle
4. สร้าง config สำหรับ provider, model, endpoint, credential reference, timeout, workspace policy และ budget; แยก secret จาก config ที่ commit ได้
5. เพิ่ม mock provider ที่ให้ผล deterministic สำหรับ integration test และ smoke test การเชื่อมต่อ provider จริงแบบ opt-in
6. ~~ตั้ง build/test/vet และ format check ใน CI โดยไม่ต้องใช้ API key~~ (ทำแล้ว — `.github/workflows/ci.yml`)
7. ทดลอง Telegram private bot ด้วยบัญชีที่เจ้าของเปิดใช้; ไม่เปิดรับผู้ใช้ทั่วไปในขั้นทดลอง
8. พิสูจน์ provider round trip: ส่ง planning prompt ผ่าน API provider → รับคำตอบ → `ParsePlan` + `Validate` → บันทึก model observation และ usage; ทดสอบ response ยาว, JSON ผิดรูป และ timeout
9. ตัดสินใจ API provider รายแรกและ local runtime ที่รองรับ พร้อมกำหนดรูปแบบ credential reference

### M1 — Safe execution

1. ตรวจ repo root, Git state และ path ที่อนุญาตก่อนสร้างพื้นที่งาน; รุ่นแรกให้ repository ที่ยังไม่มี commit หรือมีสถานะที่รองรับไม่ได้จบด้วยคำอธิบายที่ชัดเจน
2. รองรับ working tree ที่มีงานค้างโดยไม่ reset, stash หรือคัดลอกความเปลี่ยนแปลงเข้า task อย่างเงียบ ๆ รุ่นแรกให้หยุดงานแก้ไขและแนะนำให้ผู้ใช้สร้างฐานที่พร้อมใช้งาน; คำสั่งอ่านอย่าง map ยังใช้ได้
3. ตรวจ path traversal, symlink/reparse point ที่พาออกนอก workspace (เช่นเปิดไฟล์ผ่าน `os.Root`), file hash, ขอบเขต symbol และ expected text ก่อน patch
4. ทดลอง patch ทุกไฟล์ ตรวจ parse และ format แล้วเขียนอย่าง atomic ต่อไฟล์; ถ้าไฟล์ใดใช้ไม่ได้ให้คืน snapshot ของทั้ง patch set ใน worktree นั้น
5. รัน process ผ่าน policy พร้อม timeout, จำกัด output และ kill process tree เมื่อ cancel โดยทดสอบพฤติกรรมบน Windows
6. ปฏิเสธ repository task ที่ต้องรันโค้ดจนกว่าจะผ่านการตั้งค่า trust/policy เพราะ build และ test อาจเรียกโค้ดหรือ subprocess ของ repository ได้
7. เก็บ baseline ของคำสั่งตรวจสอบก่อนแก้ เพื่อแยกข้อผิดพลาดเดิมออกจากผลของ patch
8. เพิ่ม `apwd` ขั้นต้นและ Telegram adapter พร้อม owner/workspace binding, durable inbox/outbox, session, status/cancel และ task event; ใช้ mock provider ทดสอบแชตถึง core ก่อนเชื่อมโมเดลจริง
9. เพิ่ม request ID และ cancellation ของ provider call; หลังยกเลิกห้ามนำ response ที่ตามมาช้าไป apply patch

### M2 — Telegram + Model Provider + Go MVP

1. สแกน Go symbols และสร้าง repo map ที่ตัด generated/vendor/build output ตาม policy; จัดการ encoding และ CRLF โดยไม่ทำให้ diff เปลี่ยนทั้งไฟล์
2. ให้ผู้ใช้เลือก mode `preview` หรือ `execute` อย่างชัดเจน; การ execute ใน worktree ไม่ใช่การ merge กลับ workspace
3. ตรวจ JSON ตาม schema, field ที่ไม่รู้จัก, ขนาด response และขอบเขตไฟล์ แล้วตรวจ patch อีกชั้น
4. ใช้ `gofmt`, `go vet ./...`, `go build ./...` และ test profile ที่เหมาะกับ repository โดย full gate ต้องผ่านก่อนจบงาน
5. จำกัด repair เป็น **สร้างครั้งแรก 1 ครั้ง + ซ่อมสูงสุด 3 ครั้ง** ทุกครั้งมีรายงานแยก; ถ้าเกิน budget/deadline ให้หยุดก่อนครบได้
6. ส่ง error excerpt (จาก `reflex.FormatDiagnostics`) พร้อมบริบทที่จำเป็นให้โมเดลซ่อม โดยเก็บ log เต็มไว้ตรวจสอบ และปิดบัง secret
7. เมื่อซ่อมไม่สำเร็จให้คืนฐานใน worktree เก็บ patch/report สำหรับ review และรายงาน failed; ไม่จำเป็นต้องรอผู้ใช้ตอบเพื่อจบ task
8. เชื่อม API provider และ local provider จริง มี model selection, capability, usage และ normalization ตามข้อ 3.2
9. ส่ง progress, diff summary และ verification report กลับ Telegram session เจ้าของ task; สนทนาต่อได้โดยยังอ้างถึงงานเดิมและ revision ถูกต้อง
10. ทำ acceptance demo: สั่งผ่าน Telegram ให้ provider จริงแก้ fixture Go แล้วตรวจว่า patch/report เกิดจาก response ที่บันทึกจริง ทั้งในรอบแรกและ repair หนึ่งรอบ
11. ทดสอบ auth ผิด, quota/rate limit, timeout, คำตอบไม่ใช่ JSON, code block ไม่ครบ และ cancel ระหว่างรอคำตอบ; ห้ามรายงาน task success จากคำตอบที่ไม่ครบ

### M3 — Discord, Router และ Architect

1. Level 0 ใช้คำสั่งที่ตรงและตรวจสอบได้ เช่น map, format, test ตาม policy; ไม่ใช้โมเดลเมื่อไม่จำเป็น
2. Level 1 รับงานขอบเขตเล็ก; Level 2 รับงานหลายไฟล์ งาน API/schema/dependency/security หรือบริบทไม่แน่นอน จำนวนมากกว่า 3 ไฟล์เป็นสัญญาณหนึ่ง ไม่ใช่เกณฑ์เดียว
3. หากขอบเขตงานจริงขยายหลังสร้าง patch ต้องจัดประเภทใหม่; `apw fx` ปฏิเสธงานที่เกินขอบเขตและเสนอให้ใช้ plan
4. Planner สร้าง JSON ที่ตรวจ schema, dependency ที่มีอยู่จริง, cycle, file scope, verification profile และ acceptance criteria ได้ ก่อนแปลงเป็น Markdown ให้ผู้ใช้ตรวจ
5. รัน DAG แบบ sequential ก่อนเพื่อให้ rollback และเหตุการณ์ตรวจสอบง่าย; เพิ่ม parallel execution เมื่อมี conflict analysis และความจำเป็นจากข้อมูลใช้งาน
6. งานที่ล้มเหลวไม่ปล่อย dependent steps ทำต่อ; ทั้ง task ต้องผ่าน final verification ก่อนส่งมอบ และคืนฐานทั้ง task เมื่อปิดงานแบบ failed
7. การรันแผนเดิมต้องตรวจ revision ใหม่ หากเปลี่ยนต้อง revalidate หรือ replan ตามผลกระทบ
8. เพิ่ม Discord adapter สำหรับ DM/mention/channel/thread ตาม configuration พร้อม reconnect และ bot-loop prevention
9. ทดสอบ Telegram กับ Discord พร้อมกันด้วยคนละ session โดยไม่รั่ว context, artifact หรือสิทธิ์ของเจ้าของ
10. รองรับ provider/model preferences แยกต่อ session และบทบาท reflex/architect; architect ใช้โมเดลใหญ่ที่ตอบช้ากว่าได้ โดยไม่บังคับ latency เท่า reflex

### M4 — Daemon และ Gateway Reliability

1. เพิ่ม local RPC ให้ CLI คุย application service ของ daemon ที่เริ่มตั้งแต่ M1 โดยไม่ทำสำเนา core logic
2. เริ่ม protocol จาก `health`, `task.create`, `task.get`, `task.cancel`, `task.events`, `repo.map` และ `plan.get`
3. เพิ่ม protocol version, structured errors, request ID และ event sequence; reconnect ต้องไม่สร้าง task ซ้ำจาก request เดิม
4. watcher ทำ debounce, incremental invalidation และ rescan เมื่อพลาด event; index ต้องรู้ revision ที่ตนอ่านอยู่
5. จำกัดหนึ่งงานแก้ไขต่อ workspace ในรุ่นแรก และ serialize การเขียน state
6. restart แล้วทำเครื่องหมายงานที่ยังไม่จบเป็น interrupted; ยังไม่สั่งรัน tool ต่ออัตโนมัติจนกว่าจะตรวจ checkpoint และ policy
7. ทดสอบ start/status/stop, lock file, stale process metadata, graceful shutdown และ retention ของ log/worktree
8. เก็บ checkpoint ก่อน/หลังเรียก provider; หาก crash หลังส่ง request แต่ยังไม่ได้ผล ให้ถือว่าไม่ทราบผลและตัดสินใจ retry ตาม budget/policy ไม่ส่งซ้ำทันทีแบบเดา
9. แยก provider contract tests ที่ใช้ recorded responses จาก live smoke tests ที่ต้องมี API key และไม่รันบน public CI; ตั้ง provider unavailable เมื่อ provider ใช้ไม่ได้
10. ทดสอบ disconnect/reconnect ของทั้งสอง platform, outbox retry, rate limit และการตอบกลับถูก thread หลัง restart

### M5 — Memory และ Skills

1. บันทึก failure signature, project identity, version/context, วิธีที่ลอง และ verification result; ต้องระบุได้ว่าข้อสรุปมาจาก task ใด
2. เริ่มค้น memory ด้วย project + normalized error signature ก่อนเพิ่ม vector search เมื่อมีหลักฐานว่าจำเป็น
3. กรอง secret และข้อมูลที่ไม่ควรเก็บ; มีคำสั่ง inspect/delete และตั้ง retention ได้
4. เก็บ skill manifest แบบมี version, hash, source task, inputs, required permissions และ verification fixture
5. การสกัด skill สร้างเป็น candidate ก่อน; การเปิดใช้ต้องผ่านการตรวจและ project policy สคริปต์จากโมเดลไม่รันทันทีเพราะเพียงแค่ถูกบันทึกแล้ว
6. ถ้ายังไม่มี sandbox ที่บังคับ filesystem/network/process ได้จริง ให้ v1 ใช้เฉพาะ built-in tools หรือ trusted scripts ที่เปิดใช้ชัดเจน และเลื่อน arbitrary generated-skill execution ออกไป
7. ทำคู่มือติดตั้ง config, troubleshooting, data locations และ recovery พร้อมทดสอบ migration ฐานข้อมูลก่อนปล่อยรุ่น

## 6. คำสั่ง CLI ที่เสนอ

| คำสั่ง | ผลที่ผู้ใช้ได้รับ |
| --- | --- |
| `apw version` | แสดงเวอร์ชัน (มีแล้ว) |
| `apw verify [dir]` | รัน Reflex: fmt → vet → build → test และแสดง diagnostics (มีแล้ว) |
| `apw symbols <file.go>` | แสดง symbol จาก AST ผ่าน Surgeon (มีแล้ว) |
| `apw "<งาน>" --preview` | วิเคราะห์และสร้าง diff/plan โดยไม่รัน patch หรือ verification ที่ execute โค้ดของ repo |
| `apw "<งาน>" --execute` | ทำงานใน worktree ตาม policy พร้อมรายงานผล |
| `apw fx "<งาน>"` | ทางลัดสำหรับงานเล็ก; ยังต้องผ่าน patch และ verification gates |
| `apw plan "<งาน>"` | สร้าง `plan.json` ที่ตรวจด้วยเครื่องได้และ `apw-plan.md` ที่อ่านได้ |
| `apw apply <plan-id>` | ตรวจ revision/policy แล้วรันแผนที่เลือกใน worktree |
| `apw map` | แสดง symbols และความสัมพันธ์ที่ index รองรับ |
| `apw task show <id>` / `apw task cancel <id>` | ดูผลหรือยกเลิกงาน |
| `apw daemon start` / `status` / `stop` | จัดการ daemon ในเครื่อง |
| `apw channel setup telegram` / `discord` | ตั้งค่า bot, allowlist และ project bindings |
| `apw channel status` | ดู connection, session และ delivery failure โดยไม่เปิด token |
| `apw provider list` / `doctor` | ดู capability และตรวจความพร้อมของ API/local provider (เช่น key ตั้งไว้หรือไม่, endpoint ตอบหรือไม่) |
| `apw skill list` / `new` / `inspect` / `enable` | จัดการ skill และตรวจสิทธิ์ก่อนเปิดใช้ |

ค่าเริ่มต้นสำหรับคำสั่งแก้ไขที่ไม่ระบุ mode ให้เป็น preview ในช่วง MVP ทั้งใน CLI และแชต เจ้าของสามารถเปิด execute policy สำหรับ workspace ที่เลือกได้ ส่วน interactive CLI REPL (`apw` เปล่า) เพิ่มหลัง flow ผ่านแชตเสถียร

ตัวอย่างประสบการณ์ใช้งานเป้าหมาย (รูปแบบคำสั่งจริงขึ้นกับ platform adapter):

```text
ผู้ใช้: /project apw-agent
ผู้ใช้: /model api <ชื่อโมเดล>
ผู้ใช้: ช่วยวางแผนเพิ่ม timeout ให้ process runner
apw: รับงาน T-42 กำลังวางแผนด้วย provider api ...
apw: แผนพร้อมแล้ว [ดูแผน] [รันใน worktree] [ยกเลิก]
ผู้ใช้: [รันใน worktree]
apw: แก้ 2 ไฟล์ ผ่าน fmt/vet/build/test แล้ว [diff] [report]
```

แต่ละ task เก็บ `plan.json` ถ้ามี, patch/diff, events และ verification report ไว้ใต้ task directory ที่ config กำหนด ข้อมูลเครื่องไม่ commit กลับ repository โดยอัตโนมัติ

## 7. งานสัปดาห์แรก

| วัน | งาน | สิ่งที่ตรวจดูได้เมื่อจบวัน |
| --- | --- | --- |
| 1 | ~~Go skeleton, Triad engine และ CI~~ (ทำแล้ว); เพิ่ม config และ mock provider | `apw verify .` ผ่านบน CI; mock provider ใช้ในเทสต์ Architect ได้ |
| 2 | Provider round trip: API provider ส่ง planning prompt → `ParsePlan` + `Validate` | หลักฐาน request/response ตรงกัน, usage และ model observation ถูกบันทึก |
| 3 | Telegram bot proof of concept, config และ owner/session binding | bot รับและตอบข้อความใน private chat ทดสอบได้ |
| 4 | SQLite/Windows spike, mock task lifecycle และ runner/worktree prototype | ข้อสรุป storage และ demo mock ที่ไม่แก้ source workspace |
| 5 | รวม spike findings และทบทวน scope/เวลา | ระบุ provider/gateway feasibility จริง พร้อม backlog M1 และความเสี่ยงที่ต้องแก้ |

สัปดาห์แรกเน้นพิสูจน์สอง dependency สำคัญคือแชตและ model provider ส่วน surgeon/reflex มีโค้ดขั้นต้นแล้วแต่ยังไม่ทำงานใน worktree หากติดเรื่อง credential, rate limit หรือ bot ต้องแจ้งตั้งแต่จุดนี้ ไม่รอถึงปลายโครงการ และไม่ถือว่า mock ทดแทน provider acceptance ได้

## 8. การทดสอบและเกณฑ์ release

| กลุ่ม | กรณีสำคัญ |
| --- | --- |
| Patch correctness | symbol ซ้ำ, hash เปลี่ยน, expected text ไม่ตรงหรือว่าง, patch ทับกัน, CRLF, Unicode, parse ไม่ผ่าน และ rollback หลายไฟล์ |
| Workspace safety | path escape, symlink/reparse point, dirty repo, task ชนกัน และ base เปลี่ยนก่อนนำผลกลับ |
| Provider | auth ผิด, JSON ผิด, field เกิน, response ไม่ครบ, timeout, rate limit/quota, token budget หมด, usage ไม่ถูกรายงาน และ capability ไม่ตรง |
| Chat gateway | event ซ้ำ, reconnect, rate limit, owner ที่ไม่ได้รับสิทธิ์, task approval replay, wrong thread, context ปะปน และ outbox ส่งไม่สำเร็จ |
| Runner | command ไม่อนุญาต, output มาก, exit code ผิด, deadline และ cancel ที่มี child process |
| Planner | cycle, dependency หาย, scope เปลี่ยน, step ตก, stale plan, target นอก workspace และ final verification ล้มเหลว |
| Daemon/data | process crash, restart, duplicate request, watcher event หาย, migration และ log redaction |
| Skills | manifest/hash ผิด, permission ไม่พอ, candidate ยังไม่ enable และ script ที่ขอออกนอกขอบเขต |

เกณฑ์ก่อนออก v1:

- fixture suite ผ่านทั้งหมดด้วย mock โดยไม่ต้องเรียก external API; มี opt-in integration test ของ provider ที่ประกาศรองรับ
- งานตัวอย่าง Go ครอบคลุมการแก้เฉพาะจุด เพิ่มฟังก์ชัน และแก้หลายไฟล์ พร้อม task ที่ตั้งใจให้ล้มเหลวและยกเลิก
- ทุก task ที่รายงาน success มี diff และ verification report ครบ; การข้าม test เพราะไม่มีหรือไม่อนุญาตต้องแสดงเป็น incomplete/needs_review ตาม policy
- ทดสอบบน Windows จากสภาพแวดล้อมติดตั้งใหม่ และระบุ platform/language/provider ที่รองรับจริงใน release notes
- ไม่พบการแก้ไฟล์ผู้ใช้โดยไม่ตั้งใจใน acceptance suite; baseline ที่เสียอยู่เดิมต้องไม่ถูกตีความว่า patch ผ่าน
- ผู้ใช้ตรวจ artifact นำผลไป review/merge และลบข้อมูลของ task ได้
- Telegram และ Discord ทำงานจริงอย่างน้อยแพลตฟอร์มละหนึ่ง end-to-end acceptance run; ไม่มีการส่งผลหรือ context ข้าม session ผิดคน
- provider ที่ประกาศรองรับผ่าน live acceptance แบบ opt-in และนำ response ไปสร้าง patch/verification ได้จริง; model observation มาจาก response จริง
- auth หมดอายุ, quota, disconnect และ provider ไม่ตอบ ต้องให้สถานะที่เข้าใจได้ และไม่มีการ apply คำตอบที่ยังไม่ครบหรือถูกยกเลิก

## 9. ตัวชี้วัดและความเสี่ยง

เป้าหมาย router `<10ms`, index update `<5ms`, reflex `100–300ms` และ fast-path `75%` จากสเปกเดิมให้ถือเป็นสมมติฐานสำหรับ benchmark ยังไม่ใช้เป็นคำรับประกันหรือ release gate

ต้องวัด p50/p95 แยกเวลา queue, routing, context/index, model, patch และ verification พร้อมระบุเครื่อง ขนาด repo จำนวนไฟล์ที่เปลี่ยน model/provider และ warm/cold state ค่า routing ไม่รวมเวลาเรียก LLM ส่วน reflex ต้องรายงานทั้งเวลา model และเวลาจบงาน

แยก benchmark ของ API กับ local provider; token/cost ที่ provider ไม่รายงานต้องเป็น unknown

เก็บเพิ่มเติม: อัตรางานผ่านครั้งแรก, อัตราผ่านหลัง repair, จำนวน escalation, stale-patch rejection, token/cost ต่อ task, cancel latency และ workspace corruption incidents เป้าหมายของข้อสุดท้ายคือศูนย์

| ความเสี่ยง | วิธีลดความเสี่ยง | จุดตัดสินใจ |
| --- | --- | --- |
| โมเดลเล็กแก้โค้ดไม่ผ่านหรือไม่เร็วตามเป้า | benchmark งานตัวอย่างและ fallback ไป planner/provider ที่เหมาะสมภายใน budget | จบ M2 และก่อนเปิด router ใน M3 |
| index ไม่ตรงกับไฟล์จริง | ใช้ revision/hash ตรวจซ้ำก่อน patch และ re-index เมื่อเปลี่ยน | M1–M3 |
| worktree ถูกเข้าใจว่าเป็น sandbox | แยก trust/policy ของ process; จำกัด scope การรันและยังไม่เปิด remote | M1 และ M4 |
| ระบบใหญ่เกินกำลัง | ยึด Go-first, Telegram-first และ milestone gates; เพิ่ม Discord ตาม M3 และเลื่อน surface อื่นหลัง Beta | ทบทวนทุกสัปดาห์ |
| build/test ช้าเกินไป | วัดก่อน แล้วใช้ targeted checks ระหว่าง repair โดยคง final gate | หลังได้ข้อมูล M2 |
| skill ขยายสิทธิ์หรือพกข้อมูลลับ | candidate/enable, hash, permission, secret filtering และเลื่อน execution ที่ยังจำกัดไม่ได้ | M5 |
| ค่าใช้จ่าย API เกินคาดหรือโดน rate limit | budget ต่อ task, usage tracking, backoff และ local provider เป็นทางเลือก | M0 spike และทุก milestone |
| โมเดลถูกเลิกให้บริการหรือเปลี่ยนพฤติกรรม | ชื่อโมเดลอยู่ใน config, contract tests จาก recorded responses และ fallback ตาม policy | M2 และทุก release |
| prompt injection ทำให้แผนแตะไฟล์นอกขอบเขต | `ExecutionPlan.Validate`, path policy ใน Surgeon และ worktree | M0 (validate) และ M1 (path policy) |
| ข้อความซ้ำหรือส่งผลผิดห้อง | durable inbox/outbox, idempotency, conversation ownership และ thread mapping | M1/M3 ก่อนเปิดใช้งานจริง |

## 10. จุดเริ่ม implementation

ต่อจาก **M0** ที่มี skeleton, Triad engine และ CI แล้ว: เพิ่ม config + mock provider, พิสูจน์ API provider round trip และ Telegram bot ตามงานสัปดาห์แรก แล้วทบทวนผลก่อนขึ้น M1 โดยใช้ Telegram-first เป็นสมมติฐานที่เปลี่ยนเป็น Discord-first ได้ตามความสะดวกของเจ้าของบัญชี

ลำดับ dependency ของการส่งมอบคือ:

`Provider/Chat feasibility → Safe core + Daemon/Gateway → Telegram + Model MVP → Discord + Dual-brain → Reliability → Memory/Skills → Release`

เมื่อจบแต่ละ milestone ให้เก็บ demo, ผลทดสอบ, ข้อจำกัดที่ยังเหลือ และประมาณการเวลาใหม่ แล้วจึงเลือกว่าจะเดินหน้าหรือแก้ gap ที่ขวางเกณฑ์ส่งมอบ
