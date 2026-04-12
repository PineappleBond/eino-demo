package tools

import (
	"testing"
)

// ── WeatherTool tests ──

func TestWeatherTool_Run(t *testing.T) {
	tool := NewWeatherTool()

	tests := []struct {
		name     string
		location string
		want     WeatherOutput
	}{
		{
			name:     "Beijing",
			location: "Beijing",
			want:     WeatherOutput{Location: "Beijing", Temperature: 22, Condition: "Sunny"},
		},
		{
			name:     "New York",
			location: "New York",
			want:     WeatherOutput{Location: "New York", Temperature: 22, Condition: "Sunny"},
		},
		{
			name:     "empty location",
			location: "",
			want:     WeatherOutput{Location: "", Temperature: 22, Condition: "Sunny"},
		},
		{
			name:     "unicode location",
			location: "东京",
			want:     WeatherOutput{Location: "东京", Temperature: 22, Condition: "Sunny"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tool.Run(WeatherInput{Location: tt.location})
			if got != tt.want {
				t.Errorf("Run(%q) = %+v, want %+v", tt.location, got, tt.want)
			}
		})
	}
}

func TestWeatherTool_NewReturnsNonNil(t *testing.T) {
	tool := NewWeatherTool()
	if tool == nil {
		t.Error("NewWeatherTool() returned nil")
	}
}
