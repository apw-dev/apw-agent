# ADR 0001: การเลือก SQLite Driver สำหรับ Windows (`modernc.org/sqlite`)

- **วันที่:** 2026-10-16
- **สถานะ:** ยอมรับ (Accepted)
- **บริบท:** Milestone M0 (T8 SQLite Spike)

## 1. บริบทและปัญหา

`apw` และ `apwd` พัฒนาและรันบน Windows เป็นหลัก (`CGO_ENABLED=0` เป็นค่าที่ต้องการเพื่อให้ติดตั้งและคอมไพล์ได้โดยไม่ต้องติดตั้ง MinGW/GCC แยก) ระบบต้องการฐานข้อมูลแบบฝังตัว (Embedded Relational Store) สำหรับเก็บสถานะของ `tasks`, `sessions`, durable `inbox` (dedup) และ `outbox` ใน M1–M4

## 2. การทดลอง (`internal/store`)

สร้างแพ็กเกจ `internal/store` โดยใช้ `modernc.org/sqlite` (`v1.60.1`) ร่วมกับ `database/sql` มาตรฐาน และตาราง `tasks`:

1. **โหมด WAL (`journal_mode=WAL`) และ `busy_timeout`:**
   - กำหนดผ่าน DSN: `file:<path>?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)`
   - เพื่อให้ทุก connection ที่ `database/sql` เปิดขึ้นมาใน pool ได้รับค่า pragma เดียวกันโดยอัตโนมัติ
   - ผลตรวจจาก `PRAGMA journal_mode;` ยืนยันว่าได้ `wal` บน Windows NTFS
2. **การเขียนพร้อมกันจาก 2 Goroutines (`TestStore_WALAndConcurrentWrites`):**
   - ทดสอบให้ 2 goroutines เขียนข้อมูลลงตาราง `tasks` พร้อมกัน (goroutine ละ 25 รายการ รวม 50 รายการ)
   - เมื่อตั้ง `_pragma=busy_timeout(5000)` ร่วมกับ `WAL` ทั้ง 2 goroutines สามารถเขียนสำเร็จครบ 50 รายการโดยไม่เกิด `SQLITE_BUSY` (`database is locked`)
3. **ขนาด Binary และเวลา Build บน Windows:**
   - เมื่อไม่รวม SQLite (`cmd/apw`): ขนาด binary ประมาณ ~4.8 MB, เวลา build อุ่นเครื่อง (warm cache) ~0.8–1.2 วินาที
   - เมื่อนำเข้า `modernc.org/sqlite`: ขนาด binary เพิ่มขึ้นประมาณ ~11–13 MB (รวมเป็น ~16–18 MB), เวลา compile ครั้งแรก (cold cache) ประมาณ ~6–9 วินาที และครั้งถัดไปเมื่ออยู่ใน Go build cache ประมาณ ~1.0–1.5 วินาที

## 3. การตัดสินใจ

**เลือกใช้ `modernc.org/sqlite`** เป็น SQLite driver หลักสำหรับ `apw` / `apwd`

### เหตุผลที่เลือก
- **Pure Go (`CGO_ENABLED=0`):** ไม่ต้องพึ่งพา C compiler (`gcc`/`mingw-w64`) บนเครื่องผู้ใช้หรือใน CI บน Windows และ cross-compile ได้ทันที
- **รองรับ WAL และ `busy_timeout` ผ่าน DSN:** ป้องกันปัญหา connection ใหม่ใน `database/sql` ลืมตั้ง pragma จนเกิด `SQLITE_BUSY`
- **ขนาดและเวลา Build อยู่ในเกณฑ์ยอมรับได้:** ขนาด binary ที่เพิ่มขึ้น ~12 MB ไม่มีผลกระทบต่อการใช้งานแบบ CLI/daemon บนเครื่องพัฒนา และ Go build cache ช่วยให้การรัน `go test ./...` รอบถัดไปยังคงเร็ว (~3–6 วินาทีทั้งโปรเจกต์)

### ข้อกำหนดการใช้งานใน M1+
- ทุกการเปิดฐานข้อมูลต้องระบุ `_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)` ใน DSN เสมอ
- สำหรับงานเขียนที่ต้องการความแน่นอนสูงใน daemon ให้ใช้ transaction ระยะสั้นและไม่ถือ transaction ค้างระหว่างรอผลจาก Model Provider
