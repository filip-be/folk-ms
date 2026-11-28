package domain

import (
	"html"
	"net/url"
	"strings"
)

// RenderMarkdown renders a Tune's description, written in a small Markdown
// subset, to HTML: paragraphs, line breaks, **bold**, *italic* (or _italic_),
// [links](https://…) and mailto: links, bulleted and numbered lists, "#"
// and "##" headings (h3, h4) and "> " block quotes. Every character of the
// text is escaped, so raw HTML never passes through; the output uses only
// the renderer's own tags (p, br, strong, em, a, ul, ol, li, h3, h4,
// blockquote) without classes or styles. Links get
// rel="nofollow noopener ugc". Sites style the plain tags.
func RenderMarkdown(src string) string {
	src = strings.ReplaceAll(strings.ReplaceAll(src, "\r\n", "\n"), "\r", "\n")
	var out strings.Builder
	renderBlocks(&out, strings.Split(src, "\n"), 0)
	return out.String()
}

type mdKind int

const (
	mdBlank mdKind = iota
	mdPara
	mdH3
	mdH4
	mdUL
	mdOL
	mdQuote
)

// classify tells what a line starts and returns its content.
func classify(line string) (mdKind, string) {
	t := strings.TrimLeft(line, " \t")
	switch {
	case strings.TrimSpace(t) == "":
		return mdBlank, ""
	case strings.HasPrefix(t, "## "):
		return mdH4, strings.TrimSpace(t[3:])
	case strings.HasPrefix(t, "# "):
		return mdH3, strings.TrimSpace(t[2:])
	case t == ">" || strings.HasPrefix(t, "> "):
		return mdQuote, strings.TrimPrefix(strings.TrimPrefix(t, ">"), " ")
	case strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "+ "):
		return mdUL, strings.TrimSpace(t[2:])
	}
	if i := strings.IndexByte(t, '.'); i > 0 && i <= 3 && len(t) > i+1 && t[i+1] == ' ' && isDigits(t[:i]) {
		return mdOL, strings.TrimSpace(t[i+2:])
	}
	return mdPara, strings.TrimSpace(t)
}

// renderBlocks renders lines; depth bounds nested block quotes.
func renderBlocks(out *strings.Builder, lines []string, depth int) {
	for i := 0; i < len(lines); {
		kind, text := classify(lines[i])
		switch kind {
		case mdBlank:
			i++
		case mdH3, mdH4:
			tag := "h3"
			if kind == mdH4 {
				tag = "h4"
			}
			out.WriteString("<" + tag + ">" + renderInline(text) + "</" + tag + ">")
			i++
		case mdQuote:
			var inner []string
			for i < len(lines) {
				k, t := classify(lines[i])
				if k != mdQuote {
					break
				}
				inner = append(inner, t)
				i++
			}
			out.WriteString("<blockquote>")
			if depth < 3 {
				renderBlocks(out, inner, depth+1)
			} else {
				out.WriteString("<p>" + renderInline(strings.Join(inner, "\n")) + "</p>")
			}
			out.WriteString("</blockquote>")
		case mdUL, mdOL:
			tag := "ul"
			if kind == mdOL {
				tag = "ol"
			}
			out.WriteString("<" + tag + ">")
			for i < len(lines) {
				k, t := classify(lines[i])
				if k != kind {
					break
				}
				item := []string{t}
				i++
				// An indented line that starts nothing continues the item.
				for i < len(lines) {
					k2, t2 := classify(lines[i])
					if k2 != mdPara || !strings.HasPrefix(lines[i], " ") {
						break
					}
					item = append(item, t2)
					i++
				}
				out.WriteString("<li>" + renderInline(strings.Join(item, "\n")) + "</li>")
			}
			out.WriteString("</" + tag + ">")
		default:
			var para []string
			for i < len(lines) {
				k, t := classify(lines[i])
				if k != mdPara {
					break
				}
				para = append(para, t)
				i++
			}
			out.WriteString("<p>" + renderInline(strings.Join(para, "\n")) + "</p>")
		}
	}
}

// renderInline renders emphasis, links and line breaks; everything else is
// escaped text.
func renderInline(s string) string {
	var out strings.Builder
	inlineTo(&out, s, 0)
	return out.String()
}

func inlineTo(out *strings.Builder, s string, depth int) {
	text := func(t string) {
		out.WriteString(strings.ReplaceAll(html.EscapeString(t), "\n", "<br>"))
	}
	start := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' && i+1 < len(s) && strings.IndexByte("\\*_[]()#>-+.!`", s[i+1]) >= 0:
			text(s[start:i])
			text(s[i+1 : i+2])
			i++
			start = i + 1
		case (c == '*' || c == '_') && depth < 4:
			delim := string(c)
			tag := "em"
			if i+1 < len(s) && s[i+1] == c {
				delim, tag = delim+delim, "strong"
			}
			// "_" inside a word (snake_case) is text.
			if c == '_' && i > 0 && isWordByte(s[i-1]) {
				continue
			}
			open := i + len(delim)
			if open >= len(s) || s[open] == ' ' || s[open] == '\n' {
				continue
			}
			end := closing(s, open, delim)
			if end < 0 {
				continue
			}
			text(s[start:i])
			out.WriteString("<" + tag + ">")
			inlineTo(out, s[open:end], depth+1)
			out.WriteString("</" + tag + ">")
			i = end + len(delim) - 1
			start = i + 1
		case c == '[' && depth < 4:
			label, href, n, ok := link(s[i:])
			if !ok {
				continue
			}
			text(s[start:i])
			out.WriteString(`<a href="` + html.EscapeString(href) + `" rel="nofollow noopener ugc">`)
			inlineTo(out, label, 4) // no links or emphasis inside a link
			out.WriteString("</a>")
			i += n - 1
			start = i + 1
		}
	}
	text(s[start:])
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b >= 0x80
}

// closing finds the closing delimiter of emphasis opened before from: not
// preceded by a space, and for "_" not followed by a word character.
func closing(s string, from int, delim string) int {
	for j := from; j+len(delim) <= len(s); j++ {
		if s[j] == '\\' {
			j++
			continue
		}
		if !strings.HasPrefix(s[j:], delim) {
			continue
		}
		if len(delim) == 1 && j+1 < len(s) && s[j+1] == delim[0] {
			j++ // part of a "**" or "__"
			continue
		}
		if s[j-1] == ' ' || s[j-1] == '\n' {
			continue
		}
		if delim[0] == '_' && j+len(delim) < len(s) && isWordByte(s[j+len(delim)]) {
			continue
		}
		return j
	}
	return -1
}

// link reads "[label](href)" at the start of s; only https: and mailto:
// addresses make a link.
func link(s string) (label, href string, n int, ok bool) {
	close := strings.Index(s, "](")
	if close < 1 || strings.ContainsAny(s[1:close], "[]") {
		return "", "", 0, false
	}
	end := strings.IndexByte(s[close+2:], ')')
	if end < 0 {
		return "", "", 0, false
	}
	href = strings.TrimSpace(s[close+2 : close+2+end])
	if !safeLink(href) {
		return "", "", 0, false
	}
	return s[1:close], href, close + 3 + end, true
}

// safeLink accepts an https: URL with a host, or a mailto: address.
func safeLink(h string) bool {
	if len(h) > 2048 || strings.ContainsAny(h, " \t\n\"'<>\\`") {
		return false
	}
	if strings.HasPrefix(h, "mailto:") {
		addr := strings.TrimPrefix(h, "mailto:")
		at := strings.IndexByte(addr, '@')
		return at > 0 && at < len(addr)-1 && !strings.ContainsAny(addr, ":/?#")
	}
	u, err := url.Parse(h)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && strings.HasPrefix(h, "https://")
}
