package common

import "testing"

func TestSetDateValid(t *testing.T) {
	var v Version
	if !v.SetDate(29, 2, 2024) {
		t.Fatal("expected valid leap day")
	}
	if v.SetDate(29, 2, 2023) {
		t.Fatal("expected invalid leap day in non-leap year")
	}
	if v.SetDate(31, 4, 2024) {
		t.Fatal("expected invalid day for April")
	}
}

func TestSetDateTwoDigitYear(t *testing.T) {
	var v Version
	v.SetDate(1, 1, 21)
	if v.Year != 1921 {
		t.Fatalf("expected year 1921, got %d", v.Year)
	}
}

func TestCompareVersion(t *testing.T) {
	a := Version{V1: 1, V2: 0, V3: 0, V4: 0}
	b := Version{V1: 1, V2: 2, V3: 0, V4: 0}
	if CompareVersion(a, b) >= 0 {
		t.Fatal("expected a < b")
	}
	if CompareVersion(a, a) != 0 {
		t.Fatal("expected equal versions to compare as 0")
	}
}

func TestCompareDate(t *testing.T) {
	a := Version{Year: 2020, Month: 1, Day: 1}
	b := Version{Year: 2021, Month: 1, Day: 1}
	if CompareDate(a, b) >= 0 {
		t.Fatal("expected a < b")
	}
}

func TestSetInvalid(t *testing.T) {
	v := NewVersion()
	v.SetDate(1, 1, 2020)
	v.SetInvalid()
	if v.Year != -1 || v.V1 != -1 {
		t.Fatal("expected year and v1 to be -1 after SetInvalid")
	}
	if v.String() != "unknown" || v.DateString() != "unknown" {
		t.Fatal("expected unknown rendering after SetInvalid")
	}
}
