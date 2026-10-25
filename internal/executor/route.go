package executor

import (
	"regexp"
	"strings"
)

// Request routing: a plain question ("how do I get CROWDSEC_BOUNCER_KEY?") is answered from the
// source in one read-only model call instead of planning, editing and verifying a change.
// Routing is rule based (no model call) and conservative: anything that asks for a change, or is
// not clearly a question, keeps the requested mode.

// Route modes (Executor.Route).
const (
	RouteOff  = "off"
	RouteAuto = "auto"
)

var (
	questionEnglish = regexp.MustCompile(`(?i)(^|[^a-z])(how|what|why|where|which|when|who|whose|is there|are there|does|do i|can i|should i|explain|describe|tell me)([^a-z]|$)`)
	editEnglish     = regexp.MustCompile(`(?i)(^|[^a-z])(add|fix|change|create|implement|refactor|update|remove|delete|rename|write|generate|make|build|convert|replace|move|migrate|upgrade|install|patch|edit|modify|set up|setup|document)([^a-z]|$)`)
	questionThai    = []string{"ยังไง", "อย่างไร", "อะไร", "ทำไม", "ที่ไหน", "ตรงไหน", "เมื่อไหร่", "ไหม", "หรือไม่", "หรือเปล่า", "รึเปล่า", "ได้มา", "คือ", "แปลว่า", "อธิบาย", "หมายความ", "เท่าไหร่", "แบบไหน", "อันไหน"}
	editThai        = []string{"แก้", "เพิ่ม", "สร้าง", "ลบ", "เปลี่ยน", "ปรับ", "เขียน", "ทำให้", "ติดตั้ง", "อัปเดต", "อัพเดต", "ย้าย", "แทนที่", "ใส่", "เอาออก", "ตั้งค่า", "จัดการ", "รีแฟกเตอร์", "บันทึก"}
)

// isQuestion reports whether instruction only asks for information: it has a question marker and
// no request to change anything.
func isQuestion(instruction string) bool {
	text := strings.TrimSpace(instruction)
	if text == "" || len(text) > 2000 {
		return false
	}
	if editEnglish.MatchString(text) {
		return false
	}
	for _, w := range editThai {
		if strings.Contains(text, w) {
			return false
		}
	}
	if strings.Contains(text, "?") || questionEnglish.MatchString(text) {
		return true
	}
	for _, w := range questionThai {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}

// answerPrompt asks for a direct, evidence-based answer to a question about the workspace.
func answerPrompt(question, overview string) string {
	return "Answer the user's question from the supplied source and project files. Do not run tools or commands, do not modify files and do not propose edits unless the question asks for them. Be concise and practical: give the steps or facts first, cite file paths (file:line) for evidence, and say what the source does not confirm. Answer in the language of the question.\nQuestion: " + question + "\n" + overview
}
