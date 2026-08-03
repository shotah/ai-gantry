package telegram

import (
	"bytes"
	"html"
	"strconv"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// markdownToTelegramHTML converts CommonMark/GFM into Telegram's HTML subset.
// Always returns sendable text: on parse/render failure it falls back to escaped plain.
func markdownToTelegramHTML(src string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := telegramMarkdown.Convert([]byte(src), &buf); err != nil {
		return html.EscapeString(src)
	}
	out := strings.TrimSpace(buf.String())
	if out == "" {
		return html.EscapeString(src)
	}
	return out
}

var (
	telegramMarkdownOnce sync.Once
	telegramMarkdown     goldmark.Markdown
)

func init() {
	telegramMarkdownOnce.Do(func() {
		// Renderer first, then GFM (parsers + HTML renderers). goldmark registers
		// NodeRenderers low-priority-last, so priority 0 overrides GFM's 500 and
		// replaces <table>/<del> defaults with Telegram-safe tags.
		telegramMarkdown = goldmark.New(
			goldmark.WithRenderer(renderer.NewRenderer(
				renderer.WithNodeRenderers(util.Prioritized(&tgHTMLRenderer{}, 0)),
			)),
			goldmark.WithExtensions(extension.GFM),
		)
	})
}

// tgHTMLRenderer emits only tags Telegram's HTML parse mode accepts.
// Unsupported nodes degrade to plain/escaped text instead of failing the message.
type tgHTMLRenderer struct{}

func (r *tgHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindDocument, r.renderPassthrough)
	reg.Register(ast.KindParagraph, r.renderParagraph)
	reg.Register(ast.KindTextBlock, r.renderTextBlock)
	reg.Register(ast.KindHeading, r.renderHeading)
	reg.Register(ast.KindEmphasis, r.renderEmphasis)
	reg.Register(ast.KindCodeSpan, r.renderCodeSpan)
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(ast.KindList, r.renderPassthrough)
	reg.Register(ast.KindListItem, r.renderListItem)
	reg.Register(ast.KindThematicBreak, r.renderThematicBreak)
	reg.Register(ast.KindLink, r.renderLink)
	reg.Register(ast.KindAutoLink, r.renderAutoLink)
	reg.Register(ast.KindImage, r.renderImage)
	reg.Register(ast.KindText, r.renderText)
	reg.Register(ast.KindString, r.renderString)
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	reg.Register(ast.KindRawHTML, r.renderRawHTML)

	reg.Register(east.KindStrikethrough, r.renderStrikethrough)
	reg.Register(east.KindTaskCheckBox, r.renderTaskCheckBox)
	reg.Register(east.KindTable, r.renderTable)
	reg.Register(east.KindTableHeader, r.renderPassthrough)
	reg.Register(east.KindTableRow, r.renderTableRow)
	reg.Register(east.KindTableCell, r.renderTableCell)
}

func (r *tgHTMLRenderer) renderPassthrough(util.BufWriter, []byte, ast.Node, bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderParagraph(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		return ast.WalkContinue, nil
	}
	if n.NextSibling() != nil {
		_, _ = w.WriteString("\n\n")
	}
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderTextBlock(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering && n.NextSibling() != nil && n.FirstChild() != nil {
		_ = w.WriteByte('\n')
	}
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderHeading(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<b>")
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString("</b>")
	if n.NextSibling() != nil {
		_, _ = w.WriteString("\n\n")
	}
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderEmphasis(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Emphasis)
	tag := "i"
	if n.Level >= 2 {
		tag = "b"
	}
	if entering {
		_, _ = w.WriteString("<" + tag + ">")
	} else {
		_, _ = w.WriteString("</" + tag + ">")
	}
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderStrikethrough(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<s>")
	} else {
		_, _ = w.WriteString("</s>")
	}
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderCodeSpan(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString("<code>")
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			_, _ = w.Write(util.EscapeHTML(t.Segment.Value(source)))
		}
	}
	_, _ = w.WriteString("</code>")
	return ast.WalkSkipChildren, nil
}

func (r *tgHTMLRenderer) renderCodeBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString("<pre>")
	writeCodeLines(w, source, n)
	_, _ = w.WriteString("</pre>")
	if n.NextSibling() != nil {
		_ = w.WriteByte('\n')
	}
	return ast.WalkSkipChildren, nil
}

func (r *tgHTMLRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.FencedCodeBlock)
	lang := n.Language(source)
	if len(lang) > 0 {
		_, _ = w.WriteString("<pre><code class=\"language-")
		_, _ = w.Write(util.EscapeHTML(lang))
		_, _ = w.WriteString("\">")
		writeCodeLines(w, source, n)
		_, _ = w.WriteString("</code></pre>")
	} else {
		_, _ = w.WriteString("<pre>")
		writeCodeLines(w, source, n)
		_, _ = w.WriteString("</pre>")
	}
	if n.NextSibling() != nil {
		_ = w.WriteByte('\n')
	}
	return ast.WalkSkipChildren, nil
}

func writeCodeLines(w util.BufWriter, source []byte, n ast.Node) {
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		_, _ = w.Write(util.EscapeHTML(line.Value(source)))
	}
}

func (r *tgHTMLRenderer) renderBlockquote(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<blockquote>")
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString("</blockquote>")
	if n.NextSibling() != nil {
		_ = w.WriteByte('\n')
	}
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderListItem(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		marker := "• "
		if p, ok := n.Parent().(*ast.List); ok && p.IsOrdered() {
			i := 0
			for s := p.FirstChild(); s != nil && s != n; s = s.NextSibling() {
				i++
			}
			marker = strconv.Itoa(p.Start+i) + ". "
		}
		_, _ = w.WriteString(marker)
		return ast.WalkContinue, nil
	}
	if n.NextSibling() != nil {
		_ = w.WriteByte('\n')
	} else if n.Parent() != nil && n.Parent().NextSibling() != nil {
		_, _ = w.WriteString("\n\n")
	}
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderThematicBreak(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString("———")
	if n.NextSibling() != nil {
		_, _ = w.WriteString("\n\n")
	}
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderLink(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Link)
	if entering {
		dest := string(n.Destination)
		if dest == "" || isDangerousURL(dest) {
			return ast.WalkContinue, nil
		}
		_, _ = w.WriteString(`<a href="`)
		_, _ = w.WriteString(html.EscapeString(dest))
		_, _ = w.WriteString(`">`)
		return ast.WalkContinue, nil
	}
	dest := string(n.Destination)
	if dest != "" && !isDangerousURL(dest) {
		_, _ = w.WriteString("</a>")
	}
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderAutoLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.AutoLink)
	label := n.Label(source)
	url := string(label)
	if n.AutoLinkType == ast.AutoLinkEmail {
		url = "mailto:" + url
	}
	if isDangerousURL(url) {
		_, _ = w.Write(util.EscapeHTML(label))
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString(`<a href="`)
	_, _ = w.WriteString(html.EscapeString(url))
	_, _ = w.WriteString(`">`)
	_, _ = w.Write(util.EscapeHTML(label))
	_, _ = w.WriteString("</a>")
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderImage(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Image)
	dest := string(n.Destination)
	// Images are handled elsewhere via ExtractImageURLs; render as a link/alt here.
	var alt strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			alt.Write(t.Segment.Value(source))
		}
	}
	label := strings.TrimSpace(alt.String())
	if label == "" {
		label = strings.TrimSpace(string(n.Title))
	}
	if label == "" {
		label = "image"
	}
	if dest != "" && !isDangerousURL(dest) {
		_, _ = w.WriteString(`<a href="`)
		_, _ = w.WriteString(html.EscapeString(dest))
		_, _ = w.WriteString(`">`)
		_, _ = w.WriteString(html.EscapeString(label))
		_, _ = w.WriteString("</a>")
	} else {
		_, _ = w.WriteString(html.EscapeString(label))
	}
	return ast.WalkSkipChildren, nil
}

func (r *tgHTMLRenderer) renderText(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Text)
	segment := n.Segment
	if n.IsRaw() {
		_, _ = w.Write(util.EscapeHTML(segment.Value(source)))
	} else {
		_, _ = w.Write(util.EscapeHTML(segment.Value(source)))
		if n.HardLineBreak() {
			_ = w.WriteByte('\n')
		} else if n.SoftLineBreak() {
			_ = w.WriteByte('\n')
		}
	}
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderString(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.String)
	_, _ = w.Write(util.EscapeHTML(n.Value))
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.HTMLBlock)
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		_, _ = w.Write(util.EscapeHTML(line.Value(source)))
	}
	if n.NextSibling() != nil {
		_, _ = w.WriteString("\n\n")
	}
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderRawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.RawHTML)
	_, _ = w.Write(util.EscapeHTML(n.Segments.Value(source)))
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderTaskCheckBox(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*east.TaskCheckBox)
	if n.IsChecked {
		_, _ = w.WriteString("✅ ")
	} else {
		_, _ = w.WriteString("☐ ")
	}
	return ast.WalkContinue, nil
}

// Tables: Telegram has no <table> tags, and <pre> cannot nest <a>, so we emit
// pipe-separated rows with real HTML (links stay tappable).
func (r *tgHTMLRenderer) renderTable(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	var rows []string
	for row := node.FirstChild(); row != nil; row = row.NextSibling() {
		// TableHeader's children are cells (not a nested TableRow).
		if row.Kind() == east.KindTableHeader {
			rows = append(rows, "<b>"+tableRowHTML(source, row)+"</b>")
			continue
		}
		if row.Kind() == east.KindTableRow {
			rows = append(rows, tableRowHTML(source, row))
		}
	}
	if len(rows) == 0 {
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString(strings.Join(rows, "\n"))
	if node.NextSibling() != nil {
		_, _ = w.WriteString("\n\n")
	}
	return ast.WalkSkipChildren, nil
}

func (r *tgHTMLRenderer) renderTableRow(util.BufWriter, []byte, ast.Node, bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}

func (r *tgHTMLRenderer) renderTableCell(util.BufWriter, []byte, ast.Node, bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}

func tableRowHTML(source []byte, row ast.Node) string {
	var cells []string
	for c := row.FirstChild(); c != nil; c = c.NextSibling() {
		cells = append(cells, strings.TrimSpace(tableCellHTML(source, c)))
	}
	return strings.Join(cells, " | ")
}

func tableCellHTML(source []byte, cell ast.Node) string {
	var b strings.Builder
	_ = ast.Walk(cell, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		switch n := n.(type) {
		case *ast.Text:
			if entering {
				b.WriteString(html.EscapeString(string(n.Segment.Value(source))))
				if n.HardLineBreak() || n.SoftLineBreak() {
					b.WriteByte('\n')
				}
			}
		case *ast.String:
			if entering {
				b.WriteString(html.EscapeString(string(n.Value)))
			}
		case *ast.Link:
			dest := string(n.Destination)
			safe := dest != "" && !isDangerousURL(dest)
			if entering {
				if safe {
					b.WriteString(`<a href="`)
					b.WriteString(html.EscapeString(dest))
					b.WriteString(`">`)
				}
			} else if safe {
				b.WriteString("</a>")
			}
		case *ast.AutoLink:
			if !entering {
				return ast.WalkContinue, nil
			}
			label := n.Label(source)
			url := string(label)
			if n.AutoLinkType == ast.AutoLinkEmail {
				url = "mailto:" + url
			}
			if isDangerousURL(url) {
				b.WriteString(html.EscapeString(string(label)))
			} else {
				b.WriteString(`<a href="`)
				b.WriteString(html.EscapeString(url))
				b.WriteString(`">`)
				b.WriteString(html.EscapeString(string(label)))
				b.WriteString("</a>")
			}
			return ast.WalkSkipChildren, nil
		case *ast.Emphasis:
			tag := "i"
			if n.Level >= 2 {
				tag = "b"
			}
			if entering {
				b.WriteString("<" + tag + ">")
			} else {
				b.WriteString("</" + tag + ">")
			}
		case *ast.CodeSpan:
			if !entering {
				return ast.WalkContinue, nil
			}
			b.WriteString("<code>")
			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				if t, ok := c.(*ast.Text); ok {
					b.WriteString(html.EscapeString(string(t.Segment.Value(source))))
				}
			}
			b.WriteString("</code>")
			return ast.WalkSkipChildren, nil
		case *east.Strikethrough:
			if entering {
				b.WriteString("<s>")
			} else {
				b.WriteString("</s>")
			}
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

func isDangerousURL(u string) bool {
	l := strings.ToLower(strings.TrimSpace(u))
	return strings.HasPrefix(l, "javascript:") ||
		strings.HasPrefix(l, "vbscript:") ||
		strings.HasPrefix(l, "data:") ||
		strings.HasPrefix(l, "file:")
}
