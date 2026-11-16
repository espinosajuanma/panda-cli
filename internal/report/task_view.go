package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/espinosajuanma/panda-cli/internal/ui"
	"github.com/espinosajuanma/panda-cli/internal/workspace"
)

type TaskViewRow struct {
	Label string
	Value string
}

type TaskViewReleaseNote struct {
	Heading  string
	Markdown string
}

type TaskViewFile struct {
	Name        string
	ID          string
	Description string
	Staged      bool
}

type TaskViewNote struct {
	Filename string
	Markdown string
	Staged   bool
}

type TaskViewText struct {
	Title            string
	Rows             []TaskViewRow
	Link             string
	Description      string
	HasDescription   bool
	ShowDescription  bool
	ReleaseNotes     []TaskViewReleaseNote
	ShowReleaseNotes bool
	Files            []TaskViewFile
	Notes            []TaskViewNote
	ShowNotes        bool
}

func RenderTaskViewText(view TaskViewText, out, colorTarget io.Writer, width int) error {
	fmt.Fprintf(out, "%s\n\n", ui.ColorText(colorTarget, "1;32", "# "+view.Title))
	fmt.Fprintln(out, ui.ColorText(colorTarget, "1;32", "## Metadata"))
	WriteTaskViewTable(out, colorTarget, view.Rows, width)
	if view.Link != "" {
		fmt.Fprintf(out, "%s %s\n", ui.ColorText(colorTarget, "1;36", "Task link:"), view.Link)
	}
	if view.ShowDescription {
		fmt.Fprintln(out, ui.ColorText(colorTarget, "1;32", "\n## Description"))
		if !view.HasDescription || strings.TrimSpace(view.Description) == "" {
			fmt.Fprintln(out, "No description.")
		} else {
			fmt.Fprintln(out, ui.HighlightMarkdown(out, view.Description))
		}
	}
	if view.ShowReleaseNotes {
		for _, note := range view.ReleaseNotes {
			if strings.TrimSpace(note.Markdown) == "" {
				continue
			}
			fmt.Fprintf(out, "\n%s\n", ui.ColorText(colorTarget, "1;32", "## "+note.Heading))
			fmt.Fprintln(out, ui.HighlightMarkdown(out, note.Markdown))
		}
	}
	fmt.Fprintln(out, ui.ColorText(colorTarget, "1;32", "\n## Files"))
	if len(view.Files) == 0 {
		fmt.Fprintln(out, "No attached files.")
	} else {
		for _, file := range view.Files {
			fmt.Fprintf(out, "- %s", file.Name)
			if !file.Staged {
				fmt.Fprintf(out, " (id: %s)", file.ID)
			} else {
				fmt.Fprint(out, " (staged upload)")
			}
			if file.Description != "" {
				fmt.Fprintf(out, " — %s", file.Description)
			}
			fmt.Fprintln(out)
		}
	}
	if view.ShowNotes {
		fmt.Fprintln(out)
		fmt.Fprintln(out, ui.ColorText(colorTarget, "1;32", "## Notes"))
		if len(view.Notes) == 0 {
			fmt.Fprintln(out, "No notes.")
		} else {
			for _, note := range view.Notes {
				marker := ""
				if note.Staged {
					marker = ui.ColorText(colorTarget, "1;33", " [staged]")
				}
				fmt.Fprintf(out, "%s%s\n", ui.ColorText(colorTarget, "1;36", "### "+strings.TrimSuffix(note.Filename, ".md")), marker)
				fmt.Fprintln(out, ui.HighlightMarkdown(out, note.Markdown))
			}
		}
	}
	return nil
}

func WriteTaskViewTable(out, colorTarget io.Writer, rows []TaskViewRow, width int) {
	labelWidth := 0
	for _, row := range rows {
		if length := utf8.RuneCountInString(row.Label); length > labelWidth {
			labelWidth = length
		}
	}
	if labelWidth == 0 {
		return
	}
	valueWidth := width - labelWidth - 6
	if valueWidth < 12 {
		valueWidth = 12
	}
	indent := strings.Repeat(" ", labelWidth+4)
	for _, row := range rows {
		padding := labelWidth - utf8.RuneCountInString(row.Label)
		wrapped := WrapValue(row.Value, valueWidth)
		if len(wrapped) == 0 {
			wrapped = []string{"None"}
		}
		fmt.Fprintf(out, "  %s%s  %s\n", ui.ColorText(colorTarget, "1;36", row.Label), strings.Repeat(" ", padding), wrapped[0])
		for _, line := range wrapped[1:] {
			fmt.Fprintf(out, "%s%s\n", indent, line)
		}
	}
}

type TaskViewJSON struct {
	Task         map[string]any            `json:"task"`
	Description  *string                   `json:"description,omitempty"`
	ReleaseNotes *TaskViewJSONReleaseNotes `json:"releaseNotes,omitempty"`
	Notes        *[]TaskViewJSONNote       `json:"notes,omitempty"`
	Context      TaskViewJSONContext       `json:"context"`
}

type TaskViewJSONReleaseNotes struct {
	CustomerNotes string `json:"customerNotes"`
	InternalNotes string `json:"internalNotes"`
}

type TaskViewJSONNote struct {
	Filename string `json:"filename"`
	Markdown string `json:"markdown"`
	Staged   bool   `json:"staged"`
}

type TaskViewJSONContext struct {
	Link           string                 `json:"link,omitempty"`
	SyncStatus     string                 `json:"syncStatus"`
	PendingChanges bool                   `json:"pendingChanges"`
	LocalDraft     bool                   `json:"localDraft"`
	StagedFiles    []workspace.StagedFile `json:"stagedFiles,omitempty"`
}

func WriteTaskViewJSON(document TaskViewJSON, out io.Writer) error {
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", data)
	return err
}
