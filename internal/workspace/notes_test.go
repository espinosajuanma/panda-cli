package workspace

import "testing"

func TestNoteAndTextFileRepositories(t *testing.T) {
	dir := t.TempDir()
	if err := WriteNoteFile(dir, "20260101T000000.000Z-Ada.md", "body"); err != nil {
		t.Fatal(err)
	}
	if err := WriteNoteFile(dir, "20260102T000000.000Z-Ada.md", "second"); err != nil {
		t.Fatal(err)
	}
	files, err := TaskNoteFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Markdown != "body" || files[1].Markdown != "second" {
		t.Fatalf("note files = %#v", files)
	}
	body, exists, err := ReadNoteFile(dir, "20260101T000000.000Z-Ada.md")
	if err != nil || !exists || body != "body" {
		t.Fatalf("read note = %q, %v, %v", body, exists, err)
	}
	if err := RemoveNoteFile(dir, "20260101T000000.000Z-Ada.md"); err != nil {
		t.Fatal(err)
	}
	if _, exists, err := ReadNoteFile(dir, "20260101T000000.000Z-Ada.md"); err != nil || exists {
		t.Fatalf("removed note exists=%v err=%v", exists, err)
	}

	if err := EnsureTextFile(dir, "customer-notes.md", "initial"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureTextFile(dir, "customer-notes.md", "ignored"); err != nil {
		t.Fatal(err)
	}
	content, exists, err := ReadTextFile(dir, "customer-notes.md")
	if err != nil || !exists || content != "initial" {
		t.Fatalf("text file = %q, %v, %v", content, exists, err)
	}
	if err := WriteTextFile(dir, "customer-notes.md", "updated"); err != nil {
		t.Fatal(err)
	}
	content, _, _ = ReadTextFile(dir, "customer-notes.md")
	if content != "updated" {
		t.Fatalf("updated text file = %q", content)
	}
	if _, exists, err := ReadTextFile(dir, "missing.md"); err != nil || exists {
		t.Fatalf("missing text file exists=%v err=%v", exists, err)
	}
}
