package skills

import (
	"bytes"
	"path/filepath"
	"strings"

	xhtml "golang.org/x/net/html"
)

// ParsedDoc is a normalized representation of an input document.
type ParsedDoc struct {
	Path  string
	Title string
	Text  string
}

// ParseDocument parses content based on the provided path's extension.
func ParseDocument(path string, content []byte) (ParsedDoc, error) {
	ext := strings.ToLower(filepath.Ext(path))
	out := ParsedDoc{Path: path}

	switch ext {
	case ".md", ".markdown":
		title, text := parseMarkdown(string(content))
		out.Title = title
		out.Text = text
		return out, nil
	case ".html", ".htm":
		out.Title = ""
		out.Text = parseHTML(string(content))
		return out, nil
	case ".txt":
		out.Title = ""
		out.Text = string(content)
		return out, nil
	default:
		// Unknown: treat as plain text.
		out.Title = ""
		out.Text = string(content)
		return out, nil
	}
}

func parseMarkdown(in string) (title string, text string) {
	s := normalizeNewlines(in)

	// Tolerate UTF-8 BOM at the start of the document.
	s = strings.TrimPrefix(s, "\ufeff")

	lines := strings.Split(s, "\n")

	// Tolerate leading blank lines before front matter.
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}

	// Strip YAML front matter.
	if start < len(lines) && strings.TrimSpace(lines[start]) == "---" {
		end := -1
		for i := start + 1; i < len(lines); i++ {
			trim := strings.TrimSpace(lines[i])
			if trim == "---" || trim == "..." {
				end = i
				break
			}
		}
		if end != -1 {
			lines = lines[end+1:]
			// Drop a single empty line after front matter.
			if len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
				lines = lines[1:]
			}
			s = strings.Join(lines, "\n")
		}
	}

	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "# ") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			break
		}
	}

	return title, s
}

func parseHTML(in string) string {
	lowerIn := strings.ToLower(in)

	tok := xhtml.NewTokenizer(strings.NewReader(in))

	var out bytes.Buffer
	needSpace := false

	pos := 0 // byte offset into the original input (best-effort via tok.Raw())

	inRaw := false
	rawTag := ""

	sanitizeText := func(s string) string {
		// Ensure downstream chunking never sees angle brackets.
		s = strings.ReplaceAll(s, "<", " ")
		s = strings.ReplaceAll(s, ">", " ")
		return s
	}

	isTagBoundary := func(lower string, at int, needle string) bool {
		after := at + len(needle)
		if after >= len(lower) {
			return true
		}
		c := lower[after]
		return !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_')
	}

	findNextTag := func(lower string, from int, needle string) int {
		i := from
		for {
			p := strings.Index(lower[i:], needle)
			if p == -1 {
				return -1
			}
			p += i
			if isTagBoundary(lower, p, needle) {
				return p
			}
			i = p + 1
		}
	}

	hasAnotherCloseBeforeNextOpen := func(tag string, from int) bool {
		nextOpen := findNextTag(lowerIn, from, "<"+tag)
		nextClose := findNextTag(lowerIn, from, "</"+tag)
		if nextClose == -1 {
			return false
		}
		if nextOpen == -1 {
			return true
		}
		return nextClose < nextOpen
	}

	for {
		tt := tok.Next()
		raw := tok.Raw()
		tokEnd := pos + len(raw)

		switch tt {
		case xhtml.ErrorToken:
			return strings.Join(strings.Fields(out.String()), " ")

		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			name, _ := tok.TagName()
			tag := strings.ToLower(string(name))
			if (tag == "script" || tag == "style") && tt == xhtml.StartTagToken {
				inRaw = true
				rawTag = tag
			}

		case xhtml.EndTagToken:
			name, _ := tok.TagName()
			tag := strings.ToLower(string(name))
			if inRaw && tag == rawTag {
				if !hasAnotherCloseBeforeNextOpen(rawTag, tokEnd) {
					inRaw = false
					rawTag = ""
				}
			}

		case xhtml.TextToken:
			if inRaw {
				break
			}

			txt := sanitizeText(string(tok.Text()))
			if strings.TrimSpace(txt) == "" {
				needSpace = true
				break
			}
			if needSpace && out.Len() > 0 {
				out.WriteByte(' ')
			}
			needSpace = false
			out.WriteString(txt)
		}

		pos = tokEnd
	}
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}
