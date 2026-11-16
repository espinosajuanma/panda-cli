package workspace

import (
	"reflect"
	"testing"

	"github.com/espinosajuanma/panda-cli/internal/catalog"
)

func TestCatalogRepositoryRoundTripsRecordsAndBaseline(t *testing.T) {
	ws := New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	records := []map[string]any{{"id": "label-1", "name": "Bug"}}
	if err := ws.WriteCatalog(catalog.Labels, records); err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteCatalogBaseline(catalog.Labels, records); err != nil {
		t.Fatal(err)
	}
	read, err := ws.ReadCatalog(catalog.Labels)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(read, records) {
		t.Fatalf("catalog = %#v, want %#v", read, records)
	}
	baseline, err := ws.ReadCatalogBaseline(catalog.Labels)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(baseline, records) {
		t.Fatalf("baseline = %#v, want %#v", baseline, records)
	}
}

func TestReadCatalogReturnsNilWhenMissing(t *testing.T) {
	ws := New(t.TempDir())
	records, err := ws.ReadCatalog(catalog.Labels)
	if err != nil || records != nil {
		t.Fatalf("missing catalog = %#v, %v", records, err)
	}
}

func TestMembersRepositoryRoundTripsAndReportsAvailability(t *testing.T) {
	ws := New(t.TempDir())
	if err := ws.Ensure(); err != nil {
		t.Fatal(err)
	}
	if _, available, err := ws.ReadMembers(); err != nil || available {
		t.Fatalf("missing members available=%v err=%v", available, err)
	}
	members := []map[string]any{{"user": map[string]any{"id": "user-1"}}}
	if err := ws.WriteMembers(members); err != nil {
		t.Fatal(err)
	}
	read, available, err := ws.ReadMembers()
	if err != nil || !available {
		t.Fatalf("members available=%v err=%v", available, err)
	}
	if !reflect.DeepEqual(read, members) {
		t.Fatalf("members = %#v, want %#v", read, members)
	}
}
