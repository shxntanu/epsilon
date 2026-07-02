package skills

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Skill struct {
	Name        string
	Source      string
	Description string
	Content     string
	Path        string
}

type Registry struct {
	skills []Skill
	byName map[string]Skill
}

type lockfile struct {
	Skills map[string]lockedSkill `json:"skills"`
}

type lockedSkill struct {
	Source    string `json:"source"`
	SkillPath string `json:"skillPath"`
}

func LoadRegistry(workspaceRoot string) (*Registry, error) {
	root, err := filepath.Abs(strings.TrimSpace(workspaceRoot))
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "skills-lock.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return NewRegistry(nil), nil
		}
		return nil, fmt.Errorf("read skills-lock.json: %w", err)
	}

	var lock lockfile
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("parse skills-lock.json: %w", err)
	}

	names := make([]string, 0, len(lock.Skills))
	for name := range lock.Skills {
		names = append(names, name)
	}
	sort.Strings(names)

	loaded := make([]Skill, 0, len(names))
	for _, name := range names {
		spec := lock.Skills[name]
		skill, err := loadSkill(root, name, spec)
		if err != nil {
			log.Printf("epsilon: warning: skip skill %q: %v", name, err)
			continue
		}
		loaded = append(loaded, skill)
	}

	return NewRegistry(loaded), nil
}

func NewRegistry(skillList []Skill) *Registry {
	registry := &Registry{
		skills: append([]Skill(nil), skillList...),
		byName: make(map[string]Skill, len(skillList)),
	}
	for _, skill := range registry.skills {
		registry.byName[skill.Name] = skill
	}
	return registry
}

func (r *Registry) Skills() []Skill {
	if r == nil {
		return nil
	}
	return append([]Skill(nil), r.skills...)
}

func (r *Registry) Get(name string) (Skill, bool) {
	if r == nil {
		return Skill{}, false
	}
	skill, ok := r.byName[strings.TrimSpace(name)]
	return skill, ok
}

func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.skills)
}

func loadSkill(workspaceRoot string, name string, spec lockedSkill) (Skill, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Skill{}, fmt.Errorf("skill name is empty")
	}
	path, err := resolveSkillPath(workspaceRoot, spec.SkillPath)
	if err != nil {
		return Skill{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, fmt.Errorf("read SKILL.md: %w", err)
	}
	content := strings.TrimSpace(string(data))
	description := parseFrontmatterDescription(content)
	if strings.TrimSpace(description) == "" {
		description = strings.TrimSpace(spec.Source)
	}
	return Skill{
		Name:        name,
		Source:      strings.TrimSpace(spec.Source),
		Description: description,
		Content:     content,
		Path:        path,
	}, nil
}

func resolveSkillPath(workspaceRoot string, skillPath string) (string, error) {
	skillPath = strings.TrimSpace(skillPath)
	if skillPath == "" {
		return "", fmt.Errorf("skillPath is empty")
	}
	if filepath.IsAbs(skillPath) {
		return "", fmt.Errorf("skillPath must be workspace-relative")
	}

	path := filepath.Clean(filepath.Join(workspaceRoot, skillPath))
	rel, err := filepath.Rel(workspaceRoot, path)
	if err != nil {
		return "", fmt.Errorf("validate skillPath: %w", err)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("skillPath escapes workspace")
	}
	return path, nil
}

func parseFrontmatterDescription(content string) string {
	content = strings.TrimLeft(content, "\ufeff\r\n\t ")
	if !strings.HasPrefix(content, "---") {
		return ""
	}

	lines := strings.Split(content, "\n")
	if strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(strings.TrimSuffix(lines[i], "\r"))
		if line == "---" {
			return ""
		}
		if key, value, ok := strings.Cut(line, ":"); ok &&
			strings.EqualFold(strings.TrimSpace(key), "description") {
			return unquoteYAMLScalar(strings.TrimSpace(value))
		}
	}
	return ""
}

func unquoteYAMLScalar(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') ||
		(value[0] == '\'' && value[len(value)-1] == '\'') {
		return strings.TrimSpace(value[1 : len(value)-1])
	}
	return value
}
