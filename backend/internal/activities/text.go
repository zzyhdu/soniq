package activities

func truncateRunes(value string, limit int) string {
	if limit < 0 {
		limit = 0
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
