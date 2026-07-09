package handlers

import (
	"testing"
)

func TestDisplayPhone(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"us_number", "+17065551234", "(706) 555-1234"},
		{"us_zeros", "+17065550101", "(706) 555-0101"},
		{"non_us_plus", "+441234567890", "+441234567890"},
		{"ten_digits_no_plus", "7065551234", "7065551234"},
		{"empty", "", ""},
		{"too_long", "+170655512345", "+170655512345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := displayPhone(tt.in)
			if got != tt.want {
				t.Errorf("displayPhone(%q) = %q; want %q", tt.in, got, tt.want)
			}
		})
	}
}
