package controller

import "testing"

func TestParseTokenIds(t *testing.T) {
	ids, err := parseTokenIds("2, 5,2,7")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || ids[0] != 2 || ids[1] != 5 || ids[2] != 7 {
		t.Fatalf("unexpected ids: %#v", ids)
	}
}

func TestParseTokenIdsRejectsInvalidValue(t *testing.T) {
	if _, err := parseTokenIds("2,invalid"); err == nil {
		t.Fatal("expected invalid token id error")
	}
}
