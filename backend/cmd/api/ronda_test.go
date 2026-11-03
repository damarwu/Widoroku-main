package main

import "testing"

func TestParseRondaDateMapsMondayThroughSunday(t *testing.T) {
	dates := []string{"2024-01-01", "2024-01-02", "2024-01-03", "2024-01-04", "2024-01-05", "2024-01-06", "2024-01-07"}
	for index, date := range dates {
		_, got, err := parseRondaDate(date)
		if err != nil {
			t.Fatalf("parseRondaDate(%q) returned error: %v", date, err)
		}
		if want := index + 1; got != want {
			t.Errorf("parseRondaDate(%q) weekday = %d, want %d", date, got, want)
		}
	}
}

func TestParseRondaDateRejectsInvalidDate(t *testing.T) {
	if _, _, err := parseRondaDate("not-a-date"); err == nil {
		t.Fatal("parseRondaDate accepted an invalid date")
	}
}
