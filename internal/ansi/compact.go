package ansi

import "strings"

func Compact(text string) string {
	if text == "" {
		return ""
	}

	// Step 1: Remove control characters (keep \r, \n, \t)
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c < 0x20 && c != '\r' && c != '\n' && c != '\t' {
			continue
		}
		if c == 0x7F {
			continue
		}
		b.WriteByte(c)
	}
	cleaned := b.String()

	// Normalize \r\n to \n (CRLF line endings)
	cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")

	// Step 2: \r-overwrite collapse + Step 3: Trailing whitespace — both per line
	lines := strings.Split(cleaned, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if idx := strings.LastIndex(line, "\r"); idx >= 0 {
			line = line[idx+1:]
		}
		line = strings.TrimRight(line, " \t")
		result = append(result, line)
	}
	cleaned = strings.Join(result, "\n")

	// Step 4: Blank line folding — collapse 3+ consecutive \n to 2
	for strings.Contains(cleaned, "\n\n\n") {
		cleaned = strings.ReplaceAll(cleaned, "\n\n\n", "\n\n")
	}

	// Step 5: Trim leading/trailing blank lines
	cleaned = strings.TrimLeft(cleaned, "\n")
	if strings.HasSuffix(cleaned, "\n\n") {
		cleaned = strings.TrimRight(cleaned, "\n")
	}

	return cleaned
}
