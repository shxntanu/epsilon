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

	lock, err := loadLockfile(root)
	if err != nil {
		return nil, err
	}

	loaded := make(map[string]Skill)
	for _, source := range registrySources(root, lock) {
		for _, path := range discoverSkillPaths(source.Root) {
			name := skillNameFromPath(source.Root, path)
			if source.Kind == "lockfile" {
				name = source.Name
			}
			skill, err := loadSkillFile(path, name, source.Source)
			if err != nil {
				log.Printf("epsilon: warning: skip skill %q: %v", name, err)
				continue
			}
			if _, exists := loaded[skill.Name]; exists {
				continue
			}
			loaded[skill.Name] = skill
		}
	}

	names := make([]string, 0, len(loaded))
	for name := range loaded {
		names = append(names, name)
	}
	sort.Strings(names)

	skillList := make([]Skill, 0, len(names))
	for _, name := range names {
		skillList = append(skillList, loaded[name])
	}
	return NewRegistry(skillList), nil
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

type registrySource struct {
	Kind   string
	Root   string
	Name   string
	Source string
}

func loadLockfile(root string) (lockfile, error) {
	data, err := os.ReadFile(filepath.Join(root, "skills-lock.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return lockfile{}, nil
		}
		return lockfile{}, fmt.Errorf("read skills-lock.json: %w", err)
	}

	var lock lockfile
	if err := json.Unmarshal(data, &lock); err != nil {
		return lockfile{}, fmt.Errorf("parse skills-lock.json: %w", err)
	}
	return lock, nil
}

func registrySources(workspaceRoot string, lock lockfile) []registrySource {
	sources := make([]registrySource, 0, len(lock.Skills)+2)

	names := make([]string, 0, len(lock.Skills))
	for name := range lock.Skills {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		spec := lock.Skills[name]
		path, err := resolveSkillPath(workspaceRoot, spec.SkillPath)
		if err != nil {
			log.Printf("epsilon: warning: skip skill %q: %v", name, err)
			continue
		}
		sources = append(sources, registrySource{
			Kind:   "lockfile",
			Root:   path,
			Name:   name,
			Source: strings.TrimSpace(spec.Source),
		})
	}

	sources = append(sources, registrySource{
		Kind:   "project",
		Root:   filepath.Join(workspaceRoot, ".agents", "skills"),
		Source: "project .agents/skills",
	})
	sources = append(sources, registrySource{
		Kind:   "project",
		Root:   filepath.Join(workspaceRoot, ".agents"),
		Source: "project .agents",
	})
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		sources = append(sources, registrySource{
			Kind:   "global",
			Root:   filepath.Join(home, ".agents", "skills"),
			Source: "global ~/.agents/skills",
		})
		sources = append(sources, registrySource{
			Kind:   "global",
			Root:   filepath.Join(home, ".agents"),
			Source: "global ~/.agents",
		})
	}
	return sources
}

func discoverSkillPaths(root string) []string {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	if strings.EqualFold(filepath.Base(root), "SKILL.md") {
		if _, err := os.Stat(root); err == nil {
			return []string{root}
		}
		return nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("epsilon: warning: scan skills %q: %v", root, err)
		}
		return nil
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "SKILL.md")
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

func skillNameFromPath(root string, path string) string {
	name := filepath.Base(filepath.Dir(path))
	if rel, err := filepath.Rel(root, path); err == nil && rel != "." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." {
		first, _, _ := strings.Cut(rel, string(filepath.Separator))
		if first != "" && first != "SKILL.md" {
			name = first
		}
	}
	return name
}

func loadSkillFile(path string, name string, source string) (Skill, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Skill{}, fmt.Errorf("skill name is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, fmt.Errorf("read SKILL.md: %w", err)
	}
	content := strings.TrimSpace(string(data))
	description := parseFrontmatterDescription(content)
	if strings.TrimSpace(description) == "" {
		description = strings.TrimSpace(source)
	}
	return Skill{
		Name:        name,
		Source:      strings.TrimSpace(source),
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
