package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

const (
	feishuPostContentMaxBytes = 28000
)

var feishuMarkdownParser = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
)

// feishuPostMessageChunks 将 Markdown 转为一条或多条飞书 post 消息的 content JSON 字符串（仅 content 本体，不含 msg_type）。
func feishuPostMessageChunks(md string) ([]string, error) {
	md = strings.TrimSpace(md)
	if md == "" {
		raw, err := json.Marshal(feishuPostEnvelope("", [][]segment{{textSegment("（空回复）", nil)}}))
		if err != nil {
			return nil, err
		}
		return []string{string(raw)}, nil
	}

	src := []byte(md)
	doc := feishuMarkdownParser.Parser().Parse(text.NewReader(src))
	title, rows := feishuExtractPostFromDoc(doc, src)
	if len(rows) == 0 {
		raw, err := json.Marshal(feishuPostEnvelope(title, [][]segment{{textSegment(md, nil)}}))
		if err != nil {
			return nil, err
		}
		return []string{string(raw)}, nil
	}

	return feishuSplitPostChunks(title, rows, feishuPostContentMaxBytes)
}

type segment map[string]interface{}

func textSegment(s string, styles []string) segment {
	m := segment{"tag": "text", "text": s}
	if len(styles) > 0 {
		m["style"] = styles
	}
	return m
}

func linkSegment(href, label string) segment {
	return segment{"tag": "a", "href": href, "text": label}
}

type feishuPostZh struct {
	Title   string      `json:"title,omitempty"`
	Content [][]segment `json:"content"`
}

type feishuPostEnvelopeStruct struct {
	ZhCN feishuPostZh `json:"zh_cn"`
}

func feishuPostEnvelope(title string, content [][]segment) feishuPostEnvelopeStruct {
	return feishuPostEnvelopeStruct{ZhCN: feishuPostZh{Title: title, Content: content}}
}

func feishuExtractPostFromDoc(doc ast.Node, src []byte) (title string, rows [][]segment) {
	var h1 ast.Node
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		if h, ok := c.(*ast.Heading); ok && h.Level == 1 {
			h1 = c
			break
		}
	}
	if h1 != nil {
		title = strings.TrimSpace(renderTextFromNode(h1, src))
	}

	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		if h1 != nil && c == h1 {
			continue
		}
		rows = append(rows, feishuRenderBlock(c, src)...)
	}
	return title, rows
}

func feishuRenderBlock(n ast.Node, src []byte) [][]segment {
	switch n := n.(type) {
	case *ast.Paragraph:
		line := feishuRenderParagraphLine(n, src)
		if len(line) == 0 {
			return nil
		}
		return [][]segment{line}
	case *ast.TextBlock:
		line := feishuRenderParagraphLineFromInlines(n, src)
		if len(line) == 0 {
			return nil
		}
		return [][]segment{line}
	case *ast.Heading:
		if n.Level == 1 {
			return nil
		}
		prefix := strings.Repeat("#", n.Level) + " "
		text := strings.TrimSpace(renderTextFromNode(n, src))
		if text == "" {
			return nil
		}
		return [][]segment{{textSegment(prefix+text, []string{"bold"})}}
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		var raw []byte
		if fc, ok := n.(*ast.FencedCodeBlock); ok {
			raw = fc.Text(src)
		} else if bc, ok := n.(*ast.CodeBlock); ok {
			raw = bc.Text(src)
		}
		body := strings.TrimSuffix(string(raw), "\n")
		if body == "" {
			return [][]segment{{textSegment(" ", nil)}}
		}
		out := make([][]segment, 0)
		for _, ln := range strings.Split(body, "\n") {
			out = append(out, []segment{textSegment(ln, []string{"code"})})
		}
		return out
	case *ast.ThematicBreak:
		return [][]segment{{textSegment("———", nil)}}
	case *ast.Blockquote:
		return feishuRenderBlockquote(n, src)
	case *ast.List:
		return feishuRenderList(n, src, 0)
	case *ast.HTMLBlock:
		t := strings.TrimSpace(string(n.Text(src)))
		if t == "" {
			return nil
		}
		return [][]segment{{textSegment(t, nil)}}
	case *extast.Table:
		return feishuRenderTable(n, src)
	default:
		return nil
	}
}

func feishuRenderTable(t *extast.Table, src []byte) [][]segment {
	var rows [][]segment
	for c := t.FirstChild(); c != nil; c = c.NextSibling() {
		switch blk := c.(type) {
		case *extast.TableHeader:
			rows = append(rows, feishuTableHeaderRow(blk, src)...)
		case *extast.TableRow:
			rows = append(rows, feishuTableRow(blk, src, false)...)
		}
	}
	return rows
}

func feishuTableHeaderRow(th *extast.TableHeader, src []byte) [][]segment {
	var cells []string
	for c := th.FirstChild(); c != nil; c = c.NextSibling() {
		if cell, ok := c.(*extast.TableCell); ok {
			cells = append(cells, strings.TrimSpace(feishuTableCellText(cell, src)))
		}
	}
	line := strings.Join(cells, " | ")
	return [][]segment{{textSegment(line, []string{"bold"})}}
}

func feishuTableCellText(cell *extast.TableCell, src []byte) string {
	var buf strings.Builder
	for i := 0; i < cell.Lines().Len(); i++ {
		line := cell.Lines().At(i)
		buf.Write(line.Value(src))
		if i+1 < cell.Lines().Len() {
			buf.WriteByte('\n')
		}
	}
	return buf.String()
}

func feishuTableRow(tr *extast.TableRow, src []byte, header bool) [][]segment {
	var cells []string
	for c := tr.FirstChild(); c != nil; c = c.NextSibling() {
		if cell, ok := c.(*extast.TableCell); ok {
			cells = append(cells, strings.TrimSpace(feishuTableCellText(cell, src)))
		}
	}
	line := strings.Join(cells, " | ")
	if header {
		return [][]segment{{textSegment(line, []string{"bold"})}}
	}
	return [][]segment{{textSegment(line, nil)}}
}

func feishuRenderBlockquote(bq *ast.Blockquote, src []byte) [][]segment {
	var acc [][]segment
	for c := bq.FirstChild(); c != nil; c = c.NextSibling() {
		sub := feishuRenderBlock(c, src)
		for _, row := range sub {
			pref := make([]segment, len(row))
			copy(pref, row)
			if len(row) > 0 {
				if t, ok := row[0]["text"].(string); ok {
					pref[0] = textSegment("> "+t, styleFromSeg(row[0]))
				}
			}
			acc = append(acc, pref)
		}
	}
	return acc
}

func styleFromSeg(s segment) []string {
	if v, ok := s["style"].([]string); ok {
		return v
	}
	return nil
}

func feishuRenderList(list *ast.List, src []byte, depth int) [][]segment {
	var out [][]segment
	num := list.Start
	if !list.IsOrdered() {
		num = 0
	}
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		li, ok := item.(*ast.ListItem)
		if !ok {
			continue
		}
		var prefix string
		if list.IsOrdered() {
			prefix = fmt.Sprintf("%s%d. ", strings.Repeat("  ", depth), num)
			num++
		} else {
			prefix = strings.Repeat("  ", depth) + "• "
		}
		for c := li.FirstChild(); c != nil; c = c.NextSibling() {
			switch cn := c.(type) {
			case *ast.List:
				out = append(out, feishuRenderList(cn, src, depth+1)...)
			default:
				sub := feishuRenderBlock(c, src)
				for si, row := range sub {
					if len(row) == 0 {
						continue
					}
					if si == 0 {
						if t, ok := row[0]["text"].(string); ok {
							row[0] = textSegment(prefix+t, styleFromSeg(row[0]))
						}
					}
					out = append(out, row)
				}
			}
		}
	}
	return out
}

func feishuRenderParagraphLine(p *ast.Paragraph, src []byte) []segment {
	return feishuRenderParagraphLineFromInlines(p, src)
}

// feishuRenderParagraphLineFromInlines 对 Paragraph、TextBlock 等含行内子节点的块生效。
func feishuRenderParagraphLineFromInlines(block ast.Node, src []byte) []segment {
	var segs []segment
	for c := block.FirstChild(); c != nil; c = c.NextSibling() {
		segs = append(segs, feishuRenderInline(c, src)...)
	}
	return mergeAdjacentTextSegs(segs)
}

func mergeAdjacentTextSegs(segs []segment) []segment {
	if len(segs) <= 1 {
		return segs
	}
	out := make([]segment, 0, len(segs))
	for _, s := range segs {
		tag, _ := s["tag"].(string)
		if tag != "text" || len(out) == 0 {
			out = append(out, s)
			continue
		}
		last := out[len(out)-1]
		if lt, ok := last["tag"].(string); ok && lt == "text" {
			s1, _ := last["text"].(string)
			s2, _ := s["text"].(string)
			st1, _ := last["style"].([]string)
			st2, _ := s["style"].([]string)
			if feishuStyleEqual(st1, st2) {
				last["text"] = s1 + s2
				out[len(out)-1] = last
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

func feishuStyleEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func feishuRenderInline(n ast.Node, src []byte) []segment {
	switch n := n.(type) {
	case *ast.Text:
		s := string(n.Segment.Value(src))
		segs := []segment{textSegment(s, nil)}
		if n.SoftLineBreak() || n.HardLineBreak() {
			segs = append(segs, textSegment("\n", nil))
		}
		return segs
	case *ast.CodeSpan:
		t := strings.TrimSpace(string(n.Text(src)))
		return []segment{textSegment(t, []string{"code"})}
	case *ast.Emphasis:
		t := strings.TrimSpace(renderTextFromNode(n, src))
		if n.Level >= 2 {
			return []segment{textSegment(t, []string{"bold"})}
		}
		return []segment{textSegment(t, []string{"italic"})}
	case *ast.Link:
		label := strings.TrimSpace(renderTextFromNode(n, src))
		href := strings.TrimSpace(string(n.Destination))
		if href == "" {
			return []segment{textSegment(label, nil)}
		}
		if label == "" {
			label = href
		}
		return []segment{linkSegment(href, label)}
	case *ast.Image:
		alt := strings.TrimSpace(string(n.Text(src)))
		return []segment{textSegment("[图片: "+alt+"]", nil)}
	case *ast.RawHTML:
		return []segment{textSegment(string(n.Text(src)), nil)} // 降级为纯文本
	default:
		t := strings.TrimSpace(renderTextFromNode(n, src))
		if t == "" {
			return nil
		}
		return []segment{textSegment(t, nil)}
	}
}

func renderTextFromNode(n ast.Node, src []byte) string {
	var buf bytes.Buffer
	_ = ast.Walk(n, func(nd ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := nd.(*ast.Text); ok {
			buf.Write(t.Segment.Value(src))
		}
		return ast.WalkContinue, nil
	})
	return buf.String()
}

func feishuSplitPostChunks(title string, rows [][]segment, maxPayload int) ([]string, error) {
	var chunks []string
	batch := make([][]segment, 0)
	useTitle := title

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		env := feishuPostEnvelope(useTitle, batch)
		useTitle = ""
		raw, err := json.Marshal(env)
		if err != nil {
			return err
		}
		chunks = append(chunks, string(raw))
		batch = batch[:0]
		return nil
	}

	for _, row := range rows {
		try := append(append([][]segment{}, batch...), row)
		env := feishuPostEnvelope(useTitle, try)
		raw, err := json.Marshal(env)
		if err != nil {
			return nil, err
		}
		if len(raw) <= maxPayload {
			batch = try
			continue
		}
		if err := flush(); err != nil {
			return nil, err
		}
		env2 := feishuPostEnvelope(useTitle, [][]segment{row})
		raw2, err := json.Marshal(env2)
		if err != nil {
			return nil, err
		}
		if len(raw2) <= maxPayload {
			batch = [][]segment{row}
			continue
		}
		useTitle = ""
		flat := feishuFlattenParagraphRow(row)
		for _, p := range feishuSplitLongText(flat, maxPayload/8) {
			env3 := feishuPostEnvelope("", [][]segment{{textSegment(p, nil)}})
			b, err := json.Marshal(env3)
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, string(b))
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		b, err := json.Marshal(feishuPostEnvelope(title, [][]segment{{textSegment(" ", nil)}}))
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, string(b))
	}
	return chunks, nil
}

func feishuFlattenParagraphRow(row []segment) string {
	var b strings.Builder
	for _, s := range row {
		if t, ok := s["text"].(string); ok {
			b.WriteString(t)
		}
	}
	return b.String()
}

func feishuSplitLongText(s string, lim int) []string {
	if lim <= 8 {
		lim = 512
	}
	runes := []rune(s)
	if len(runes) <= lim {
		if strings.TrimSpace(s) == "" {
			return []string{" "}
		}
		return []string{s}
	}
	var out []string
	for len(runes) > 0 {
		if len(runes) <= lim {
			out = append(out, string(runes))
			break
		}
		out = append(out, string(runes[:lim]))
		runes = runes[lim:]
	}
	return out
}
