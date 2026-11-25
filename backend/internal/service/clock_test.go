package service

import (
	"errors"
	"testing"
)

func TestStartDurationValidation(t *testing.T) {
	c := NewClock(nil, 5400)

	tooShort := MinDurationSeconds - 1
	if _, err := c.Start(nil, &tooShort); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Start() with %ds: got err=%v, want ErrInvalidInput", tooShort, err)
	}

	tooLong := MaxDurationSeconds + 1
	if _, err := c.Start(nil, &tooLong); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Start() with %ds: got err=%v, want ErrInvalidInput", tooLong, err)
	}
}
