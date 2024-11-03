package gp

import (
	"math"
	"testing"
)

func TestLongCreation(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected int64
	}{
		{"zero", 0, 0},
		{"positive", 42, 42},
		{"negative", -42, -42},
		{"max_int64", math.MaxInt64, math.MaxInt64},
		{"min_int64", math.MinInt64, math.MinInt64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := MakeLong(tt.input)
			if got := l.Int64(); got != tt.expected {
				t.Errorf("MakeLong(%d) = %d; want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLongFromFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected int64
	}{
		{"integer_float", 42.0, 42},
		{"truncated_float", 42.9, 42},
		{"negative_float", -42.9, -42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := LongFromFloat64(tt.input)
			if got := l.Int64(); got != tt.expected {
				t.Errorf("LongFromFloat64(%f) = %d; want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLongFromString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		base     int
		expected int64
	}{
		{"decimal", "42", 10, 42},
		{"hex", "2A", 16, 42},
		{"binary", "101010", 2, 42},
		{"octal", "52", 8, 42},
		{"negative", "-42", 10, -42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := LongFromString(tt.input, tt.base)
			if got := l.Int64(); got != tt.expected {
				t.Errorf("LongFromString(%q, %d) = %d; want %d", tt.input, tt.base, got, tt.expected)
			}
		})
	}
}

func TestLongConversions(t *testing.T) {
	l := MakeLong(42)

	t.Run("Int", func(t *testing.T) {
		if got := l.Int(); got != 42 {
			t.Errorf("Int() = %d; want 42", got)
		}
	})

	t.Run("Uint", func(t *testing.T) {
		if got := l.Uint(); got != 42 {
			t.Errorf("Uint() = %d; want 42", got)
		}
	})

	t.Run("Uint64", func(t *testing.T) {
		if got := l.Uint64(); got != 42 {
			t.Errorf("Uint64() = %d; want 42", got)
		}
	})

	t.Run("Float64", func(t *testing.T) {
		if got := l.Float64(); got != 42.0 {
			t.Errorf("Float64() = %f; want 42.0", got)
		}
	})
}

func TestLongFromUintptr(t *testing.T) {
	tests := []struct {
		name     string
		input    uintptr
		expected int64
	}{
		{"zero", 0, 0},
		{"positive", 42, 42},
		{"large_number", 1 << 30, 1 << 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := LongFromUintptr(tt.input)
			if got := l.Int64(); got != tt.expected {
				t.Errorf("LongFromUintptr(%d) = %d; want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLongFromUnicode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		base     int
		expected int64
	}{
		{"unicode_decimal", "42", 10, 42},
		{"unicode_hex", "2A", 16, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create Unicode object from string
			u := MakeStr(tt.input)
			l := LongFromUnicode(u.Object, tt.base)
			if got := l.Int64(); got != tt.expected {
				t.Errorf("LongFromUnicode(%q, %d) = %d; want %d", tt.input, tt.base, got, tt.expected)
			}
		})
	}
}
