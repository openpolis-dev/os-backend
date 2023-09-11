package model_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/theseed-labs/os-backend/internal/model"
)

const timeStrLayout = "2006-01-02 15:04:05"

// newTimeFromGMTp8TimeStr parse the timeStr in yyyy-mm-dd HH:MM:SS format into GMT+8 time object
func newTimeFromGMTp8TimeStr(timeStr string) time.Time {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	t, err := time.ParseInLocation(timeStrLayout, timeStr, location)
	if err != nil {
		panic(err)
	}
	return t
}

func quarterNumberStr(year, quarterIdx int) string {
	return fmt.Sprintf("%d%02d", year, quarterIdx)
}

var _ = Describe("TreasuryAsset", func() {
	BeforeEach(func() {

	})

	AfterEach(func() {
	})

	Describe("Quarter calculation", func() {
		When("calculates quarter index", func() {
			It("should return correct quarter index for different months", func() {
				Expect(ExportedMonthToQuarterIndex(time.January)).To(Equal(1))
				Expect(ExportedMonthToQuarterIndex(time.February)).To(Equal(1))
				Expect(ExportedMonthToQuarterIndex(time.March)).To(Equal(2))
				Expect(ExportedMonthToQuarterIndex(time.April)).To(Equal(2))
				Expect(ExportedMonthToQuarterIndex(time.May)).To(Equal(2))
				Expect(ExportedMonthToQuarterIndex(time.June)).To(Equal(3))
				Expect(ExportedMonthToQuarterIndex(time.July)).To(Equal(3))
				Expect(ExportedMonthToQuarterIndex(time.August)).To(Equal(3))
				Expect(ExportedMonthToQuarterIndex(time.September)).To(Equal(4))
				Expect(ExportedMonthToQuarterIndex(time.October)).To(Equal(4))
				Expect(ExportedMonthToQuarterIndex(time.November)).To(Equal(4))
				Expect(ExportedMonthToQuarterIndex(time.December)).To(Equal(1))
			})
		})

		When("calculates quarter num", func() {
			It("should return correct quarter num for first day of each months", func() {
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-01-01 00:00:00"))).To(Equal(quarterNumberStr(2023, 1)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-02-01 00:00:00"))).To(Equal(quarterNumberStr(2023, 1)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-03-01 00:00:00"))).To(Equal(quarterNumberStr(2023, 2)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-04-01 00:00:00"))).To(Equal(quarterNumberStr(2023, 2)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-05-01 00:00:00"))).To(Equal(quarterNumberStr(2023, 2)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-06-01 00:00:00"))).To(Equal(quarterNumberStr(2023, 3)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-07-01 00:00:00"))).To(Equal(quarterNumberStr(2023, 3)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-08-01 00:00:00"))).To(Equal(quarterNumberStr(2023, 3)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-09-01 00:00:00"))).To(Equal(quarterNumberStr(2023, 4)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-10-01 00:00:00"))).To(Equal(quarterNumberStr(2023, 4)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-11-01 00:00:00"))).To(Equal(quarterNumberStr(2023, 4)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-12-01 00:00:00"))).To(Equal(quarterNumberStr(2024, 1)))
			})

			It("should return correct quarter num for last day in of each months", func() {
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-01-31 23:59:59"))).To(Equal(quarterNumberStr(2023, 1)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-02-28 23:59:59"))).To(Equal(quarterNumberStr(2023, 1)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-03-31 23:59:59"))).To(Equal(quarterNumberStr(2023, 2)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-04-30 23:59:59"))).To(Equal(quarterNumberStr(2023, 2)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-05-31 23:59:59"))).To(Equal(quarterNumberStr(2023, 2)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-06-30 23:59:59"))).To(Equal(quarterNumberStr(2023, 3)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-07-31 23:59:59"))).To(Equal(quarterNumberStr(2023, 3)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-08-31 23:59:59"))).To(Equal(quarterNumberStr(2023, 3)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-09-30 23:59:59"))).To(Equal(quarterNumberStr(2023, 4)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-10-31 23:59:59"))).To(Equal(quarterNumberStr(2023, 4)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-11-30 23:59:59"))).To(Equal(quarterNumberStr(2023, 4)))
				Expect(ExportedGetQuarterNumForTime(newTimeFromGMTp8TimeStr("2023-12-31 23:59:59"))).To(Equal(quarterNumberStr(2024, 1)))
			})
		})

		When("calculates quarter time range", func() {
		})
	})
})
