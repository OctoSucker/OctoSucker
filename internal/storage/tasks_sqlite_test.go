package storage

import (
	"bytes"
	"testing"
)

func TestTaskSnapshotRoundTrip(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	want := []byte(`{"id":"task-1","status":"running"}`)
	if err := db.SaveTaskSnapshot("task-1", want); err != nil {
		t.Fatal(err)
	}
	loaded, err := db.LoadTaskSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || !bytes.Equal(loaded[0], want) {
		t.Fatalf("loaded = %q", loaded)
	}

	updated := []byte(`{"id":"task-1","status":"completed"}`)
	if err := db.SaveTaskSnapshot("task-1", updated); err != nil {
		t.Fatal(err)
	}
	loaded, err = db.LoadTaskSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || !bytes.Equal(loaded[0], updated) {
		t.Fatalf("updated load = %q", loaded)
	}
}
