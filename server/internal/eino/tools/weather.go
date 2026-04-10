package tools

// WeatherInput is the input schema for the weather tool.
type WeatherInput struct {
	Location string `json:"location" jsonschema_description:"City or location name"`
}

// WeatherOutput is the output schema for the weather tool.
type WeatherOutput struct {
	Location    string `json:"location"`
	Temperature int    `json:"temperature"`
	Condition   string `json:"condition"`
}

// WeatherTool is a mock weather tool for demo purposes.
type WeatherTool struct{}

// NewWeatherTool creates a weather tool.
func NewWeatherTool() *WeatherTool {
	return &WeatherTool{}
}

// Run returns mock weather data for the given location.
func (t *WeatherTool) Run(input WeatherInput) WeatherOutput {
	return WeatherOutput{
		Location:    input.Location,
		Temperature: 22,
		Condition:   "Sunny",
	}
}
