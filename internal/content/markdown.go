package content

import (
	"bytes"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/yuin/goldmark"
)

func HTMLToMarkdown(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	converter := md.NewConverter("", true, nil)
	return converter.ConvertString(value)
}

func MarkdownToHTML(value string) (string, error) {
	var output bytes.Buffer
	if err := goldmark.Convert([]byte(value), &output); err != nil {
		return "", err
	}
	return output.String(), nil
}
