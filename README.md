# apw — Apiwat Agent

Coding agent เขียนด้วย Go ที่วางแผน แก้โค้ดเฉพาะจุด และตรวจผลด้วย compiler/test อัตโนมัติ เป้าหมายคือสั่งงานผ่าน Telegram/Discord และใช้โมเดลผ่าน API หรือ local

> **สถานะ:** `0.1.0-alpha` — milestone M0 ยังไม่มี model provider จริง, gateway หรือ daemon ที่ใช้งานได้ ดูแผนเต็มที่ [docs/implementation-plan.md](docs/implementation-plan.md)

## Triad Engine

| ส่วน | Package | ทำอะไร |
| --- | --- | --- |
| **Architect** | `internal/architect` | สร้าง planning prompt, parse คำตอบโมเดลเป็น `ExecutionPlan` และตรวจด้วย `Validate()` (action ต้องรู้จัก, `target_file` ต้องอยู่ใน workspace) |
| **Surgeon** | `internal/surgeon` | แก้ไฟล์แบบ read-before-write ตรวจ SHA-256 กัน drift แทนที่เฉพาะจุดที่ match ครั้งเดียว รักษา CRLF/LF และอ่าน symbol จาก Go AST |
| **Reflex** | `internal/reflex` | รัน `gofmt -l` → `go vet` → `go build` → `go test` แล้วแปลง output เป็น diagnostic ที่มีไฟล์/บรรทัด/คอลัมน์ |

## ความต้องการ

- Go 1.27+
- พัฒนาและทดสอบบน Windows เป็นหลัก (Linux ผ่าน CI)

## Build

```bash
go build -o bin/ ./cmd/...
```

ได้ `bin/apw` (CLI) และ `bin/apwd` (daemon — ตอนนี้แค่รอ Ctrl+C)

## ใช้งาน

```bash
go run ./cmd/apw version
```

ตรวจ repository Go ด้วย Reflex (ค่าเริ่มต้นคือ directory ปัจจุบัน; exit 1 เมื่อไม่ผ่าน):

```bash
go run ./cmd/apw verify .
```

ดู symbol ในไฟล์ Go:

```bash
go run ./cmd/apw symbols internal/surgeon/surgeon.go
```

## พัฒนา

ชุดตรวจเดียวกับ CI:

```bash
gofmt -l .
```

```bash
go mod tidy -diff
```

```bash
go vet ./...
```

```bash
go test -race ./...
```

หรือใช้ apw ตรวจตัวเอง:

```bash
go run ./cmd/apw verify .
```

## โครงสร้าง

```text
cmd/apw/              CLI
cmd/apwd/             daemon (stub)
internal/core/        domain models: Task, ModelRequest, ModelResponse
internal/provider/    Provider interface (API / local)
internal/architect/   planning + plan validation
internal/surgeon/     safe file patching + AST symbols
internal/reflex/      verification pipeline + diagnostics
docs/                 แผนพัฒนาและบันทึกการตรวจ
```

## เอกสาร

- [แผนพัฒนา](docs/implementation-plan.md)
- [ผลตรวจ repo และการแก้ไข](docs/review-fixes.md)
