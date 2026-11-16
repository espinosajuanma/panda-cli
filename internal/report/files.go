package report

import (
	"fmt"
	"io"
)

type TaskFile struct {
	Name        string
	ID          string
	Description string
}

type StagedTaskFile struct {
	Name          string
	Path          string
	Description   string
	SourceMissing bool
}

type TaskFilesView struct {
	Label  string
	Title  string
	Files  []TaskFile
	Staged []StagedTaskFile
}

func RenderTaskFiles(view TaskFilesView, out io.Writer) error {
	fmt.Fprintf(out, "Files for task %s — %s\n\n", view.Label, view.Title)
	fmt.Fprintln(out, "Attached files:")
	if len(view.Files) == 0 {
		fmt.Fprintln(out, "  No attached files.")
	} else {
		for index, file := range view.Files {
			fmt.Fprintf(out, "  [%d] %s (id: %s)", index+1, file.Name, file.ID)
			if file.Description != "" {
				fmt.Fprintf(out, " — %s", file.Description)
			}
			fmt.Fprintln(out)
		}
	}
	if len(view.Staged) > 0 {
		fmt.Fprintln(out, "\nStaged uploads:")
		for _, staged := range view.Staged {
			status := "[staged]"
			if staged.SourceMissing {
				status = "[staged; source missing]"
			}
			fmt.Fprintf(out, "  %s %s (source: %s)", status, staged.Name, staged.Path)
			if staged.Description != "" {
				fmt.Fprintf(out, " — %s", staged.Description)
			}
			fmt.Fprintln(out)
		}
	} else if len(view.Files) > 0 {
		fmt.Fprintln(out)
	}
	return nil
}
