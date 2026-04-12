/*
 * Copyright 2025 CloudWeGo Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package skill

import (
	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/prompts"
)

const toolName = "skill"

// renderSystemPrompt renders the skill system instruction prompt.
func renderSystemPrompt(toolName string, useChinese bool) (string, error) {
	return prompts.RenderSelect("skill_system", map[string]string{"ToolName": toolName}, useChinese)
}

// renderToolDescriptionBase renders the tool description header (en/zh).
func renderToolDescriptionBase(useChinese bool) (string, error) {
	return prompts.RenderSelect("skill_tool_description", nil, useChinese)
}

// renderSkillList renders the available skills list from frontmatters.
func renderSkillList(skills []FrontMatter) (string, error) {
	type skillItem struct {
		Name        string
		Description string
	}
	items := make([]skillItem, len(skills))
	for i, s := range skills {
		items[i] = skillItem{Name: s.Name, Description: s.Description}
	}
	return prompts.Render("skill_available", map[string][]skillItem{"Skills": items})
}

// renderSkillParamDesc renders the skill parameter description.
func renderSkillParamDesc(useChinese bool) string {
	en := "The skill name (no arguments). E.g., \"pdf\" or \"xlsx\""
	zh := "Skill 名称（无需其他参数）。例如：\"pdf\" 或 \"xlsx\""
	if useChinese {
		return zh
	}
	return en
}

// renderResultLaunch renders the "launching skill" line.
func renderResultLaunch(skillName string, useChinese bool) (string, error) {
	return prompts.RenderSelect("skill_result_launch", map[string]string{"SkillName": skillName}, useChinese)
}

// renderResultContent renders the skill content section.
func renderResultContent(baseDir, content string, useChinese bool) (string, error) {
	return prompts.RenderSelect("skill_result_content", map[string]string{
		"BaseDirectory": baseDir,
		"Content":       content,
	}, useChinese)
}

// renderAgentResult renders the sub-agent execution result.
func renderAgentResult(skillName, result string, useChinese bool) (string, error) {
	return prompts.RenderSelect("skill_result_agent", map[string]string{
		"SkillName": skillName,
		"Result":    result,
	}, useChinese)
}
