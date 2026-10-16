# แผนพัฒนา apw — Apiwat Agent

สถานะ: แผนเสนอ revision 5 — บูรณาการคำแนะนำสถาปัตยกรรมและจุดเสี่ยงจาก Gemini Argon (go-rod/rod CDP, Single-Worker FIFO, Network Stream Event, SingletonLock Lifecycle) พร้อมรับคำสั่งผ่าน Telegram Gateway และ Browser Provider

## 1. เป้าหมายและขอบเขต

สร้าง agent ด้วย Go ที่ใช้งานผ่าน Telegram หรือ Discord ได้คล้ายแนวทาง Hermes รับงานต่อเนื่องผ่านบริการเบื้องหลัง เข้าใจบริบท สร้างแผนหรือ patch แก้ไฟล์ในพื้นที่แยก และส่งผลตรวจสอบกลับแชตเดิม โดยเลือกใช้โมเดลได้ทั้ง API, local และหน้าเว็บ เช่น Gemini/Argon ที่บัญชีผู้ใช้เข้าถึงได้

จากการตรวจครั้งแรก workspace `D:\APW_Project\apw-agent` ยังไม่มี source code หรือ Git repository ปัจจุบันมีเอกสารแผนนี้ แผนยังอยู่ในขั้นออกแบบและยังไม่ได้ทดลองเชื่อมบัญชีแชตหรือ Gemini ของผู้ใช้

**ข้อกำหนดหลักที่ห้ามเลื่อนไปเป็นของเสริม**

1. สั่งงาน สนทนาต่อเนื่อง ดูสถานะ ยกเลิก และรับผลกลับผ่าน Telegram หรือ Discord ได้ โดย MVP ส่งมอบอย่างน้อยหนึ่งช่องทาง และ Beta รองรับทั้งสองช่องทาง
2. เปิดเว็บโมเดลผ่าน browser ที่ผู้ใช้เข้าสู่ระบบแล้ว เลือกโมเดลที่มีสิทธิ์ใช้งาน ส่ง prompt รอ response และนำคำตอบกลับเข้า agent workflow ได้ เป็น provider ทางเลือกที่เลือกใช้จริงได้
3. คำตอบจากเว็บต้องนำไปใช้วางแผนหรือสร้าง patch และตรวจผลได้ครบวงจร การเปิดหน้าเว็บได้หรือคัดลอกคำตอบมาวางด้วยมืออย่างเดียวไม่ถือว่าผ่านข้อ 2

**สมมติฐานสำหรับประมาณเวลา**

- นักพัฒนาเต็มเวลา 1 คน ใช้เวลา 12 สัปดาห์สำหรับเส้นทางหลัก และเผื่ออีก 2 สัปดาห์สำหรับการรวมระบบและแก้ปัญหา ปรับจากแผนเดิมเพื่อรวม gateway และ browser provider; ต้องประเมินใหม่หลัง spike สัปดาห์แรกและทุก milestone
- รุ่นแรกพัฒนาและใช้งานบน Windows เป็นหลัก รองรับ repository ภาษา Go ก่อน
- ใช้ Telegram เป็นช่องทางแรกตามแผนเสนอ แล้วเพิ่ม Discord ใน Beta; CLI เป็นเครื่องมือ setup/debug ที่เรียก core เดียวกัน
- ทดสอบ Gemini browser provider ตั้งแต่สัปดาห์แรก และส่งมอบพร้อม MVP; มี API/local เป็นทางเลือกแยกที่ผู้ใช้ตั้งค่าเอง ไม่บังคับให้มี API key เพื่อใช้เส้นทางเว็บ
- ใช้โปรเจกต์ Go ตัวอย่างเป็นพื้นที่ทดสอบ รวมทั้งกรณี build ไม่ผ่าน ไฟล์ถูกแก้ระหว่างทำงาน และผู้ใช้ยกเลิกงาน
- ไม่มีข้อกำหนดเรื่องงบโมเดลหรือเครื่อง local ในเอกสาร จึงต้องตั้งเพดาน request, token และเวลาต่อ task ผ่าน config และเก็บค่าที่ใช้จริงก่อนเลือกค่าเริ่มต้นสำหรับ release

## 2. ผลลัพธ์ที่ต้องใช้งานได้

### MVP: จบ milestone M2

ผู้ใช้ส่งงานใน Telegram เลือก `gemini-web` และโมเดลที่บัญชีเข้าถึงได้ เช่น Argon ระบบส่งบริบทที่จำเป็นไปหน้าเว็บ รับคำตอบกลับมาสร้างแผน/patch สำหรับ Go หนึ่งจุด ทดลองใน Git worktree รัน build/vet/test และส่งสรุป diff/report กลับแชตเดิม ผู้ใช้ดูสถานะและยกเลิกจากแชตได้ หากล้มเหลวต้องไม่ทำให้ working tree เดิมเสียหาย

ต้องมีหลักฐานทดสอบบนบัญชีที่ผู้ใช้เปิดใช้จริง; ถ้าเว็บยังอ่าน response อัตโนมัติไม่ได้ ให้ระบุ browser provider ว่า blocked/experimental และยังไม่ประกาศผ่าน MVP ข้อนี้ แม้เส้นทาง API จะทำงานแล้วก็ตาม

### รุ่น Beta: จบ milestone M4

รองรับ Telegram และ Discord ผ่าน gateway เดียวกัน มี router สองระดับและ planner สำหรับงานหลายไฟล์ พร้อม browser session recovery, การยกเลิกงาน และประวัติที่ยังอ่านได้หลัง daemon restart

### รุ่น v1: จบ milestone M5

เพิ่มความจำการแก้ปัญหาและ skill registry ที่บันทึกแหล่งที่มา ตรวจสอบได้ และมีขอบเขตสิทธิ์ชัดเจน พร้อมแพ็กเกจติดตั้งและเอกสารใช้งาน

**เลื่อนออกจากเส้นทางหลัก:** VS Code extension, Web UI ของ apw, public remote RPC, semantic rename ข้ามภาษา, voice/video และการรัน skill ที่สร้างเองโดยไม่ผ่านการเปิดใช้ การเชื่อม Telegram/Discord ใช้ outbound connection จากเครื่องที่รัน agent ได้ตาม transport ที่เลือก จึงแยกจากการเปิด public RPC

## 3. ข้อตกลงทางสถาปัตยกรรม

| ประเด็น | แนวทางในแผนนี้ | เหตุผลและเงื่อนไขขยาย |
| --- | --- | --- |
| Core | ให้ application service เป็นศูนย์กลางตั้งแต่ต้น | CLI, chat gateway และ daemon ใช้ task lifecycle และ policy เดียวกัน |
| Parser | เริ่ม Go ด้วย `go/parser` และ `go/ast` หลัง interface ของ indexer | ลดขอบเขตการ build รุ่นแรก; เพิ่ม tree-sitter adapter เมื่อเริ่มภาษาอื่นและผ่าน spike บน Windows |
| Model | provider interface รองรับ `api`, `local`, `browser` และ capability ของแต่ละแบบ | เส้นทางเว็บไม่สมมติว่ามี native tool calling หรือ constrained JSON; ตรวจ output และ normalize ก่อนเข้า core |
| Gateway | platform adapter → session/task service → event/outbox → platform adapter | แยก platform จาก agent; แยก conversation, owner และ workspace อย่างชัดเจน |
| Browser worker | process แยกจาก Go core มี protocol ที่ versioned | spike ทางเลือก Playwright worker หรือ browser driver ที่เหมาะสมก่อนเลือก dependency; แยก lifecycle/session และความล้มเหลวจาก core |
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

1. **The Architect (การคิดเชิงสถาปัตยกรรมและวางผังงาน):**
   - **หน้าที่:** ถอดรหัสความต้องการของผู้ใช้ (intent analysis), ทำความเข้าใจโครงสร้างโปรเจกต์เชิงลึก (system & symbol topology), และแตกงานเป็นขั้นตอนย่อยที่มีลำดับขึ้นต่อกัน (DAG of atomic subtasks)
   - **กลยุทธ์โมเดล:** มอบหมายให้โมเดลระดับสูงที่มี reasoning capacity กว้างขวาง (เช่น **Gemini Argon / Advanced** ผ่าน Browser CDP หรือ Claude Sonnet) รับหน้าที่คิด ออกแบบโครงสร้าง และตรวจสอบ architecture tradeoffs
   - **สัญญาการทำงาน:** สถาปัตย์จะไม่ลงมือแก้ไขไฟล์โดยตรงแบบสุ่มสี่สุ่มห้า แต่จะส่งมอบ `ExecutionPlan` ที่ระบุชัดเจนถึงเป้าหมาย, ไฟล์ที่เกี่ยวข้อง, สัญลักษณ์ (symbol) ที่กระทบ, และเงื่อนไขการทดสอบ (acceptance criteria)

2. **The Surgeon (ศัลยแพทย์โค้ดตามหลักการ Claude Code):**
   - **หลักการ Read-Before-Write:** ห้ามแก้ไขไฟล์ใดๆ โดยไม่อ่านบริบทจริงล่าสุดเด็ดขาด เพื่อป้องกัน hallucination ของโครงสร้างโค้ด
   - **Precision Surgical Patching:** ไม่เขียนทับไฟล์ทั้งไฟล์ (Full file overwrite) หากไม่จำเป็นอย่างยิ่ง แต่ใช้การผ่าตัดเฉพาะจุด (Targeted block replacement) โดยอ้างอิง unique surrounding context lines และ AST node boundaries เพื่อรับประกันว่าจะไม่มี line drift หรือแก้ผิดตำแหน่ง
   - **Base Hash & Integrity Fingerprint:** ทุก patch ต้องระบุ SHA-256 hash หรือ content fingerprint ของไฟล์ตั้งต้น หากไฟล์ในดิสก์ถูกแก้ไขโดยภายนอกระหว่างทำงาน ระบบศัลยกรรมจะปฏิเสธการแก้ทับทันที (Refuse on drift)
   - **Safe Workspace Isolation:** การผ่าตัดโค้ดทุกครั้งต้องทำในพื้นที่ทดลองแยก (**Git Worktree**) เสมอ เพื่อรับประกันว่า working tree เดิมของผู้ใช้จะไม่เสียหายแม้การผ่าตัดจะล้มเหลว

3. **The Reflex (การสะท้อนคิด แก้ไขตัวเอง และทดสอบอัตโนมัติ - Reflexion Loop):**
   - **Closed-Loop Verification:** เมื่อ Surgeon ลงมือผ่าตัดเสร็จสิ้น Reflex Engine จะเข้ามารับช่วงต่อทันที โดยสั่งรันวงจรตรวจจับข้อผิดพลาดแบบ multi-stage: `gofmt` → `go vet` → `go build` → `go test`
   - **Autonomous Self-Healing:** หากคอมไพเลอร์หรือเทสต์ล้มเหลว Reflex Engine จะดักจับ stderr / compiler error traceback นำมาวิเคราะห์ความผิดพลาด (reflection) และสร้าง surgical fix วนซ้ำอัตโนมัติ (สูงสุด 3 ครั้งแบบมี bounded retry)
   - **Fast-Path Reflex:** สำหรับข้อผิดพลาดเชิงไวยากรณ์หรือ type checking เล็กๆ สามารถใช้โมเดลความเร็วสูง (Fast Reflex เช่น Gemini Flash หรือ Local model) เพื่อแก้ปัญหาได้ในเสี้ยววินาที โดยไม่ต้องส่งกลับไปยัง Architect ขนาดใหญ่ เว้นแต่เป็นปัญหาเชิงสถาปัตยกรรม

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

### 3.2 Browser Model Provider — ใช้โมเดลผ่านหน้าเว็บ

**เป้าหมาย:** apw ควบคุม browser ผ่าน UI ที่บัญชีมีสิทธิ์ใช้จริง ส่งคำถามและอ่านคำตอบเพื่อทำงานต่อได้ โดยยังเลือก API/local แทนได้ตาม config

เอกสาร [Gemini models](https://support.google.com/gemini/answer/18648722?hl=en-12) ระบุ Argon และการเลือกโมเดล แต่สิทธิ์ขึ้นกับบัญชี แผน และ usage ที่เหลือ จึงเก็บชื่อ model เป็น config และตรวจจาก UI จริง ไม่ hard-code ว่าทุกบัญชีใช้ Argon ได้ การอ่านเอกสารยังไม่ใช่หลักฐานว่า automation ใช้กับบัญชีของผู้ใช้ได้สำเร็จ

1. **Setup:** เปิด browser profile เฉพาะ apw แบบมองเห็นได้ ให้ผู้ใช้ sign in/เลือกบัญชีเอง แล้วบันทึก reference ของ profile ในเครื่อง ไม่เก็บรหัสผ่านหรือส่ง cookies ผ่าน Telegram/Discord
2. **Session:** หนึ่ง browser conversation ต่อ agent session มี mapping ของ task/request กับ chat URL และ response turn; จำกัดหนึ่งคำขอที่กำลัง generate ต่อ conversation/profile ตามข้อจำกัดที่ทดสอบได้
3. **Model selection:** แยก URL/account profile ออกจากชื่อโมเดล เลือกด้วย UI และบันทึก requested model กับ observed model/label; ถ้ายืนยันตัวที่ใช้จริงไม่ได้ให้รายงาน unknown และ strict mode ต้องไม่อ้างว่าใช้ Argon สำเร็จ
4. **Prompt:** ส่ง instruction + context ที่คัดแล้ว + response schema โดยมีเพดานขนาด; ไม่ส่ง secret และส่ง source เฉพาะในขอบเขต project/provider policy ที่ผู้ใช้เปิดใช้
5. **Wait:** ใช้สถานะ generation ของ UI ร่วมกับ turn identity, CDP Network events (ตรวจจับ Network response streaming เพื่อทราบจุดสิ้นสุดชัดเจน) และเงื่อนไข completion ที่ทดสอบแล้ว ไม่ใช้ sleep คงที่หรือ text หยุดเปลี่ยนอย่างเดียว
6. **Read:** ดึงเฉพาะ assistant answer ของ request ปัจจุบัน พร้อม code blocks/format ที่จำเป็น แยก reasoning/status/navigation และไม่เผลอใช้คำตอบเก่าหรือคำตอบจาก chat อื่น
7. **Normalize:** เก็บ raw response และสร้าง `ModelResponse` ที่มี provider, request ID, conversation/turn, model observation, completion status และ content; parse เป็น Plan/PatchSet แล้วตรวจ schema และ scope ตาม flow ปกติ
8. **Continue:** ส่ง error ของ compiler/test กลับเป็น turn ใหม่เพื่อซ่อมภายในเพดานเดิม; ใช้ browser response เป็นข้อมูล ไม่ execute คำสั่งจากหน้าเว็บข้าม policy
9. **Recover:** เมื่อ login หมดอายุ, CAPTCHA, quota, model ไม่พร้อม หรือ DOM เปลี่ยน ให้สถานะเฉพาะและแจ้งเจ้าของ; login/CAPTCHA ให้ผู้ใช้จัดการเอง ไม่มีการข้าม access control
10. **Fallback:** เลือกหยุดรอ, เปลี่ยน provider ตาม fallback policy ที่เปิดไว้ หรือ export prompt/import response ด้วยมือเพื่อกู้งาน ต้องบอกเสมอว่าเปลี่ยนเส้นทาง และ manual mode ไม่ทดแทนเกณฑ์ผ่าน browser automation
11. **Concurrency & Rate Limit Policy (Argon Review):**
    - บังคับใช้คิวคำขอ (FIFO Request Queue) จัดการแบบ Single Browser Worker ต่อ 1 Browser Profile ห้ามส่งคำสั่งขนานบนแท็บเดียวกัน
    - เพิ่ม Jitter (หน่วงเวลาสุ่ม 2–5 วินาที) ก่อนส่งคำสั่งเพื่อเลียนแบบพฤติกรรมมนุษย์และลดความเสี่ยง Anti-bot / Rate Limit
    - ใช้ flags ป้องกัน bot detection เช่น `--disable-blink-features=AutomationControlled` และใช้ Persistent User Data Dir
    - กำหนด Timeout ชัดเจนผ่าน `context.WithTimeout` (90–120s) หาก timeout หรือค้าง ให้ Worker สั่ง Reload หน้าเว็บเพื่อรีเซ็ตสถานะทันที
    - ทำ Graceful Shutdown ดักจับ SIGINT/SIGTERM เพื่อเคลียร์ `SingletonLock` ในโปรไฟล์ Chrome ป้องกันปัญหา Browser Lock ในการบูตครั้งถัดไป

auth state ของ browser อาจมีข้อมูลที่ใช้สวม session ได้ จึงแยก profile, จำกัดสิทธิ์ไฟล์, ไม่ commit หรือแนบ report ตามข้อควรระวังใน [Playwright authentication](https://playwright.dev/docs/auth) แต่ยังต้องทดสอบการเข้าสู่ระบบและ profile persistence ของเว็บจริงใน spike

**ข้อจำกัดที่ต้องออกแบบไว้:** UI เปลี่ยนได้, subscription เว็บไม่ใช่ API credential, การวัด token/cost อาจไม่เปิดเผยจาก UI, อาจถูกเปลี่ยนโมเดลเมื่อ quota เปลี่ยน และ structured output บนเว็บอาจต้องซ่อมรูปแบบเพิ่มเติม ตัวเลขที่อ่านไม่ได้ต้องเป็น unknown ไม่รายงานเป็นศูนย์

### 3.3 โมดูลที่เพิ่มจากสเปกเดิม

```text
internal/gateway/                  # รับ event, session routing, inbox/outbox
internal/channels/telegram/        # Telegram bot adapter
internal/channels/discord/         # Discord bot adapter
internal/session/                  # owner + conversation + workspace + provider
internal/provider/browser/         # Go adapter ติดต่อ browser worker
internal/browser/                  # worker lifecycle, profile และ request mapping
workers/browser/                   # runtime/driver ที่เลือกหลัง spike
```

core และ daemon ยังคงเขียนด้วย Go; browser worker อาจมี runtime เพิ่ม หากเลือกแนวทางนั้นต้องรวมวิธีติดตั้ง browser/runtime และ version compatibility ใน installer ด้วย

## 4. ลำดับทำงานหลักและข้อมูลกลาง

```text
Telegram / Discord / CLI
  → Gateway + ตรวจ owner/workspace + inbox dedup
  → Session / Task service + ตรวจ policy
  → จัดประเภทงาน
  → อ่าน repo map และบริบทที่จำเป็น
  → Model router: API / Local / Browser (เช่น Gemini/Argon)
  → ส่ง prompt → รอคำตอบ → normalize + validate
  → สร้างและตรวจ Plan/PatchSet
  → ตรวจ base hash / path / symbol
  → ทดลองใช้ใน worktree
  → format + build + vet + test
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
- `ConversationSession`: platform identity, owner, workspace binding, context revision, provider/model preference และ browser conversation reference
- `InboundMessage` / `OutboundDelivery`: platform event/message ID, task mapping, dedup key, delivery state และ retry metadata
- `ModelRequest` / `ModelResponse`: request ID, provider capability, model ที่ขอ/ที่สังเกตได้, content, completion status และ raw response reference

ใช้ verification profile เช่น `go-package` หรือ `go-repo` แทนการให้ `verification_cmd` ในแผนเป็น shell string อิสระ profile จะแปลงเป็นคำสั่งที่ project policy อนุญาต

สถานะหลัก: `queued → analyzing → planning/patching → verifying → succeeded` พร้อม `failed`, `cancelled` และ `needs_review`; เพิ่มสถานะรอ `waiting_for_login`, `waiting_for_user` และ `provider_unavailable` ที่มี timeout/resume policy ทุกสถานะต้องมีเหตุผลและ artifact ที่เกี่ยวข้อง

คำว่า `succeeded` หมายถึงงานและการตรวจที่กำหนดเสร็จในพื้นที่แยก การนำผลกลับเข้า working tree ของผู้ใช้เป็นอีกขั้นที่ผู้ใช้เลือก และต้องตรวจ base hash ซ้ำ

## 5. Milestones และเกณฑ์ส่งมอบ

| Milestone | เวลาโดยประมาณ | สิ่งที่ทำ | เกณฑ์ผ่านก่อนขึ้นช่วงถัดไป |
| --- | --- | --- | --- |
| **M0: Foundation + feasibility** | สัปดาห์ 1 | Go skeleton, provider/session contracts, SQLite/Windows spike, Telegram bot และ browser proof of concept | bot รับ/ตอบข้อความทดสอบได้; browser ส่ง prompt และจับคู่คำตอบจบครบ turn ได้บนบัญชีที่เปิดใช้; บันทึกข้อจำกัดจริง |
| **M1: Safe core + gateway** | สัปดาห์ 2–3 | worktree, runner, patch/rollback, `apwd` ขั้นต้น, Telegram adapter, session, inbox/outbox และ browser worker lifecycle | สั่ง mock task จากแชต ดูสถานะ/cancel/รับ report ได้; duplicate event ไม่สร้างงานซ้ำ; ไฟล์เดิมไม่เสียหาย |
| **M2: Chat + web-model MVP** | สัปดาห์ 4–5 | Go AST, Gemini browser provider, output validation, repair และ flow ครบวงจร | Telegram → browser model → patch Go → build/vet/test → ส่งผลกลับแชตเดิมได้; login/quota/cancel มีสถานะจริง; ยืนยันโมเดลตาม strict policy |
| **M3: Discord + Dual-brain** | สัปดาห์ 6–8 | Discord adapter, router, API/local option, repo map, DAG planner และ executor | ใช้ทั้งสองแพลตฟอร์มได้; session ไม่ปะปน; งานหลายไฟล์มีแผนและ final gate; เลือก provider/model ได้ |
| **M4: Reliability Beta** | สัปดาห์ 9–10 | local RPC, watcher, reconnect/restart recovery, browser UI regression และ delivery retry | อ่าน task หลัง restart ได้; ระบุ interrupted โดยไม่ส่ง prompt ซ้ำแบบเดา; outbox กลับมาส่งได้; shutdown ไม่ทิ้ง process |
| **M5: Memory, skills, release** | สัปดาห์ 11–12 | episodic memory, skill registry, permissions, browser/runtime packaging และเอกสาร | memory ตรวจ/ลบได้; skill ที่ไม่เปิดใช้รันไม่ได้; เครื่องใหม่ตั้งค่า bot และ browser แล้วผ่าน acceptance demo |

**สัปดาห์ 13–14 เป็น buffer:** แก้ปัญหาจากใช้งานจริง ความเปลี่ยนแปลง UI และ packaging ไม่ถือว่า TypeScript/Python/extension จะเสร็จโดยอัตโนมัติใน buffer หาก browser spike ไม่ผ่านต้องประเมินเวลาใหม่ทันที

ปรับจากสเปกเดิมโดยย้าย daemon ขั้นต้นและ chat gateway มา M1 เพิ่ม browser provider ใน M0–M2 และเพิ่ม Discord ใน M3 ส่วน M4 เน้นความทนทาน แทนการเพิ่งเริ่ม daemon ตอนปลายโครงการ

### M0 — Foundation

1. เมื่อเริ่ม implementation ให้สร้าง Git repository และโครง `cmd/apw`, `cmd/apwd`, `internal/config`, `internal/core`, `internal/provider`, `internal/gateway`, `internal/session`, `internal/task` และ `testdata`
2. เขียนข้อสรุปสถาปัตยกรรมสั้น ๆ เรื่อง parser, provider รายแรก, patch format, verification และ task lifecycle
3. สร้าง config สำหรับ model, endpoint, credential reference, timeout, workspace policy และ budget; แยก secret จาก config ที่ commit ได้
4. เพิ่ม mock provider ที่ให้ผล deterministic สำหรับ integration test และ smoke test การเชื่อมต่อ provider จริงแบบ opt-in
5. ตั้ง build/test/vet และ format check ใน CI โดยไม่ต้องใช้ API key
6. ทดลอง Telegram private bot และ browser session ด้วยบัญชีที่เจ้าของเปิดใช้; ไม่เปิดรับผู้ใช้ทั่วไปในขั้นทดลอง
7. พิสูจน์ browser round trip: เลือกโมเดล → ส่ง prompt → รอจบ → อ่าน code block/JSON → จับคู่ request; ทดสอบอย่างน้อย multi-turn, fresh chat และ response ยาว พร้อมบันทึกสิ่งที่ยังทำไม่ได้
8. ตัดสินใจ browser driver/worker จากผล spike และกำหนดแหล่งเก็บ profile, runtime version และโปรโตคอลกับ Go core; **เลือกใช้ `go-rod/rod` เป็นตัวขับเคลื่อน CDP หลักตามข้อเสนอแนะของ Argon** (เพื่อความแม่นยำในการจัดการ Shadow DOM, Auto-wait, และ Network Event stream)
9. **Concurrency & Rate Limit Policy (จาก Argon):** กำหนดให้ Browser Worker ทำงานแบบ Single Worker Queue (FIFO channel) 1 แท็บต่อ 1 งาน ป้องกัน race condition และ rate limit พร้อมเพิ่ม jitter หน่วงเวลาสุ่ม 2–5 วินาที
10. **Process & Lock Management (จาก Argon):** ออกแบบ graceful shutdown สำหรับจัดการลบไฟล์ `SingletonLock` ใน User Data Dir เมื่อโปรเซสหยุดทำงาน ป้องกันปัญหา Chrome ค้างหรือเปิดซ้ำไม่ได้

### M1 — Safe execution

1. ตรวจ repo root, Git state และ path ที่อนุญาตก่อนสร้างพื้นที่งาน; รุ่นแรกให้ repository ที่ยังไม่มี commit หรือมีสถานะที่รองรับไม่ได้จบด้วยคำอธิบายที่ชัดเจน
2. รองรับ working tree ที่มีงานค้างโดยไม่ reset, stash หรือคัดลอกความเปลี่ยนแปลงเข้า task อย่างเงียบ ๆ รุ่นแรกให้หยุดงานแก้ไขและแนะนำให้ผู้ใช้สร้างฐานที่พร้อมใช้งาน; คำสั่งอ่านอย่าง map ยังใช้ได้
3. ตรวจ path traversal, symlink/reparse point ที่พาออกนอก workspace, file hash, ขอบเขต symbol และ expected text ก่อน patch
4. ทดลอง patch ทุกไฟล์ ตรวจ parse และ format แล้วเขียนอย่าง atomic ต่อไฟล์; ถ้าไฟล์ใดใช้ไม่ได้ให้คืน snapshot ของทั้ง patch set ใน worktree นั้น
5. รัน process ผ่าน policy พร้อม timeout, จำกัด output และ kill process tree เมื่อ cancel โดยทดสอบพฤติกรรมบน Windows
6. ปฏิเสธ repository task ที่ต้องรันโค้ดจนกว่าจะผ่านการตั้งค่า trust/policy เพราะ build และ test อาจเรียกโค้ดหรือ subprocess ของ repository ได้
7. เก็บ baseline ของคำสั่งตรวจสอบก่อนแก้ เพื่อแยกข้อผิดพลาดเดิมออกจากผลของ patch
8. เพิ่ม `apwd` ขั้นต้นและ Telegram adapter พร้อม owner/workspace binding, durable inbox/outbox, session, status/cancel และ task event; ใช้ mock provider ทดสอบแชตถึง core ก่อนเชื่อมโมเดลจริง
9. เพิ่ม browser worker start/stop, request IDs, per-profile lock และ cancellation; หลังยกเลิกห้ามนำ response ที่ตามมาช้าไป apply patch

### M2 — Telegram + Browser Model + Go MVP

1. สแกน Go symbols และสร้าง repo map ที่ตัด generated/vendor/build output ตาม policy; จัดการ encoding และ CRLF โดยไม่ทำให้ diff เปลี่ยนทั้งไฟล์
2. ให้ผู้ใช้เลือก mode `preview` หรือ `execute` อย่างชัดเจน; การ execute ใน worktree ไม่ใช่การ merge กลับ workspace
3. ตรวจ JSON ตาม schema, field ที่ไม่รู้จัก, ขนาด response และขอบเขตไฟล์ แล้วตรวจ patch อีกชั้น
4. ใช้ `gofmt`, `go build ./...`, `go vet ./...` และ test profile ที่เหมาะกับ repository โดย full gate ต้องผ่านก่อนจบงาน
5. จำกัด repair เป็น **สร้างครั้งแรก 1 ครั้ง + ซ่อมสูงสุด 3 ครั้ง** ทุกครั้งมีรายงานแยก; ถ้าเกิน budget/deadline ให้หยุดก่อนครบได้
6. ส่ง error excerpt พร้อมบริบทที่จำเป็นให้โมเดลซ่อม โดยเก็บ log เต็มไว้ตรวจสอบ และปิดบัง secret
7. เมื่อซ่อมไม่สำเร็จให้คืนฐานใน worktree เก็บ patch/report สำหรับ review และรายงาน failed; ไม่จำเป็นต้องรอผู้ใช้ตอบเพื่อจบ task
8. เชื่อม browser provider กับ Gemini UI จริง มี model selection, response-turn mapping, completion detection และ normalization ตามข้อ 3.2
9. ส่ง progress, diff summary และ verification report กลับ Telegram session เจ้าของ task; สนทนาต่อได้โดยยังอ้างถึงงานเดิมและ revision ถูกต้อง
10. ทำ acceptance demo โดยปิด API/local provider: สั่งผ่าน Telegram ให้ใช้เว็บโมเดลแก้ fixture Go แล้วตรวจว่า patch/report เกิดจากคำตอบ turn ที่บันทึกจริง ทั้งในรอบแรกและ repair หนึ่งรอบ
11. ทดสอบ session หมดอายุ, quota, UI หาไม่พบ, คำตอบไม่ใช่ JSON, code block ไม่ครบ และ cancel ระหว่าง generate; ห้ามรายงาน task success จากข้อความบางส่วน

### M3 — Discord, Router และ Architect

1. Level 0 ใช้คำสั่งที่ตรงและตรวจสอบได้ เช่น map, format, test ตาม policy; ไม่ใช้โมเดลเมื่อไม่จำเป็น
2. Level 1 รับงานขอบเขตเล็ก; Level 2 รับงานหลายไฟล์ งาน API/schema/dependency/security หรือบริบทไม่แน่นอน จำนวนมากกว่า 3 ไฟล์เป็นสัญญาณหนึ่ง ไม่ใช่เกณฑ์เดียว
3. หากขอบเขตงานจริงขยายหลังสร้าง patch ต้องจัดประเภทใหม่; `apw fx` ปฏิเสธงานที่เกินขอบเขตและเสนอให้ใช้ plan
4. Planner สร้าง JSON ที่ตรวจ schema, dependency ที่มีอยู่จริง, cycle, file scope, verification profile และ acceptance criteria ได้ ก่อนแปลงเป็น Markdown ให้ผู้ใช้ตรวจ
5. รัน DAG แบบ sequential ก่อนเพื่อให้ rollback และเหตุการณ์ตรวจสอบง่าย; เพิ่ม parallel execution เมื่อมี conflict analysis และความจำเป็นจากข้อมูลใช้งาน
6. งานที่ล้มเหลวไม่ปล่อย dependent steps ทำต่อ; ทั้ง task ต้องผ่าน final verification ก่อนส่งมอบ และคืนฐานทั้ง task เมื่อปิดงานแบบ failed
7. การรันแผนเดิมต้องตรวจ revision ใหม่ หากเปลี่ยนต้อง revalidate หรือ replan ตามผลกระทบ
8. เพิ่ม Discord adapter สำหรับ DM/mention/channel/thread ตาม configuration พร้อม reconnect และ bot-loop prevention
9. ทดสอบ Telegram กับ Discord พร้อมกันด้วยคนละ session โดยไม่รั่ว context, browser conversation, artifact หรือสิทธิ์ของเจ้าของ
10. รองรับ provider/model preferences แยกต่อ session และบทบาท reflex/architect; browser ใช้เป็น architect ที่ตอบช้ากว่าได้ โดยไม่บังคับ latency เท่า local reflex

### M4 — Daemon และ Browser/Gateway Reliability

1. เพิ่ม local RPC ให้ CLI คุย application service ของ daemon ที่เริ่มตั้งแต่ M1 โดยไม่ทำสำเนา core logic
2. เริ่ม protocol จาก `health`, `task.create`, `task.get`, `task.cancel`, `task.events`, `repo.map` และ `plan.get`
3. เพิ่ม protocol version, structured errors, request ID และ event sequence; reconnect ต้องไม่สร้าง task ซ้ำจาก request เดิม
4. watcher ทำ debounce, incremental invalidation และ rescan เมื่อพลาด event; index ต้องรู้ revision ที่ตนอ่านอยู่
5. จำกัดหนึ่งงานแก้ไขต่อ workspace ในรุ่นแรก และ serialize การเขียน state
6. restart แล้วทำเครื่องหมายงานที่ยังไม่จบเป็น interrupted; ยังไม่สั่งรัน tool ต่ออัตโนมัติจนกว่าจะตรวจ checkpoint และ policy
7. ทดสอบ start/status/stop, lock file, stale process metadata, graceful shutdown และ retention ของ log/worktree
8. เก็บ browser checkpoint ก่อน/หลัง submit และ turn ที่อ่านได้; หาก crash หลังส่งแต่ยังไม่ทราบผล ให้ reconcile UI ก่อน retry ไม่ส่ง prompt เดิมซ้ำทันที
9. แยก UI regression tests ที่ใช้ fixture DOM จาก live smoke tests ที่ต้องมีบัญชีผู้ใช้และไม่รันบน public CI; ตั้ง provider unavailable เมื่อ adapter ใช้ไม่ได้
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
| `apw browser login gemini` | เปิด browser profile ของ apw ให้ผู้ใช้เข้าสู่ระบบ |
| `apw provider list` / `doctor` | ดู capability และตรวจความพร้อมของ API/local/browser |
| `apw skill list` / `new` / `inspect` / `enable` | จัดการ skill และตรวจสิทธิ์ก่อนเปิดใช้ |

ค่าเริ่มต้นสำหรับคำสั่งแก้ไขที่ไม่ระบุ mode ให้เป็น preview ในช่วง MVP ทั้งใน CLI และแชต เจ้าของสามารถเปิด execute policy สำหรับ workspace ที่เลือกได้ ส่วน interactive CLI REPL (`apw` เปล่า) เพิ่มหลัง flow ผ่านแชตเสถียร

ตัวอย่างประสบการณ์ใช้งานเป้าหมาย (รูปแบบคำสั่งจริงขึ้นกับ platform adapter):

```text
ผู้ใช้: /project apw-agent
ผู้ใช้: /model gemini-web Argon
ผู้ใช้: ช่วยวางแผนเพิ่ม timeout ให้ process runner
apw: รับงาน T-42 กำลังใช้ Gemini ผ่าน browser ...
apw: แผนพร้อมแล้ว [ดูแผน] [รันใน worktree] [ยกเลิก]
ผู้ใช้: [รันใน worktree]
apw: แก้ 2 ไฟล์ ผ่าน build/vet/test แล้ว [diff] [report]
```

แต่ละ task เก็บ `plan.json` ถ้ามี, patch/diff, events และ verification report ไว้ใต้ task directory ที่ config กำหนด ข้อมูลเครื่องไม่ commit กลับ repository โดยอัตโนมัติ

## 7. งานสัปดาห์แรก

| วัน | งาน | สิ่งที่ตรวจดูได้เมื่อจบวัน |
| --- | --- | --- |
| 1 | เตรียม Go skeleton, contracts และ fixture; เปิด browser profile ทดลอง login | project เริ่มได้; รู้ข้อจำกัดบัญชี/โมเดลและ browser approach |
| 2 | Browser round trip: เลือกโมเดล ส่ง prompt รอจบ อ่าน response | หลักฐาน request/turn ตรงกัน รวม code block และ multi-turn |
| 3 | Telegram bot proof of concept, config และ owner/session binding | bot รับและตอบข้อความใน private chat ทดสอบได้ |
| 4 | SQLite/Windows spike, mock task lifecycle และ runner/worktree prototype | ข้อสรุป storage/worker และ demo mock ที่ไม่แก้ source workspace |
| 5 | รวม spike findings, CI และทบทวน scope/เวลา | ระบุ browser feasibility จริง พร้อม backlog M1 และความเสี่ยงที่ต้องแก้ |

สัปดาห์แรกเน้นพิสูจน์สอง dependency สำคัญคือแชตและเว็บโมเดล ส่วน surgeon ยังเป็น prototype หากติดบัญชี/login/driver ต้องแจ้งตั้งแต่จุดนี้ ไม่รอถึงปลายโครงการ และไม่ถือว่า mock/API ทดแทน browser acceptance ได้

## 8. การทดสอบและเกณฑ์ release

| กลุ่ม | กรณีสำคัญ |
| --- | --- |
| Patch correctness | symbol ซ้ำ, hash เปลี่ยน, expected text ไม่ตรง, patch ทับกัน, CRLF, Unicode, parse ไม่ผ่าน และ rollback หลายไฟล์ |
| Workspace safety | path escape, symlink/reparse point, dirty repo, task ชนกัน และ base เปลี่ยนก่อนนำผลกลับ |
| Provider | JSON ผิด, field เกิน, response ไม่ครบ, timeout, rate limit, token budget หมด และ capability ไม่ตรง |
| Browser provider | model lock/fallback, login/CAPTCHA, quota, DOM เปลี่ยน, turn เก่า, partial answer, code fence/CRLF, cancel และ crash หลัง submit |
| Chat gateway | event ซ้ำ, reconnect, rate limit, owner ที่ไม่ได้รับสิทธิ์, task approval replay, wrong thread, context ปะปน และ outbox ส่งไม่สำเร็จ |
| Runner | command ไม่อนุญาต, output มาก, exit code ผิด, deadline และ cancel ที่มี child process |
| Planner | cycle, dependency หาย, scope เปลี่ยน, step ตก, stale plan และ final verification ล้มเหลว |
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
- Browser provider ผ่าน live acceptance บนบัญชีที่ตั้งค่าไว้ โดยปิด API fallback และนำ response ไปสร้าง patch/verification ได้จริง; ระบุ model observation ตามหลักฐาน ไม่เรียกทุก response ว่า Argon
- การบังคับ logout, disconnect, quota และ UI ไม่ตรง adapter ต้องให้สถานะที่เข้าใจได้ และไม่มีการ apply คำตอบที่ยังไม่ครบหรือถูกยกเลิก

## 9. ตัวชี้วัดและความเสี่ยง

เป้าหมาย router `<10ms`, index update `<5ms`, reflex `100–300ms` และ fast-path `75%` จากสเปกเดิมให้ถือเป็นสมมติฐานสำหรับ benchmark ยังไม่ใช้เป็นคำรับประกันหรือ release gate

ต้องวัด p50/p95 แยกเวลา queue, routing, context/index, model, patch และ verification พร้อมระบุเครื่อง ขนาด repo จำนวนไฟล์ที่เปลี่ยน model/provider และ warm/cold state ค่า routing ไม่รวมเวลาเรียก LLM ส่วน reflex ต้องรายงานทั้งเวลา model และเวลาจบงาน

แยก browser provider ออกจาก API/local ใน benchmark เพิ่ม login readiness, submit-to-complete, response extraction success และ model-selection confidence; token/cost ที่ UI ไม่เปิดเผยต้องรายงาน unknown และติดตาม quota status เท่าที่สังเกตได้

เก็บเพิ่มเติม: อัตรางานผ่านครั้งแรก, อัตราผ่านหลัง repair, จำนวน escalation, stale-patch rejection, token/cost ต่อ task, cancel latency และ workspace corruption incidents เป้าหมายของข้อสุดท้ายคือศูนย์

| ความเสี่ยง | วิธีลดความเสี่ยง | จุดตัดสินใจ |
| --- | --- | --- |
| โมเดลเล็กแก้โค้ดไม่ผ่านหรือไม่เร็วตามเป้า | benchmark งานตัวอย่างและ fallback ไป planner/provider ที่เหมาะสมภายใน budget | จบ M2 และก่อนเปิด router ใน M3 |
| index ไม่ตรงกับไฟล์จริง | ใช้ revision/hash ตรวจซ้ำก่อน patch และ re-index เมื่อเปลี่ยน | M1–M3 |
| worktree ถูกเข้าใจว่าเป็น sandbox | แยก trust/policy ของ process; จำกัด scope การรันและยังไม่เปิด remote | M1 และ M4 |
| ระบบใหญ่เกินกำลัง | ยึด Go-first, Telegram-first และ milestone gates; เพิ่ม Discord ตาม M3 และเลื่อน surface อื่นหลัง Beta | ทบทวนทุกสัปดาห์ |
| build/test ช้าเกินไป | วัดก่อน แล้วใช้ targeted checks ระหว่าง repair โดยคง final gate | หลังได้ข้อมูล M2 |
| skill ขยายสิทธิ์หรือพกข้อมูลลับ | candidate/enable, hash, permission, secret filtering และเลื่อน execution ที่ยังจำกัดไม่ได้ | M5 |
| browser UI/login ไม่รองรับ automation ที่เลือก | spike ในสัปดาห์แรก, profile ที่ผู้ใช้ login เอง, live smoke test และ adapter แยก | M0 ก่อนผูก roadmap กับ driver |
| โมเดลที่ขอไม่พร้อมหรือถูกลดระดับ | ตรวจ model observation, strict mode, แสดง quota และให้ fallback ตาม policy ที่เปิดไว้ | M2 และทุก browser request |
| ข้อความซ้ำหรือส่งผลผิดห้อง | durable inbox/outbox, idempotency, conversation ownership และ thread mapping | M1/M3 ก่อนเปิดใช้งานจริง |

## 10. จุดเริ่ม implementation

เริ่มจาก **M0: browser + Telegram feasibility** ตามงานสัปดาห์แรก แล้วทบทวนผลก่อนขึ้น M1 โดยใช้ Telegram-first เป็นสมมติฐานที่เปลี่ยนลำดับเป็น Discord-first ได้ตามความสะดวกของเจ้าของบัญชี ส่วน browser provider ยังคงเป็นงานหลักที่ต้องผ่านเกณฑ์จริง

ลำดับ dependency ของการส่งมอบคือ:

`Browser/Chat feasibility → Safe core + Daemon/Gateway → Telegram + Web-model MVP → Discord + Dual-brain → Reliability → Memory/Skills → Release`

เมื่อจบแต่ละ milestone ให้เก็บ demo, ผลทดสอบ, ข้อจำกัดที่ยังเหลือ และประมาณการเวลาใหม่ แล้วจึงเลือกว่าจะเดินหน้าหรือแก้ gap ที่ขวางเกณฑ์ส่งมอบ
