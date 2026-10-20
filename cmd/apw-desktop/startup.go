package main

import "fmt"

// OpenInitialWorkspace opens the workspace named on the command line, falling back to the last
// one used. It runs before the window loads its first page, so that page is never the chooser
// for a workspace still being opened. Every failure is collected into the returned notice, which
// is also set on h so the chooser shows it.
func OpenInitialWorkspace(h *WorkspaceHost, args []string, last string) (notice string) {
	for _, dir := range []string{WorkspaceArg(args), last} {
		if dir == "" || (h.Current() != "" && samePath(dir, h.Current())) {
			continue
		}
		if err := h.Open(dir); err != nil {
			if notice != "" {
				notice += "\n"
			}
			notice += fmt.Sprintf("เปิด workspace ไม่ได้: %v", err)
			continue
		}
		break
	}
	if notice != "" {
		h.SetNotice(notice)
	}
	return notice
}
