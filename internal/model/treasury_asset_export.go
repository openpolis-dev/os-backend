package model

import "time"

func ExportedMonthToQuarterIndex(m time.Month) int {
	return monthToQuarterIndex(m)
}

func ExportedGetQuarterNumForTime(t time.Time) string {
	return getQuarterNumForTime(t)
}

func ExportedGetQuarterTimeRangeForTime(t time.Time) (string, string) {
	return getQuarterTimeRangeForTime(t)
}
