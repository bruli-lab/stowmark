package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidRetentionDuration = errors.New("invalid retention duration")

const (
	day  = 24 * time.Hour
	week = 7 * day
)

func parseRetentionDuration(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	for suffix, unit := range map[string]time.Duration{"d": day, "w": week} {
		number, ok := strings.CutSuffix(value, suffix)
		if !ok {
			continue
		}
		count, err := strconv.Atoi(number)
		if err != nil || count < 0 {
			return 0, fmt.Errorf("%w: %q", ErrInvalidRetentionDuration, value)
		}
		return time.Duration(count) * unit, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidRetentionDuration, value)
	}
	return duration, nil
}
