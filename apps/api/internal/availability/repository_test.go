package availability

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeSearchTextFoldsAccents(t *testing.T) {
	if got := normalizeSearchText("Concórdia/SC"); got != "concordia/sc" {
		t.Fatalf("expected folded concordia/sc, got %q", got)
	}
	if got := normalizeSearchText("Santa Ines/MA"); got != "santa ines/ma" {
		t.Fatalf("expected santa ines/ma, got %q", got)
	}
	if got := normalizeSearchText("  Chapecó  /SC "); got != "chapeco/sc" {
		t.Fatalf("expected folded chapeco/sc, got %q", got)
	}
	if got := normalizeSearchText("Pacote p/ Maranhão"); got != "pacote p/maranhao" {
		t.Fatalf("expected folded package name, got %q", got)
	}
}

func TestNormalizedSearchColumnSQLUsesAccentInsensitiveTranslation(t *testing.T) {
	got := normalizedSearchColumnSQL("destination_stop.display_name")
	want := "replace(replace(translate(lower(coalesce(destination_stop.display_name, '')), 'áàâãäéèêëíìîïóòôõöúùûüçñ', 'aaaaaeeeeiiiiooooouuuucn'), ' /', '/'), '/ ', '/')"
	if got != want {
		t.Fatalf("unexpected sql expression: %q", got)
	}
}

func TestBuildAvailabilitySearchQueryNormalizesPackageNameAccentInsensitive(t *testing.T) {
	query, args := buildAvailabilitySearchQuery(SearchFilter{
		Origin:      "Chapecó/SC",
		Destination: "Santa Inês/MA",
		PackageName: "Pacote p/ Maranhao",
		Qty:         1,
		OnlyActive:  true,
	})

	assertQueryContains(t, query, normalizedSearchColumnSQL("origin_stop.display_name")+" = $1")
	assertQueryContains(t, query, normalizedSearchColumnSQL("destination_stop.display_name")+" = $2")
	assertQueryContains(t, query, normalizedSearchColumnSQL("t.package_name")+" = $3")
	assertQueryContains(t, query, "t.trip_date >= current_date")
	assertQueryContains(t, query, "upper(coalesce(rsp.status, 'ACTIVE')) = 'ACTIVE'")
	assertQueryContains(t, query, "greatest(coalesce(t.seats_available, 0), 0) >= $4")

	wantArgs := []interface{}{
		"chapeco/sc",
		"santa ines/ma",
		"pacote p/maranhao",
		1,
		10,
	}
	assertArgsEqual(t, args, wantArgs)
}

func TestBuildAvailabilitySearchQueryExactPackageMatchStillUsesNormalizedValue(t *testing.T) {
	query, args := buildAvailabilitySearchQuery(SearchFilter{
		PackageName: "Pacote p/ Maranhão",
		OnlyActive:  true,
	})

	assertQueryContains(t, query, normalizedSearchColumnSQL("t.package_name")+" = $1")
	assertQueryNotContains(t, query, "lower(coalesce(t.package_name, '')) = lower(")

	wantArgs := []interface{}{
		"pacote p/maranhao",
		10,
	}
	assertArgsEqual(t, args, wantArgs)
}

func TestBuildAvailabilitySearchQueryKeepsOriginDestinationNormalization(t *testing.T) {
	query, args := buildAvailabilitySearchQuery(SearchFilter{
		Origin:      "  Chapecó  /SC ",
		Destination: "Santa Inês/MA",
	})

	assertQueryContains(t, query, normalizedSearchColumnSQL("origin_stop.display_name")+" = $1")
	assertQueryContains(t, query, normalizedSearchColumnSQL("destination_stop.display_name")+" = $2")

	wantArgs := []interface{}{
		"chapeco/sc",
		"santa ines/ma",
		10,
	}
	assertArgsEqual(t, args, wantArgs)
}

func assertQueryContains(t *testing.T, query, want string) {
	t.Helper()
	if !strings.Contains(query, want) {
		t.Fatalf("expected query to contain %q, got %q", want, query)
	}
}

func TestBuildAvailabilitySearchQueryTripDateKeepsExactFilter(t *testing.T) {
	tripDate := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	dateFrom := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	dateTo := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)

	query, args := buildAvailabilitySearchQuery(SearchFilter{
		Origin:     "Moncao/MA",
		TripDate:   &tripDate,
		DateFrom:   &dateFrom,
		DateTo:     &dateTo,
		OnlyActive: true,
	})

	assertQueryContains(t, query, "t.trip_date = $2::date")
	assertQueryNotContains(t, query, "t.trip_date >= $")
	assertQueryNotContains(t, query, "t.trip_date <= $")
	assertArgsEqual(t, args, []interface{}{
		"moncao/ma",
		"2026-07-10",
		10,
	})
}

func TestBuildAvailabilitySearchQueryDateRangeFiltersWhenTripDateMissing(t *testing.T) {
	dateFrom := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	dateTo := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)

	query, args := buildAvailabilitySearchQuery(SearchFilter{
		Origin:      "Moncao/MA",
		Destination: "Videira/SC",
		DateFrom:    &dateFrom,
		DateTo:      &dateTo,
		Qty:         1,
		OnlyActive:  true,
	})

	assertQueryContains(t, query, "t.trip_date >= $3::date")
	assertQueryContains(t, query, "t.trip_date <= $4::date")
	assertQueryNotContains(t, query, "t.trip_date = $")
	assertArgsEqual(t, args, []interface{}{
		"moncao/ma",
		"videira/sc",
		"2026-07-01",
		"2026-07-31",
		1,
		10,
	})
}

func assertQueryNotContains(t *testing.T, query, unwanted string) {
	t.Helper()
	if strings.Contains(query, unwanted) {
		t.Fatalf("expected query not to contain %q, got %q", unwanted, query)
	}
}

func assertArgsEqual(t *testing.T, got, want []interface{}) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("unexpected args length: got %+v want %+v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("unexpected arg %d: got %#v want %#v; all args got %+v want %+v", i, got[i], want[i], got, want)
		}
	}
}
