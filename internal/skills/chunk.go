package skills

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Chunk is a bounded piece of document text for downstream embedding/ingestion.
type Chunk struct {
	ChunkID string
	Text    string
}

// ChunkText splits doc.Text by paragraphs and packs them into chunks.
func ChunkText(doc ParsedDoc, maxChars int) []Chunk {
	text := normalizeNewlines(doc.Text)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if maxChars <= 0 {
		maxChars = 1
	}

	paragraphs := splitParagraphs(text)
	var out []Chunk
	id := 1

	emit := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		out = append(out, Chunk{
			ChunkID: fmt.Sprintf("chunk_%04d", id),
			Text:    s,
		})
		id++
	}

	var cur string
	curRunes := 0

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pRunes := runeLen(p)

		// If this paragraph alone exceeds maxChars, flush current and split.
		if pRunes > maxChars {
			if strings.TrimSpace(cur) != "" {
				emit(cur)
				cur = ""
				curRunes = 0
			}
			for len(p) > 0 {
				seg, rest := takeRunes(p, maxChars)
				emit(seg)
				p = strings.TrimLeftFunc(rest, func(r rune) bool {
					return r == '\n' || r == '\t' || r == ' '
				})
			}
			continue
		}

		if strings.TrimSpace(cur) == "" {
			cur = p
			curRunes = pRunes
			continue
		}

		candidateRunes := curRunes + 2 + pRunes // "\n\n" separator
		if candidateRunes <= maxChars {
			cur = cur + "\n\n" + p
			curRunes = candidateRunes
			continue
		}

		emit(cur)
		cur = p
		curRunes = pRunes
	}

	if strings.TrimSpace(cur) != "" {
		emit(cur)
	}

	return out
}

func splitParagraphs(s string) []string {
	var paras []string
	var buf []string
	for _, line := range strings.Split(normalizeNewlines(s), "\n") {
		if strings.TrimSpace(line) == "" {
			if len(buf) > 0 {
				paras = append(paras, strings.Join(buf, "\n"))
				buf = nil
			}
			continue
		}
		buf = append(buf, strings.TrimRight(line, " \t"))
	}
	if len(buf) > 0 {
		paras = append(paras, strings.Join(buf, "\n"))
	}
	return paras
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

func takeRunes(s string, n int) (head string, tail string) {
	if n <= 0 {
		return "", s
	}
	if runeLen(s) <= n {
		return s, ""
	}
	// Walk byte indices by rune.
	count := 0
	i := 0
	for i < len(s) {
		_, size := utf8.DecodeRuneInString(s[i:])
		if size == 0 {
			break
		}
		count++
		i += size
		if count >= n {
			break
		}
	}
	if i > len(s) {
		i = len(s)
	}
	return s[:i], s[i:]
}
