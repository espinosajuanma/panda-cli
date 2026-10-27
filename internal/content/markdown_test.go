package content

import (
	"strings"
	"testing"
)

func TestMarkdownHTMLRoundTrip(t *testing.T) {
	markdown := "A **bold** line.\n"
	html, err := MarkdownToHTML(markdown)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "<strong>bold</strong>") {
		t.Fatalf("markdown was not rendered as HTML: %s", html)
	}
	converted, err := HTMLToMarkdown(html)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(converted, "**bold**") {
		t.Fatalf("HTML was not converted back to markdown: %s", converted)
	}
}
