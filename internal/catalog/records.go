package catalog

import (
	"fmt"

	"github.com/espinosajuanma/panda-cli/internal/content"
	"github.com/espinosajuanma/panda-cli/internal/jsonutil"
)

func RecordID(record map[string]any) string {
	if id := jsonutil.StringField(record, "id"); id != "" {
		return id
	}
	return jsonutil.StringField(record, "localId")
}

func RecordName(kind Kind, record map[string]any) string {
	return jsonutil.StringField(record, NameField(kind))
}

func CloneRecords(records []map[string]any) []map[string]any {
	cloned := make([]map[string]any, 0, len(records))
	for _, record := range records {
		cloned = append(cloned, jsonutil.CloneMap(record))
	}
	return cloned
}

func NormalizeRecords(kind Kind, records []map[string]any) ([]map[string]any, error) {
	normalized := make([]map[string]any, 0, len(records))
	for _, record := range records {
		item := jsonutil.CloneMap(record)
		if kind == Packages {
			markdown, err := content.HTMLToMarkdown(jsonutil.StringField(record, "description"))
			if err != nil {
				return nil, fmt.Errorf("convert package %q description to Markdown: %w", RecordName(kind, record), err)
			}
			item["description"] = markdown
		}
		normalized = append(normalized, item)
	}
	return normalized, nil
}

func RecordForAPI(kind Kind, record map[string]any) (map[string]any, error) {
	item := jsonutil.CloneMap(record)
	if kind == Packages {
		html, err := content.MarkdownToHTML(jsonutil.StringField(record, "description"))
		if err != nil {
			return nil, fmt.Errorf("convert package %q description to HTML: %w", RecordName(kind, record), err)
		}
		item["description"] = html
	}
	return item, nil
}

func SetField(destination map[string]any, field string, source map[string]any) {
	if value, exists := source[field]; exists {
		destination[field] = value
	} else {
		delete(destination, field)
	}
}
