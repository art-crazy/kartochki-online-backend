package billing

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func parseUserID(userID string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(userID))
}

func currentMonthPeriod(now time.Time) (time.Time, time.Time) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

func toTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func toPgText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func toPgUUID(value uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: value, Valid: value != uuid.Nil}
}

func nullableTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func int32Value(value pgtype.Int4) int {
	if !value.Valid {
		return 0
	}
	return int(value.Int32)
}
