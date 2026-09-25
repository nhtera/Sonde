// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package datefmt

// English month, weekday and am/pm names, matching chrono's built-in
// default (POSIX/English) locale used when the "unstable-locales" feature
// is disabled.

func shortMonths() [12]string {
	return [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
}

func longMonths() [12]string {
	return [12]string{
		"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December",
	}
}

// shortWeekdays and longWeekdays are indexed by "days from Sunday"
// (Sunday=0 ... Saturday=6), matching Go's time.Weekday values directly.
func shortWeekdays() [7]string {
	return [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
}

func longWeekdays() [7]string {
	return [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
}

func amPmNames() [2]string {
	return [2]string{"AM", "PM"}
}

const decimalPoint = "."
