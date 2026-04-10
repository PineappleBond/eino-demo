package templates

func init() {
	Register(TemplateDetail{
		TemplateInfo: TemplateInfo{
			ID:          "01",
			Name:        "Basic Agent",
			Description: "A single ChatModelAgent with tools and ReAct loop. The simplest Eino integration pattern.",
			Tags:        []string{"agent", "tools", "react"},
			Difficulty:  "beginner",
		},
		Agents: []TemplateAgentInfo{
			{
				AgentKey:     "basic",
				AgentName:    "Basic Agent",
				Description:  "A chat agent with tool support and ReAct loop.",
				SystemPrompt: "You are a helpful assistant. Use tools when appropriate to answer questions.",
			},
		},
		Tools: []TemplateToolInfo{
			{Name: "weather", Description: "Get current weather for a location"},
		},
		InitialPrompt: "Hello! I'm a basic agent. I can answer questions and use tools. Try asking me about the weather!",
	})
}
