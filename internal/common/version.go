// Package common holds small value types shared across the SDI engine.
package common

import "fmt"

// Version holds a driver's release date and four-part version number.
// V1 < 0 means the version number is unset; Year < 1000 means the date is unset.
type Version struct {
	Day, Month, Year int
	V1, V2, V3, V4   int
}

// NewVersion returns a Version with an unset version number.
func NewVersion() Version {
	return Version{V1: -2}
}

// SetDate stores a date and reports whether it is a valid calendar date.
// A two-digit year is treated as 19xx.
func (v *Version) SetDate(day, month, year int) bool {
	if year < 100 {
		year += 1900
	}
	v.Day, v.Month, v.Year = day, month, year
	return isValidCalendarDate(day, month, year)
}

func isValidCalendarDate(day, month, year int) bool {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return day >= 1 && day <= 31
	case 4, 6, 9, 11:
		return day >= 1 && day <= 30
	case 2:
		leap := (year%4 == 0 && year%100 != 0) || year%400 == 0
		if leap {
			return day >= 1 && day <= 29
		}
		return day >= 1 && day <= 28
	default:
		return false
	}
}

// SetVersion stores the four-part version number.
func (v *Version) SetVersion(v1, v2, v3, v4 int) {
	v.V1, v.V2, v.V3, v.V4 = v1, v2, v3, v4
}

// SetInvalid marks both the date and version number as unset.
func (v *Version) SetInvalid() {
	v.Year = -1
	v.V1 = -1
}

// CompareDate compares two versions by date (year, month, day). It returns
// a negative number if a < b, zero if equal, and positive if a > b.
func CompareDate(a, b Version) int {
	if d := a.Year - b.Year; d != 0 {
		return d
	}
	if d := a.Month - b.Month; d != 0 {
		return d
	}
	return a.Day - b.Day
}

// CompareVersion compares two versions by their four-part version number.
// It returns a negative number if a < b, zero if equal, and positive if a > b.
func CompareVersion(a, b Version) int {
	if d := a.V1 - b.V1; d != 0 {
		return d
	}
	if d := a.V2 - b.V2; d != 0 {
		return d
	}
	if d := a.V3 - b.V3; d != 0 {
		return d
	}
	return a.V4 - b.V4
}

// String renders the version number, or "unknown" if unset.
func (v Version) String() string {
	if v.V1 < 0 {
		return "unknown"
	}
	return fmt.Sprintf("%d.%d.%d.%d", v.V1, v.V2, v.V3, v.V4)
}

// DateString renders the date as YYYY-MM-DD, or "unknown" if unset.
func (v Version) DateString() string {
	if v.Year < 1000 {
		return "unknown"
	}
	return fmt.Sprintf("%04d-%02d-%02d", v.Year, v.Month, v.Day)
}
