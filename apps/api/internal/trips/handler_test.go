package trips

import (
	"net/http/httptest"
	"testing"
)

func TestParseListFilterTripsValid(t *testing.T) {
	req := httptest.NewRequest("GET", "/trips?limit=5&offset=10", nil)
	filter, err := parseListFilter(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filter.Limit != 5 || filter.Offset != 10 {
		t.Fatalf("unexpected filter: %+v", filter)
	}
}

func TestParseListFilterTripsInvalidLimit(t *testing.T) {
	req := httptest.NewRequest("GET", "/trips?limit=0", nil)
	if _, err := parseListFilter(req); err == nil {
		t.Fatalf("expected error for invalid limit")
	}
}

func TestParseListFilterTripsInvalidOffset(t *testing.T) {
	req := httptest.NewRequest("GET", "/trips?offset=-3", nil)
	if _, err := parseListFilter(req); err == nil {
		t.Fatalf("expected error for invalid offset")
	}
}
