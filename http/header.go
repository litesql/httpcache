package http

import (
	"log/slog"
	"net/http"
	"time"
)

// Age based on RFC 9111
func Age(header http.Header, requestTime time.Time, responseTime time.Time) *int {
	headerDate := header.Get("Date")
	if headerDate == "" {
		return nil
	}

	date, err := time.Parse(time.RFC1123, headerDate)
	if err != nil {
		slog.Debug("calculating Age: Date header is invalid", "error", err)
		return nil
	}

	age := calculateAge(date, requestTime, responseTime)
	return &age
}

func calculateAge(date, requestTime, responseTime time.Time) int {
	apparentAge := max(0, int(responseTime.Sub(date).Seconds()))

	responseDelay := int(responseTime.Sub(requestTime).Seconds())
	correctedAgeValue := apparentAge + responseDelay

	correctedInitialAge := max(apparentAge, correctedAgeValue)

	residentTime := int(time.Since(responseTime).Seconds())

	currentAge := correctedInitialAge + residentTime
	return currentAge
}
