package webdav

import (
	"testing"
)

func TestConditionalMatch(t *testing.T) {
	// testing match and no match
	val := ConditionalMatch("\"AAA\", \"BBB\", \"CCC\"")
	if isSet, ok, err := val.MatchETag("AAA"); err != nil {
		t.Fatal(err)
	} else if !isSet {
		t.Fatalf("Expected isSet true")
	} else if !ok {
		t.Fatalf("Expected ok true")
	}
	if isSet, ok, err := val.MatchETag("DDD"); err != nil {
		t.Fatal(err)
	} else if !isSet {
		t.Fatalf("Expected isSet true")
	} else if ok {
		t.Fatalf("Expected ok false")
	}

	// testing parse error
	val = ConditionalMatch("\"AAA\", BBB, CCC")
	if _, _, err := val.MatchETag("BBB"); err == nil {
		t.Fatalf("Expected non-nil error")
	}

	// testing WildCard
	val = ConditionalMatch("*")
	if isSet, ok, err := val.MatchETag("BBB"); err != nil {
		t.Fatal(err)
	} else if !isSet {
		t.Fatalf("Expected isSet true")
	} else if !ok {
		t.Fatalf("Expected ok true")
	}

	// testing isSet
	val = ConditionalMatch("")
	if isSet, _, _ := val.MatchETag("BBB"); isSet {
		t.Fatalf("Expected isSet false")
	}
}
