# Handoff — งานที่เหลือของ M0 (เขียน 2026-10-16)

เอกสารนี้ใช้ส่งต่องานให้คนหรือ agent ที่มาทำต่อ ทำตามลำดับ T1 → T8 ทุกงานเขียนแยกกันได้และ commit แยกกันได้ ก่อนเริ่มให้อ่าน [implementation-plan.md](implementation-plan.md) §3.0, §3.2 และ §5 M0

## สถานะตอนส่งต่อ

- branch `master` · commits: `0be36ba` skeleton → `cbd5d9f` Triad engine → `f0afb91` docs/README/CI
- ทำแล้ว: `internal/architect` (`ExecutionPlan`, `ParsePlan`, `Validate`), `internal/surgeon` (patch ที่ตรวจ hash, AST symbols), `internal/reflex` (fmt/vet/build/test + diagnostics), CLI `version`/`verify`/`symbols`, CI
- ยังไม่มี remote ไม่มี provider จริง ไม่มี config ไม่มี gateway
- `go run ./cmd/apw verify .` ผ่าน

## กติกาสำหรับผู้ทำต่อ

1. ก่อน commit ทุกครั้งต้องรัน `go run ./cmd/apw verify .` ให้ผ่าน และ `go mod tidy -diff` ต้องไม่มี diff
2. ทำ 1 งาน ต่อ 1 commit ใช้ข้อความแบบ `feat(m0): ...`
3. ใช้ stdlib เท่านั้น ยกเว้นที่ระบุไว้ในแต่ละงาน
4. ห้ามเก็บ secret ใน config หรือ commit ให้อ่าน API key / bot token จาก environment variable เท่านั้น
5. เทสต์ที่เรียก network จริงต้อง opt-in ด้วย `APW_LIVE_TEST=1` ถ้าไม่ได้ตั้งต้อง `t.Skip` เพื่อไม่ให้ CI เรียกของจริง
6. ทำงานเสร็จแล้วให้ขีดฆ่าข้อนั้นใน implementation-plan.md §5 M0 และติ๊กในตารางท้ายเอกสารนี้

---

## T1 — Config (`internal/config`)

ใช้ JSON (stdlib) ไฟล์ชื่อ `apw.json` และเพิ่ม `apw.json` ใน `.gitignore` แล้ว commit ไฟล์ `apw.example.json` แทน

```go
type ProviderType string // "api" | "local" | "mock"

type ProviderConfig struct {
	Type       ProviderType `json:"type"`
	Endpoint   string       `json:"endpoint,omitempty"`
	Model      string       `json:"model"`
	APIKeyEnv  string       `json:"api_key_env,omitempty"` // ชื่อ env var ไม่ใช่ตัว key
	TimeoutSec int          `json:"timeout_sec,omitempty"` // default 120
}

type Config struct {
	Workspace string                    `json:"workspace"`
	Providers map[string]ProviderConfig `json:"providers"`
	Roles     struct {
		Architect string `json:"architect"` // key ใน Providers
		Reflex    string `json:"reflex"`
	} `json:"roles"`
	Budget struct {
		MaxRepairs     int `json:"max_repairs"`      // default 3
		TaskTimeoutSec int `json:"task_timeout_sec"` // default 600
	} `json:"budget"`
}

func Load(path string) (*Config, error) // อ่านไฟล์ ใส่ default แล้วเรียก Validate
func (c *Config) Validate() error       // role ต้องชี้ provider ที่มีอยู่, type ต้องถูก, type api ต้องมี api_key_env
func (p ProviderConfig) APIKey() (string, error) // os.Getenv(APIKeyEnv); ถ้าว่างให้คืน error ที่บอกชื่อ env
```

**เสร็จเมื่อ:** มีเทสต์ครอบ default, role ชี้ provider ที่ไม่มีอยู่, type ผิด, api ไม่มี key env และ `DisallowUnknownFields` ปฏิเสธ field ที่สะกดผิด

## T2 — Mock provider (`internal/provider/mock`)

```go
type Provider struct {
	Responses []string           // คืนตามลำดับ ถ้าหมดแล้วให้ error
	Requests  []core.ModelRequest // บันทึกทุก request ไว้ให้เทสต์ตรวจ
}
// implements provider.Provider; Name() = "mock"
```

**เสร็จเมื่อ:** มีเทสต์ใน `internal/architect` เรียก `Architect.Plan` ผ่าน mock แล้วได้แผนที่ผ่าน `Validate` และมีเทสต์กรณี mock คืน `target_file: "../x"` ต้องได้ error

## T3 — Reflex repair loop (`internal/reflex`)

```go
// Fixer ได้รับ diagnostics ที่ format แล้ว และต้องแก้ไฟล์ (ปกติผ่าน Surgeon)
type Fixer func(ctx context.Context, diagnostics string) error

type Attempt struct {
	Number int
	Result *VerificationResult
}

// Run: verify → ถ้าไม่ผ่านให้เรียก fix แล้ว verify ใหม่ ทำซ้ำได้สูงสุด MaxRetries รอบ
// คืนผลทุกรอบไว้ทำ report; หยุดทันทีถ้า ctx ถูกยกเลิกหรือ fix คืน error
func (e *ReflexEngine) Run(ctx context.Context, fix Fixer) ([]Attempt, error)
```

**เสร็จเมื่อ:** มีเทสต์ที่สร้าง Go module ชั่วคราว (`t.TempDir()`, `go.mod` + `main.go` ที่ compile ไม่ผ่าน) แล้ว fixer แก้ด้วย `surgeon.ApplyPatch` → ผ่านในรอบที่ 2 และมีเทสต์ fixer ที่แก้ไม่ได้ → หยุดที่ `MaxRetries` และคืนผลทุกรอบ

## T4 — API provider (`internal/provider/api`)

ใช้ Anthropic Messages API ผ่าน `net/http` (ไม่ใช้ SDK)

- `POST {endpoint}/v1/messages` โดย endpoint ค่าเริ่มต้นคือ `https://api.anthropic.com`
- header: `x-api-key`, `anthropic-version: 2023-06-01`, `content-type: application/json`
- body: `{"model": cfg.Model, "max_tokens": 8192, "messages": [{"role":"user","content": req.Prompt}]}`
- อ่าน `content[].text` (เอาเฉพาะ type `text` มาต่อกัน), `model` → `ModelObserved`, `stop_reason` (`end_turn` → `Completed=true`; `max_tokens` → `Completed=false`), `usage.input_tokens` / `usage.output_tokens`
- เพิ่ม `Usage *Usage` ใน `core.ModelResponse` โดย `type Usage struct{ InputTokens, OutputTokens int }` ถ้าไม่รู้ค่าให้เป็น nil ไม่ใช่ 0
- retry เฉพาะ 429, 5xx และ network timeout: สูงสุด 3 ครั้ง แบบ exponential backoff + jitter ถ้ามี header `retry-after` ให้ใช้ค่านั้น ส่วน 401/403 ห้าม retry และต้องคืน error ที่ห่อ sentinel `ErrUnauthorized`
- ทุก request ใช้ `ctx` และ timeout จาก config
- ห้าม log หรือใส่ API key ลงใน error message

**เสร็จเมื่อ:** มีเทสต์ด้วย `httptest.Server` ครอบกรณี success, 429 → success, 401, `max_tokens` และ ctx cancel พร้อม live test แบบ opt-in หนึ่งตัว

## T5 — CLI `apw plan "<งาน>"`

`cmd/apw/main.go`: โหลด config (`--config`, ค่าเริ่มต้น `./apw.json`) → สร้าง provider ตาม `roles.architect` → `architect.Plan` → แสดง JSON แบบ indent และบันทึกที่ `.apw/plans/<plan_id>.json` (`.apw/` อยู่ใน `.gitignore` แล้ว)

- `codebaseOverview` รุ่นแรกให้ใช้รายการ symbol จาก `surgeon.InspectGoSymbols` ของทุกไฟล์ `.go` ใน workspace (ข้าม `vendor/`, `.git/`, `testdata/`) จำกัดไม่เกิน 20 KB
- เพิ่ม factory `provider.New(cfg config.ProviderConfig) (Provider, error)` ไว้ใน package ใหม่ `internal/provider/factory` เพื่อเลี่ยง import cycle
- แยก logic ออกจาก `main()` เป็นฟังก์ชัน `run(args []string, stdout, stderr io.Writer) int` เพื่อให้เทสต์ได้

**เสร็จเมื่อ:** มีเทสต์ `run` ที่ใช้ config แบบ mock ได้ exit 0 และพบไฟล์แผนบนดิสก์ และอัปเดต help กับ README

## T6 — Local provider (`internal/provider/local`) — ทำหรือข้ามก็ได้

ใช้ OpenAI-compatible `POST {endpoint}/v1/chat/completions` (Ollama, llama.cpp server) แบบไม่มี key อ่าน `choices[0].message.content`, `model`, `usage.prompt_tokens` / `completion_tokens` ส่วนรูปแบบเทสต์ใช้แบบเดียวกับ T4

## T7 — Telegram PoC (`internal/channels/telegram`)

- long polling `getUpdates?timeout=30&offset=<last+1>` และ `sendMessage` ผ่าน `net/http`
- ใช้ token จาก env `APW_TELEGRAM_TOKEN` และ allowlist `APW_TELEGRAM_ALLOW` (user ID คั่นด้วย comma) โดยข้อความจากคนที่ไม่อยู่ใน allowlist ให้ทิ้งเงียบๆ
- PoC: ตอบ `/status` → `apw <version> ok` และตอบข้อความอื่นด้วย echo ส่วนการผูกกับ task จะทำใน M1
- รันจาก `apwd` เมื่อตั้ง env ไว้ครบ และหยุดอย่างเรียบร้อยเมื่อได้ SIGINT

**เสร็จเมื่อ:** มีเทสต์ด้วย `httptest.Server` ครอบการเลื่อน offset, allowlist และ shutdown แล้วทดสอบมือกับ bot จริงหนึ่งครั้ง จดผลไว้ในเอกสารนี้

## T8 — SQLite spike (ตัดสินใจ ไม่ต้อง production)

- ทดลอง `modernc.org/sqlite` (pure Go ไม่ต้องใช้ cgo บน Windows) โดยเป็น dependency ใหม่ตัวเดียวที่อนุญาต
- เขียน `docs/adr/0001-sqlite-driver.md`: ทดสอบ WAL, ให้ 2 goroutine เขียนพร้อมกัน, ขนาด binary ที่เพิ่ม และเวลา build บน Windows แล้วสรุปว่าเลือกหรือไม่เลือกเพราะอะไร
- spike ใส่ใน `internal/store` พร้อมตาราง `tasks` เพียงตารางเดียว

---

## งานเก็บกวาด (เจ้าของตัดสินใจ)

- [ ] เลือกว่าจะใช้ branch `main` หรือ `master` แล้วเพิ่ม remote และ push เพื่อให้ CI ได้รันครั้งแรก
- [ ] ลบ `apw.exe` ที่ root (ถูก ignore อยู่แล้ว เป็นแค่ไฟล์ค้าง)
- [ ] `internal/config`, `gateway`, `session`, `task` ตอนนี้เป็นโฟลเดอร์ว่าง ไม่ได้อยู่ใน git T1 จะใช้ `config`

## ติดตามงาน

| งาน | สถานะ | commit |
| --- | --- | --- |
| T1 Config | ☑ | `198e97b` |
| T2 Mock provider | ☑ | `180ba42` |
| T3 Reflex repair loop | ☑ | `41cfb59` |
| T4 API provider | ☑ | `feat(m0): add Anthropic Messages API provider (T4)` |
| T5 CLI plan | ☐ | |
| T6 Local provider (optional) | ☐ | |
| T7 Telegram PoC | ☐ | |
| T8 SQLite spike | ☐ | |

## Prompt สำหรับเริ่ม session ใหม่

```text
อ่าน docs/handoff.md แล้วทำงาน T ถัดไปที่ยังไม่ติ๊ก ตามกติกาในเอกสาร
ทำทีละงาน รัน `go run ./cmd/apw verify .` ให้ผ่าน แล้ว commit และอัปเดตตารางติดตามงาน
```
