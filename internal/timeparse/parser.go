package timeparse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ParsedEntry struct {
	ClientName  string
	Hours       float64
	Dates       []time.Time
	Description string
}

func ParseNaturalLanguage(input string) (*ParsedEntry, error) {
	input = strings.ToLower(input)
	entry := &ParsedEntry{}

	hoursPattern := regexp.MustCompile(`(\d+(?:\.\d+)?)\s*hours?`)
	if matches := hoursPattern.FindStringSubmatch(input); len(matches) > 1 {
		hours, err := strconv.ParseFloat(matches[1], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid hours format: %w", err)
		}
		entry.Hours = hours
	} else {
		return nil, fmt.Errorf("no hours specified in input")
	}

	clientPattern := regexp.MustCompile(`for\s+(\w+(?:\s+\w+)?)`)
	if matches := clientPattern.FindStringSubmatch(input); len(matches) > 1 {
		entry.ClientName = strings.TrimSpace(matches[1])
	} else {
		words := strings.Fields(input)
		for i, word := range words {
			if i > 0 && !strings.Contains(word, "hour") && !isTimeKeyword(word) {
				entry.ClientName = word
				break
			}
		}
	}

	if entry.ClientName == "" {
		return nil, fmt.Errorf("no client name found in input")
	}

	entry.ClientName = strings.Title(entry.ClientName)

	if strings.Contains(input, "this week") {
		entry.Dates = getThisWeekDates()
	} else if strings.Contains(input, "last week") {
		entry.Dates = getLastWeekDates()
	} else if strings.Contains(input, "today") {
		entry.Dates = []time.Time{time.Now()}
	} else if strings.Contains(input, "yesterday") {
		entry.Dates = []time.Time{time.Now().AddDate(0, 0, -1)}
	} else {
		entry.Dates = []time.Time{time.Now()}
	}

	descPattern := regexp.MustCompile(`"([^"]+)"`)
	if matches := descPattern.FindStringSubmatch(input); len(matches) > 1 {
		entry.Description = matches[1]
	}

	return entry, nil
}

func ParseDate(dateStr string) (time.Time, error) {
	dateStr = strings.ToLower(strings.TrimSpace(dateStr))

	if dateStr == "today" {
		return time.Now(), nil
	}
	if dateStr == "yesterday" {
		return time.Now().AddDate(0, 0, -1), nil
	}
	if dateStr == "tomorrow" {
		return time.Now().AddDate(0, 0, 1), nil
	}

	if strings.HasPrefix(dateStr, "this ") {
		return parseRelativeDate(dateStr, 0)
	}
	if strings.HasPrefix(dateStr, "last ") {
		return parseRelativeDate(dateStr, -1)
	}
	if strings.HasPrefix(dateStr, "next ") {
		return parseRelativeDate(dateStr, 1)
	}

	formats := []string{
		"2006-01-02",
		"01/02/2006",
		"02/01/2006",
		"January 2, 2006",
		"Jan 2, 2006",
		"2 January 2006",
		"2 Jan 2006",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
}

func parseRelativeDate(dateStr string, weekOffset int) (time.Time, error) {
	now := time.Now()

	if strings.Contains(dateStr, "week") {
		startOfWeek := now.AddDate(0, 0, -int(now.Weekday())+weekOffset*7)
		return startOfWeek, nil
	}

	if strings.Contains(dateStr, "month") {
		if weekOffset == 0 {
			return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()), nil
		}
		return now.AddDate(0, weekOffset, 0), nil
	}

	return time.Time{}, fmt.Errorf("unable to parse relative date: %s", dateStr)
}

func ParsePeriod(period string) (time.Time, time.Time, error) {
	period = strings.ToLower(strings.TrimSpace(period))
	now := time.Now()

	if period == "this month" || period == "current month" {
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 1, -1)
		return start, end, nil
	}

	if period == "last month" {
		start := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 1, -1)
		return start, end, nil
	}

	if period == "this week" {
		start := now.AddDate(0, 0, -int(now.Weekday()))
		end := start.AddDate(0, 0, 6)
		return start, end, nil
	}

	if period == "last week" {
		start := now.AddDate(0, 0, -int(now.Weekday())-7)
		end := start.AddDate(0, 0, 6)
		return start, end, nil
	}

	monthYear := regexp.MustCompile(`(\w+)\s+(\d{4})`)
	if matches := monthYear.FindStringSubmatch(period); len(matches) == 3 {
		monthName := matches[1]
		yearStr := matches[2]

		month, err := parseMonth(monthName)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}

		year, err := strconv.Atoi(yearStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid year: %s", yearStr)
		}

		start := time.Date(year, month, 1, 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 1, -1)
		return start, end, nil
	}

	return time.Time{}, time.Time{}, fmt.Errorf("unable to parse period: %s", period)
}

func parseMonth(monthStr string) (time.Month, error) {
	monthStr = strings.ToLower(monthStr)
	months := map[string]time.Month{
		"january":   time.January,
		"february":  time.February,
		"march":     time.March,
		"april":     time.April,
		"may":       time.May,
		"june":      time.June,
		"july":      time.July,
		"august":    time.August,
		"september": time.September,
		"october":   time.October,
		"november":  time.November,
		"december":  time.December,
		"jan":       time.January,
		"feb":       time.February,
		"mar":       time.March,
		"apr":       time.April,
		"jun":       time.June,
		"jul":       time.July,
		"aug":       time.August,
		"sep":       time.September,
		"oct":       time.October,
		"nov":       time.November,
		"dec":       time.December,
	}

	if month, ok := months[monthStr]; ok {
		return month, nil
	}

	return 0, fmt.Errorf("invalid month: %s", monthStr)
}

func getThisWeekDates() []time.Time {
	now := time.Now()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := now.AddDate(0, 0, 1-weekday)

	dates := []time.Time{}
	for i := 0; i < 5; i++ {
		dates = append(dates, monday.AddDate(0, 0, i))
	}
	return dates
}

func getLastWeekDates() []time.Time {
	now := time.Now()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	lastMonday := now.AddDate(0, 0, 1-weekday-7)

	dates := []time.Time{}
	for i := 0; i < 5; i++ {
		dates = append(dates, lastMonday.AddDate(0, 0, i))
	}
	return dates
}

func isTimeKeyword(word string) bool {
	keywords := []string{"today", "yesterday", "tomorrow", "week", "month", "this", "last", "next", "for"}
	for _, kw := range keywords {
		if word == kw {
			return true
		}
	}
	return false
}
