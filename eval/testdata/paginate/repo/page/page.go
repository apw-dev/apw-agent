package page

// Page returns the items of the 1-based page with the given size.
func Page(items []string, page, size int) []string {
	start := (page - 1) * size
	end := start + size - 1
	if start >= len(items) {
		return nil
	}
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
