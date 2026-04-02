package service

import (
	"fmt"
	"strings"
)

const (
	shift0700 = "shift-0700"
	shift1500 = "shift-1500"
	shift2300 = "shift-2300"
)

func normalizeShiftID(raw string) (string, error) {
	shiftID := strings.TrimSpace(strings.ToLower(raw))
	if shiftID == "" {
		return "", fmt.Errorf("%w: shift_id is required", ErrValidation)
	}

	switch shiftID {
	case shift0700, "0700", "07:00", "7:00", "700":
		return shift0700, nil
	case shift1500, "1500", "15:00", "3:00pm", "3pm":
		return shift1500, nil
	case shift2300, "2300", "23:00", "11:00pm", "11pm":
		return shift2300, nil
	default:
		return "", fmt.Errorf("%w: shift_id must be one of shift-0700, shift-1500, shift-2300", ErrValidation)
	}
}
