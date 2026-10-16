# แผนแก้ไขจากการตรวจ repo (2026-10-16)

สถานะ: **แก้ไขครบทุกข้อแล้ว** — ผ่าน `gofmt`, `go mod tidy -diff`, `go vet`, `go build`, `go test`, และ `apw verify .`

## สภาพปัจจุบัน

| ขั้น | ผล |
|---|---|
| `go vet ./...` / `go build ./...` | ❌ `internal\provider\browser\gemini.go:92:25: undefined: rod.DefaultMouseButton` |
| `go test ./...` | architect, reflex, surgeon ผ่าน · `provider/browser` build failed |
| `gofmt -l .` | `internal/architect/architect.go`, `internal/core/models.go`, `internal/surgeon/surgeon.go` |
| `go mod tidy -diff` | `go-rod/rod` ถูกมาร์ก `// indirect` ทั้งที่ import ตรง, `go.sum` ขาด `ysmood/gop`, `ysmood/gotrace` |
| `apw verify .` | FAIL ที่ stage `vet` และ diagnostic มีชื่อไฟล์ผิด (ดูข้อ 2) |

## ลำดับการแก้ที่แนะนำ

1. ข้อ 1 (ลบ Gemini provider) + `gofmt -w .` + `go mod tidy` → ให้ `apw verify .` ผ่านก่อน
2. ข้อ 2, 9, 10 (Reflex) → diagnostic ถูกต้อง พร้อมใช้ในลูป self-heal
3. ข้อ 6, 7, 8 (Surgeon) → ความปลอดภัยของการแก้ไฟล์
4. ข้อ 11, 12 (CLI, Architect validation)

> ข้อ 3, 4, 5, 13 (บั๊กใน Gemini browser provider) **ตัดออก** — ไม่ทำงาน Gemini ต่อแล้ว

---

## Build / Toolchain

### 1. Build พัง → ลบ Gemini browser provider

**ไฟล์:** `internal/provider/browser/gemini.go` (ยังไม่เคย commit)

build พังเพราะ `rod.DefaultMouseButton` ไม่มีจริงใน rod v0.116.2 และไฟล์นี้เป็นจุดเดียวที่ใช้ rod เมื่อไม่ทำงาน Gemini ต่อแล้ว ให้ลบทั้ง package แทนการแก้

> **คำเตือน:** โฟลเดอร์นี้เป็น untracked — ลบแล้วกู้จาก git ไม่ได้ ถ้าอยากเก็บไว้อ้างอิง ให้ commit แยกหรือย้ายออกนอก repo ก่อน

```bash
rm -r internal/provider/browser
```

```bash
go mod tidy
```

```bash
gofmt -w .
```

`go mod tidy` จะเอา `go-rod/rod` และ `ysmood/*` ออกจาก `go.mod`/`go.sum` เอง

เก็บกวาดที่เหลือ:

- `internal/architect/architect.go:138` — `Model: "gemini-argon"` ฮาร์ดโค้ด → ย้ายไปเป็นฟิลด์ของ `Architect` (เช่น `Model string` ใน `NewArchitect`) หรือปล่อยว่างให้ provider เลือก
- `internal/provider/provider.go:8` — comment `Browser/Argon` → แก้เป็นแค่ `API, Local`
- `internal/core/models.go` — `StatusWaitingForLogin` ใช้กับ browser provider เท่านั้น → ลบได้ถ้าไม่มีแผนใช้
- `docs/implementation-plan.md` — revision 5 ยังอิง Gemini Argon/go-rod/CDP/SingletonLock (M0 ข้อ 7–10) → อัปเดตแผนให้ตรงทิศทางใหม่

**ตรวจ:** `go build ./...` และ `go vet ./...` ไม่มี error, `gofmt -l .` ว่าง, `grep -rn rod go.mod` ไม่เจอ

---

## Reflex (`internal/reflex/reflex.go`)

### 2. Regex แยกชื่อไฟล์ผิดเมื่อมี prefix `vet.exe:` หรือ indent

**ไฟล์:** `reflex.go:47`

ผลจริงตอนรัน `apw verify .`:

```
- `vet.exe: internal\provider\browser\gemini.go:92:25`: undefined: rod.DefaultMouseButton
```

และ output ของ `go test` มี indent เช่น `    foo_test.go:12: msg` ทำให้ File มีช่องว่างนำหน้า

```go
var errRegex = regexp.MustCompile(`(?m)^\s*(?:vet(?:\.exe)?:\s+)?(\S.*?\.go):(\d+):(?:(\d+):)?\s*(.+)$`)
```

**เทสต์เพิ่ม** ใน `reflex_test.go`:

```go
func TestParseGoErrors_Prefixes(t *testing.T) {
	raw := "vet.exe: internal\\a\\b.go:92:25: undefined: X\n" +
		"    foo_test.go:12: expected 1, got 2"
	diags := ParseGoErrors("vet", raw)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}
	if diags[0].File != `internal\a\b.go` {
		t.Errorf("diag[0].File = %q", diags[0].File)
	}
	if diags[1].File != "foo_test.go" || diags[1].Line != 12 {
		t.Errorf("diag[1] = %+v", diags[1])
	}
}
```

### 9. `Verify` ทิ้ง `err` — FAIL แบบไม่มีเหตุผล

**ไฟล์:** `reflex.go:103-136`

กรณี `go` ไม่อยู่ใน PATH หรือโดน timeout kill → `out` ว่าง → `Diagnostics` ว่าง ผู้ใช้/โมเดลไม่รู้สาเหตุ

แก้ `RunCommand` ให้บอก timeout ชัด:

```go
err := cmd.Run()
if err != nil && cmdCtx.Err() == context.DeadlineExceeded {
	err = fmt.Errorf("%s %s: timed out after %v: %w", name, strings.Join(args, " "), v.Timeout, err)
}
return outBuf.String(), err
```

แยก helper เพื่อลดโค้ดซ้ำ 3 จุดใน `Verify`:

```go
func failed(stage, out string, err error, start time.Time) *VerificationResult {
	diags := ParseGoErrors(stage, out)
	if len(diags) == 0 && err != nil {
		diags = []Diagnostic{{Stage: stage, Message: err.Error(), RawError: err.Error()}}
	}
	return &VerificationResult{
		Passed:      false,
		FailedStage: stage,
		Diagnostics: diags,
		RawOutput:   out,
		Duration:    time.Since(start),
	}
}
```

### 10. ไม่มี stage `gofmt` ตามที่แผนระบุ

แผน §3.0 ระบุ `gofmt → go vet → go build → go test` แต่โค้ดเริ่มที่ vet

`gofmt -l` คืน exit 0 แม้มีไฟล์ไม่ผ่าน ต้องเช็ก output ด้วย:

```go
// Stage 0: gofmt
out, err := v.RunCommand(ctx, "gofmt", "-l", ".")
if err != nil {
	return failed("fmt", out, err, start)
}
if files := strings.TrimSpace(out); files != "" {
	var diags []Diagnostic
	for _, f := range strings.Split(files, "\n") {
		f = strings.TrimSpace(f)
		diags = append(diags, Diagnostic{Stage: "fmt", File: f, Message: "file is not gofmt-formatted", RawError: f})
	}
	return &VerificationResult{FailedStage: "fmt", Diagnostics: diags, RawOutput: out, Duration: time.Since(start)}
}
```

อัปเดตข้อความใน `cmd/apw/main.go` ให้เป็น `fmt -> vet -> build -> test` ด้วย

> หมายเหตุ: `ReflexEngine.MaxRetries` ยังไม่มีลูป retry — เป็นงาน milestone ถัดไป ไม่ใช่บั๊ก

---

## Surgeon (`internal/surgeon/surgeon.go`)

### 6. CRLF fallback แปลงทั้งไฟล์เป็น LF

**ไฟล์:** `surgeon.go:117-130`

ปัจจุบันเมื่อหา `OldContent` ไม่เจอ จะ normalize ทั้งไฟล์เป็น LF แล้วเขียนกลับ → ไฟล์ CRLF บน Windows จะ diff ทุกบรรทัด ขัดหลัก surgical patch และถ้าหลัง normalize เจอ >1 ที่ จะคืน `ErrTargetNotFound` ซึ่งผิด

แก้: คงชนิดบรรทัดของไฟล์ไว้ แปลง old/new ให้เป็น CRLF แทน

```go
func toCRLF(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}

matchCount := strings.Count(currentDiskContent, op.OldContent)
if matchCount == 0 && strings.Contains(currentDiskContent, "\r\n") {
	crlfOld := toCRLF(op.OldContent)
	if c := strings.Count(currentDiskContent, crlfOld); c > 0 {
		op.OldContent = crlfOld
		op.NewContent = toCRLF(op.NewContent)
		matchCount = c
	}
}
if matchCount == 0 {
	return "", ErrTargetNotFound
}
// เช็ก matchCount > 1 ต่อจากนี้ตามเดิม → ได้ ErrMultipleMatches ถูกต้อง
```

**เทสต์เพิ่ม:** ไฟล์ CRLF + patch ที่ส่ง old แบบ LF → ไฟล์ผลลัพธ์ยังเป็น CRLF ทุกบรรทัด (`!strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n")`)

### 7. ไม่กัน `OldContent` ว่าง

`strings.Count(s, "")` = `len(s)+1` → ถ้า `ReplaceAll: true` จะแทรก `NewContent` ระหว่างทุกตัวอักษร

```go
var ErrEmptyTarget = errors.New("surgeon: old_content must not be empty")

// ต้นฟังก์ชัน ApplyPatch
if op.OldContent == "" {
	return "", ErrEmptyTarget
}
```

การสร้างไฟล์ใหม่ควรเป็น operation แยก (`ActionCreate`) ไม่ใช่ patch ด้วย old ว่าง

### 8. Snapshot ใช้ path ดิบเป็น key

`ReadFile("a.go")` แล้ว `ApplyPatch({Path: "./a.go"})` → `ErrReadRequired` ทั้งที่อ่านแล้ว

```go
func snapshotKey(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}
	return abs, nil
}
```

ใช้ `snapshotKey` ทั้งใน `ReadFile` และ `ApplyPatch` (ใช้กับ map เท่านั้น — I/O ยังใช้ path เดิม) บน Windows อาจ `strings.ToLower` เพิ่มเพราะ filesystem ไม่สนตัวพิมพ์

**เทสต์เพิ่ม:** `ReadFile(filepath.Join(dir, "x.go"))` แล้ว patch ด้วย `filepath.Join(dir, ".", "x.go")` ต้องสำเร็จ

---

## CLI และ Architect

### 11. CLI

**ไฟล์:** `cmd/apw/main.go`

- help มี `daemon` แต่ยังไม่ได้ทำ → ลบออก หรือเขียน `(not implemented)`
- คำสั่งที่ไม่รู้จัก → พิมพ์ help ไป stderr แล้ว `os.Exit(2)` (ตอนนี้ exit 0)

```go
default:
	fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
	printHelp()
	os.Exit(2)
```

### 12. `ParsePlan` ไม่ตรวจความถูกต้องของแผน (security)

**ไฟล์:** `internal/architect/architect.go`

แผนมาจากโมเดล ซึ่งรับ input ภายนอกได้ (prompt injection) ถ้า Surgeon ทำตาม `target_file` โดยไม่ตรวจ → อาจแก้/ลบไฟล์นอก workspace เช่น `../../.ssh/config` **ต้องมีก่อนต่อ Architect เข้ากับ Surgeon**

```go
func (p *ExecutionPlan) Validate() error {
	if len(p.Subtasks) == 0 {
		return errors.New("plan has no subtasks")
	}
	for _, st := range p.Subtasks {
		switch st.Action {
		case ActionRead, ActionPatch, ActionCreate, ActionDelete, ActionVerify:
		default:
			return fmt.Errorf("subtask %s: unknown action %q", st.ID, st.Action)
		}
		if st.TargetFile == "" {
			if st.Action != ActionVerify {
				return fmt.Errorf("subtask %s: action %s requires target_file", st.ID, st.Action)
			}
			continue
		}
		if !filepath.IsLocal(st.TargetFile) {
			return fmt.Errorf("subtask %s: target_file %q escapes workspace", st.ID, st.TargetFile)
		}
	}
	return nil
}
```

เรียกใน `ParsePlan` ก่อน return `filepath.IsLocal` (Go 1.20+) ปฏิเสธ path absolute, `..`, และชื่อสงวนของ Windows แต่ไม่กัน symlink — ฝั่ง Surgeon ควรใช้ `os.Root` (Go 1.24+) ตอนเปิดไฟล์จริงเป็นชั้นที่สอง

ข้อสังเกตเพิ่ม: การหา fence ใช้ `strings.LastIndex("```")` — ถ้าโมเดลตอบมี code block อื่นตามหลัง JSON จะรวมขยะเข้ามา พิจารณาใช้ `json.Decoder` อ่านจาก `{` ตัวแรกแทน

---

## อื่นๆ (ไม่บล็อก)

- ไม่มี `README.md` และ CI (เช่น GitHub Actions รัน `gofmt -l`, `go vet`, `go test -race`)
- Git: อยู่บน `master` แต่ main branch ที่ตั้งไว้คือ `main` — ตัดสินใจว่าจะ rename หรือไม่ก่อน push
- งานใหม่ยังไม่ commit: `internal/architect/`, `internal/reflex/`, `internal/surgeon/`

## เกณฑ์เสร็จ

- [x] `gofmt -l .` ว่าง
- [x] `go mod tidy -diff` ไม่มี diff และไม่มี `go-rod/rod` ใน `go.mod`
- [x] `go vet ./...` และ `go build ./...` ผ่าน
- [x] `go test ./...` ผ่าน รวมเทสต์ใหม่ข้อ 2, 6, 7, 8, 12
- [x] `go run ./cmd/apw verify .` ได้ PASS
