package domain

import (
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	const rel = ` rel="nofollow noopener ugc"`
	cases := map[string]string{
		"Hello\nworld\n\nSecond":                         "<p>Hello<br>world</p><p>Second</p>",
		"Windows\r\nline\rends":                          "<p>Windows<br>line<br>ends</p>",
		"**bold** and *it* and _it_ and snake_case_word": "<p><strong>bold</strong> and <em>it</em> and <em>it</em> and snake_case_word</p>",
		"__strong__":                                     "<p><strong>strong</strong></p>",
		"*a **b** c*":                                    "<p><em>a <strong>b</strong> c</em></p>",
		"**a *b* c**":                                    "<p><strong>a <em>b</em> c</strong></p>",
		"# Head\n## Sub\n### three":                      "<h3>Head</h3><h4>Sub</h4><p>### three</p>",
		"- one\n- two\n  cont\n\n1. a\n2. b":             "<ul><li>one</li><li>two<br>cont</li></ul><ol><li>a</li><li>b</li></ol>",
		"* star\n+ plus":                                 "<ul><li>star</li><li>plus</li></ul>",
		"> quote\n> more\n>\n> > nested":                 "<blockquote><p>quote<br>more</p><blockquote><p>nested</p></blockquote></blockquote>",
		"[x](https://ex.org/a?b=1&c=2)":                  `<p><a href="https://ex.org/a?b=1&amp;c=2"` + rel + `>x</a></p>`,
		"[m](mailto:a@b.org)":                            `<p><a href="mailto:a@b.org"` + rel + `>m</a></p>`,
		"[**b**](https://ex.org)":                        `<p><a href="https://ex.org"` + rel + `>**b**</a></p>`,
		"**unclosed and *also":                           "<p>**unclosed and *also</p>",
		"*open":                                          "<p>*open</p>",
		"\\*not\\* 3 * 4":                                "<p>*not* 3 * 4</p>",
		"a & b < c > d \"e\" 'f'":                        "<p>a &amp; b &lt; c &gt; d &#34;e&#34; &#39;f&#39;</p>",
		"":                                               "",
	}
	for in, want := range cases {
		if got := RenderMarkdown(in); got != want {
			t.Errorf("%q:\n got %q\nwant %q", in, got, want)
		}
	}
}

// Raw HTML and unsafe links never pass through (T: stored XSS).
func TestRenderMarkdownXSS(t *testing.T) {
	cases := map[string]string{
		"<script>alert(1)</script>":           "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>",
		"<img src=x onerror=alert(1)>":        "<p>&lt;img src=x onerror=alert(1)&gt;</p>",
		"[j](javascript:alert(1))":            "<p>[j](javascript:alert(1))</p>",
		"[j](JavaScript:alert(1))":            "<p>[j](JavaScript:alert(1))</p>",
		"[d](data:text/html,<b>x</b>)":        "<p>[d](data:text/html,&lt;b&gt;x&lt;/b&gt;)</p>",
		"[h](http://ex.org)":                  "<p>[h](http://ex.org)</p>",
		"[q](https://ex.org/\"onmouseover=x)": "<p>[q](https://ex.org/&#34;onmouseover=x)</p>",
		"[q](https://ex.org/'onmouseover=x)":  "<p>[q](https://ex.org/&#39;onmouseover=x)</p>",
		"[u](https://u:p@ex.org)":             "<p>[u](https://u:p@ex.org)</p>",
		"[s](//ex.org)":                       "<p>[s](//ex.org)</p>",
		"[m](mailto:a@b.org?subject=x)":       "<p>[m](mailto:a@b.org?subject=x)</p>",
		"**<b>x</b>**":                        "<p><strong>&lt;b&gt;x&lt;/b&gt;</strong></p>",
		"[<i>x</i>](https://ex.org)":          `<p><a href="https://ex.org" rel="nofollow noopener ugc">&lt;i&gt;x&lt;/i&gt;</a></p>`,
		"# <h1>x":                             "<h3>&lt;h1&gt;x</h3>",
	}
	for in, want := range cases {
		if got := RenderMarkdown(in); got != want {
			t.Errorf("%q:\n got %q\nwant %q", in, got, want)
		}
	}
	// Deep nesting stays bounded and well-formed.
	deep := strings.Repeat("> ", 50) + "x"
	if got := RenderMarkdown(deep); strings.Count(got, "<blockquote>") != strings.Count(got, "</blockquote>") || strings.Count(got, "<blockquote>") > 4 {
		t.Errorf("deep quote: %q", got)
	}
	emph := strings.Repeat("*a ", 20) + "b" + strings.Repeat(" a*", 20)
	if got := RenderMarkdown(emph); strings.Count(got, "<em>") != strings.Count(got, "</em>") {
		t.Errorf("deep emphasis: %q", got)
	}
}
