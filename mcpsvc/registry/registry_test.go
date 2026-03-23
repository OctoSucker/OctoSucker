package registry

import (
	"testing"
)

func TestParseJSON(t *testing.T) {
	b := []byte(`[{"id":"a","command":["echo","hi"]},{"id":"b","endpoint":"http://localhost:9"}]`)
	list, err := ParseJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "a" || list[1].Endpoint == "" {
		t.Fatalf("%+v", list)
	}
	r := New()
	if err := r.LoadFile("___missing___"); err == nil {
		t.Fatal("expected error")
	}
}
