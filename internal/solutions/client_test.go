package solutions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginSendsCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/login" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var credentials map[string]string
		if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
			t.Fatal(err)
		}
		if credentials["email"] != "dev@example.com" || credentials["password"] != "secret" {
			t.Fatalf("unexpected credentials: %#v", credentials)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"session-token"}`))
	}))
	defer server.Close()

	token, err := NewClient(server.URL, "").Login(context.Background(), "dev@example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if token != "session-token" {
		t.Fatalf("got token %q", token)
	}
}

func TestListTasksFollowsOffsetAndSendsProjectFilter(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/data/dev.tasks" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("token") != "session-token" {
			t.Fatalf("missing token header")
		}
		if r.URL.Query().Get("project.id") != "project-7" {
			t.Fatalf("missing project filter: %v", r.URL.Query())
		}
		if got := r.URL.Query().Get("_size"); got != "1000" {
			t.Fatalf("page size = %q, want 1000", got)
		}
		if !strings.Contains(r.URL.Query().Get("_fields"), "rank") {
			t.Fatalf("rank field was not explicitly requested: %v", r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			if got := r.URL.Query().Get("_offset"); got != "" {
				t.Fatalf("first page unexpectedly has offset %q", got)
			}
			_, _ = w.Write([]byte(`{"total":2,"offset":"page-two","items":[{"id":"task-1"}]}`))
			return
		}
		if got := r.URL.Query().Get("_offset"); got != "page-two" {
			t.Fatalf("got offset %q, want page-two", got)
		}
		_, _ = w.Write([]byte(`{"total":2,"items":[{"id":"task-2"}]}`))
	}))
	defer server.Close()

	tasks, err := NewClient(server.URL, "session-token").ListTasks(context.Background(), "project-7")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 || tasks[0]["id"] != "task-1" || tasks[1]["id"] != "task-2" {
		t.Fatalf("unexpected tasks: %#v", tasks)
	}
	if requests != 2 {
		t.Fatalf("made %d requests, want 2", requests)
	}
}

func TestListLabelsAndPackagesRequestMaximumPageSize(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
	}{
		{name: "labels", path: "/data/dev.labels"},
		{name: "packages", path: "/data/dev.packages"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != test.path {
					t.Fatalf("request path = %q, want %q", r.URL.Path, test.path)
				}
				if r.URL.Query().Get("project.id") != "project-7" {
					t.Fatalf("project filter missing: %v", r.URL.Query())
				}
				if got := r.URL.Query().Get("_size"); got != "1000" {
					t.Fatalf("page size = %q, want 1000", got)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"total":0,"items":[]}`))
			}))
			defer server.Close()

			client := NewClient(server.URL, "token")
			var list func(context.Context, string) ([]map[string]any, error)
			if test.name == "labels" {
				list = client.ListLabels
			} else {
				list = client.ListPackages
			}
			if _, err := list(context.Background(), "project-7"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestListProjectMembersFetchesEmbeddedMembersAndRoles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/data/projects/project-7" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("_fields") != "id,members" {
			t.Fatalf("project members were not requested: %v", r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"project-7","members":[{"user":{"id":"user-1","label":"Developer"},"role":"developer"}]}`))
	}))
	defer server.Close()

	members, err := NewClient(server.URL, "session-token").ListProjectMembers(context.Background(), "project-7")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0]["role"] != "developer" {
		t.Fatalf("unexpected members: %#v", members)
	}
	user, _ := members[0]["user"].(map[string]any)
	if user["id"] != "user-1" || user["label"] != "Developer" {
		t.Fatalf("unexpected member user: %#v", user)
	}
}

func TestListStopsWhenTotalIsSatisfiedEvenIfOffsetIsPresent(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":1,"offset":"last-record-id","items":[{"id":"project-1","name":"Stokes Wagner"}]}`))
	}))
	defer server.Close()

	project, err := NewClient(server.URL, "token").FindProject(context.Background(), "Stokes Wagner")
	if err != nil {
		t.Fatal(err)
	}
	if project.ID != "project-1" {
		t.Fatalf("got project ID %q", project.ID)
	}
	if requests != 1 {
		t.Fatalf("made %d requests, want exactly one", requests)
	}
}

func TestInvalidTokenResponsePromptsUserToLoginAgain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"code":"unauthorize","message":"Invalid token. Try to log in again."}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "expired-token").ListTasks(context.Background(), "project-7")
	if err == nil {
		t.Fatal("ListTasks unexpectedly succeeded")
	}
	if got, want := err.Error(), "your Solutions login token is invalid or expired; run `panda login` to sign in again"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestLoginErrorsAreNotReportedAsExpiredTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"code":"unauthorize","message":"Invalid token. Try to log in again."}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "existing-token").Login(context.Background(), "dev@example.com", "wrong")
	if err == nil {
		t.Fatal("Login unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), "panda login") {
		t.Fatalf("login error was incorrectly reported as an expired token: %v", err)
	}
}
