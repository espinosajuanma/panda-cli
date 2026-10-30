package solutions

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadFileSendsMultipartAndReturnsFileLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/files" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("token") != "session-token" {
			t.Fatal("missing token header")
		}
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/form-data" {
			t.Fatalf("Content-Type = %q, err=%v", r.Header.Get("Content-Type"), err)
		}
		reader := multipart.NewReader(r.Body, params["boundary"])
		part, err := reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		if part.FormName() != "file" || part.FileName() != "screen.txt" {
			t.Fatalf("unexpected multipart file: name=%q filename=%q", part.FormName(), part.FileName())
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "file contents" {
			t.Fatalf("uploaded content = %q", data)
		}
		if got := part.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
			t.Fatalf("file Content-Type = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"fileId":"file-7","fileLink":"/files/file-7"}`)
	}))
	defer server.Close()

	link, err := NewClient(server.URL, "session-token").UploadFile(context.Background(), "screen.txt", strings.NewReader("file contents"))
	if err != nil {
		t.Fatal(err)
	}
	if link.FileID != "file-7" || link.FileLink != "/files/file-7" {
		t.Fatalf("unexpected uploaded file link: %#v", link)
	}
}

func TestAttachFilesSendsNestedFileReferences(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/data/dev.tasks/task-1/attachFile" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		var body struct {
			Files []struct {
				File        map[string]string `json:"file"`
				Description string            `json:"description"`
			} `json:"files"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Files) != 1 || body.Files[0].File["id"] != "file-7" || body.Files[0].Description != "spec" {
			t.Fatalf("unexpected attach payload: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"task-1","files":[{"file":{"id":"file-7"}}]}`)
	}))
	defer server.Close()

	updated, err := NewClient(server.URL, "token").AttachFiles(context.Background(), "task-1", []FileAttachment{{FileID: "file-7", Description: "spec"}})
	if err != nil {
		t.Fatal(err)
	}
	if updated["id"] != "task-1" {
		t.Fatalf("unexpected task response: %#v", updated)
	}
}

func TestDownloadTaskFileStreamsContentAndReturnsFilename(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/data/dev.tasks/task-1/files/file-7" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("token") != "token" {
			t.Fatal("missing token header")
		}
		w.Header().Set("Content-Disposition", `attachment; filename="report.txt"`)
		_, _ = io.WriteString(w, "downloaded data")
	}))
	defer server.Close()

	var destination bytes.Buffer
	name, err := NewClient(server.URL, "token").DownloadTaskFile(context.Background(), "task-1", "file-7", &destination)
	if err != nil {
		t.Fatal(err)
	}
	if name != "report.txt" || destination.String() != "downloaded data" {
		t.Fatalf("name=%q content=%q", name, destination.String())
	}
}
