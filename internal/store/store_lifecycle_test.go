package store

import (
	"testing"
)

func TestStoreCloseClosesSQLiteConnection(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if store.db != nil {
		t.Fatal("closed store should clear sqlite connection")
	}
}
