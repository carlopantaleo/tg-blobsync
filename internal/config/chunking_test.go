package config

import "testing"

func TestValidateChunkConfig(t *testing.T) {
	valid := []struct {
		name      string
		threshold int64
		size      int64
	}{
		{"defaults", DefaultChunkThreshold, DefaultChunkSize},
		{"equal", 100, 100},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateChunkConfig(test.threshold, test.size); err != nil {
				t.Fatalf("validate config: %v", err)
			}
		})
	}
	for _, test := range []struct {
		name      string
		threshold int64
		size      int64
	}{
		{"zero threshold", 0, 1},
		{"zero size", 100, 0},
		{"size exceeds threshold", 100, 101},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateChunkConfig(test.threshold, test.size); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
