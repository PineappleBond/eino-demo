package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PineappleBond/eino-demo-dev/server/internal/eino/runner/skill"
	"gopkg.in/yaml.v3"
)

const skillFileName = "SKILL.md"

// SkillFileBackend implements skill.Backend by reading SKILL.md files
// from a directory on the local filesystem.
type SkillFileBackend struct {
	baseDir string
}

// NewSkillFileBackend creates a backend that scans baseDir for skill directories.
// Each immediate subdirectory containing a SKILL.md file is treated as one skill.
func NewSkillFileBackend(baseDir string) (*SkillFileBackend, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("baseDir is required")
	}
	// Verify baseDir exists
	info, err := os.Stat(baseDir)
	if err != nil {
		return nil, fmt.Errorf("skill base dir %q not accessible: %w", baseDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skill base dir %q is not a directory", baseDir)
	}
	return &SkillFileBackend{baseDir: baseDir}, nil
}

// List returns frontmatter metadata for all skills found in baseDir.
func (b *SkillFileBackend) List(_ context.Context) ([]skill.FrontMatter, error) {
	skills, err := b.loadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to load skills: %w", err)
	}
	matters := make([]skill.FrontMatter, 0, len(skills))
	for _, s := range skills {
		matters = append(matters, s.FrontMatter)
	}
	return matters, nil
}

// Get returns a single skill by name.
func (b *SkillFileBackend) Get(_ context.Context, name string) (skill.Skill, error) {
	skills, err := b.loadAll()
	if err != nil {
		return skill.Skill{}, fmt.Errorf("failed to load skills: %w", err)
	}
	for _, s := range skills {
		if s.Name == name {
			return skill.Skill{
				FrontMatter:   s.FrontMatter,
				Content:       s.Content,
				BaseDirectory: s.BaseDirectory,
			}, nil
		}
	}
	return skill.Skill{}, fmt.Errorf("skill not found: %s", name)
}

func (b *SkillFileBackend) loadAll() ([]skill.Skill, error) {
	entries, err := os.ReadDir(b.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill dir: %w", err)
	}

	var skills []skill.Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(b.baseDir, entry.Name(), skillFileName)
		s, err := b.loadSkillFromFile(skillPath)
		if err != nil {
			// Skip invalid skill directories
			continue
		}
		skills = append(skills, s)
	}
	return skills, nil
}

func (b *SkillFileBackend) loadSkillFromFile(path string) (skill.Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return skill.Skill{}, fmt.Errorf("failed to read %s: %w", path, err)
	}

	frontmatter, content, err := parseFrontmatter(string(data))
	if err != nil {
		return skill.Skill{}, fmt.Errorf("failed to parse frontmatter in %s: %w", path, err)
	}

	var fm skill.FrontMatter
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return skill.Skill{}, fmt.Errorf("failed to unmarshal frontmatter in %s: %w", path, err)
	}

	absDir := filepath.Dir(path)

	return skill.Skill{
		FrontMatter:   fm,
		Content:       strings.TrimSpace(content),
		BaseDirectory: absDir,
	}, nil
}

// parseFrontmatter extracts YAML frontmatter (delimited by ---) from raw text.
func parseFrontmatter(data string) (frontmatter string, content string, err error) {
	const delimiter = "---"

	data = strings.TrimSpace(data)

	if !strings.HasPrefix(data, delimiter) {
		return "", "", fmt.Errorf("file does not start with frontmatter delimiter")
	}

	rest := data[len(delimiter):]
	endIdx := strings.Index(rest, "\n"+delimiter)
	if endIdx == -1 {
		return "", "", fmt.Errorf("frontmatter closing delimiter not found")
	}

	frontmatter = strings.TrimSpace(rest[:endIdx])
	content = rest[endIdx+len("\n"+delimiter):]

	if strings.HasPrefix(content, "\n") {
		content = content[1:]
	}

	return frontmatter, content, nil
}
