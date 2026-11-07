package api

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// shopTime is the Shop's time: Bangkok, UTC+7 all year (dashboard-and-reports R1). A fixed
// zone, so it needs no time zone database on the server.
var shopTime = time.FixedZone("Asia/Bangkok", 7*60*60)

// startOfShopDay is the instant the Shop's day starts, in Bangkok, days after date: a
// YYYY-MM-DD whose format Huma has checked. An empty date is no bound.
func startOfShopDay(date string, days int) pgtype.Timestamptz {
	if date == "" {
		return pgtype.Timestamptz{}
	}
	day, _ := time.ParseInLocation(time.DateOnly, date, shopTime)
	return pgtype.Timestamptz{Time: day.AddDate(0, 0, days), Valid: true}
}
