// Файл: internal/normalize/docx_text.go
package normalize

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// docxWalker собирает текст абзацев; таблицы верхнего уровня — Markdown-таблицами (вложенные — текстом ячейки).
type docxWalker struct {
	out, para strings.Builder
	rows      [][]string
	row, cell []string
	depth     int
	inText    bool
}

// docxText — текст document.xml.
func docxText(r io.Reader) (string, error) {
	dec := xml.NewDecoder(r)
	w := &docxWalker{}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			w.start(t.Name.Local)
		case xml.CharData:
			if w.inText {
				w.para.Write(t)
			}
		case xml.EndElement:
			w.end(t.Name.Local)
		}
	}
	return strings.TrimSpace(collapseNewlines(w.out.String())), nil
}

func (w *docxWalker) start(name string) {
	switch name {
	case "tbl":
		w.depth++
		if w.depth == 1 {
			w.rows = nil
		}
	case "tr":
		if w.depth == 1 {
			w.row = nil
		}
	case "tc":
		if w.depth == 1 {
			w.cell = nil
		}
	case "p":
		w.para.Reset()
	case "t":
		w.inText = true
	case "tab":
		w.para.WriteString("\t")
	case "br", "cr":
		w.para.WriteString("\n")
	}
}

func (w *docxWalker) end(name string) {
	switch name {
	case "t":
		w.inText = false
	case "p":
		w.endParagraph()
	case "tc":
		if w.depth == 1 {
			w.row = append(w.row, strings.Join(w.cell, " "))
		}
	case "tr":
		if w.depth == 1 {
			w.rows = append(w.rows, w.row)
		}
	case "tbl":
		if w.depth == 1 {
			w.out.WriteString("\n" + markdownTable(w.rows) + "\n\n")
		}
		w.depth--
	}
}

func (w *docxWalker) endParagraph() {
	if w.depth > 0 {
		if s := strings.TrimSpace(w.para.String()); s != "" {
			w.cell = append(w.cell, s)
		}
		return
	}
	w.out.WriteString(w.para.String())
	w.out.WriteString("\n")
}

// collapseNewlines — три и больше переводов строки подряд → два (замена регулярного выражения doc-agent).
func collapseNewlines(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

func markdownTable(rows [][]string) string {
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	if cols == 0 {
		return ""
	}
	line := func(r []string) string {
		cells := make([]string, cols)
		for i := range cells {
			if i < len(r) {
				cells[i] = strings.NewReplacer("|", `\|`, "\n", " ").Replace(r[i])
			}
		}
		return "| " + strings.Join(cells, " | ") + " |"
	}
	lines := []string{line(rows[0]), "|" + strings.Repeat(" --- |", cols)}
	for _, r := range rows[1:] {
		lines = append(lines, line(r))
	}
	return strings.Join(lines, "\n")
}
