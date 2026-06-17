package prompts

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed defaults/*.txt
var defaultPromptFiles embed.FS

type ID string

const (
	Agent   ID = "agent"
	Summary ID = "summary"
	Title   ID = "title"
)

type Section struct {
	Name string
	Text string
}

type Definition struct {
	ID          ID
	Description string
	Sections    []Section
}

func (d Definition) Render(extensions ...string) string {
	parts := make([]string, 0, len(d.Sections)+len(extensions))
	for _, section := range d.Sections {
		text := strings.TrimSpace(section.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	for _, extension := range extensions {
		text := strings.TrimSpace(extension)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n\n")
}

func (d Definition) validate() error {
	if strings.TrimSpace(string(d.ID)) == "" {
		return fmt.Errorf("prompt ID cannot be empty")
	}
	if strings.TrimSpace(d.Render()) == "" {
		return fmt.Errorf("prompt %q has no text", d.ID)
	}
	return nil
}

type Catalog struct {
	definitions map[ID]Definition
}

func NewCatalog(definitions ...Definition) (*Catalog, error) {
	catalog := &Catalog{
		definitions: make(map[ID]Definition),
	}
	for _, definition := range definitions {
		if err := catalog.Register(definition); err != nil {
			return nil, err
		}
	}
	return catalog, nil
}

func DefaultCatalog() *Catalog {
	catalog, err := NewCatalog(DefaultAgent(), DefaultSummary(), DefaultTitle())
	if err != nil {
		panic(err)
	}
	return catalog
}

func (c *Catalog) Register(definition Definition) error {
	if c == nil {
		return fmt.Errorf("prompt catalog is nil")
	}
	if err := definition.validate(); err != nil {
		return err
	}
	c.definitions[definition.ID] = cloneDefinition(definition)
	return nil
}

func (c *Catalog) Definition(id ID) (Definition, bool) {
	if c == nil {
		return Definition{}, false
	}
	definition, ok := c.definitions[id]
	if !ok {
		return Definition{}, false
	}
	return cloneDefinition(definition), true
}

func (c *Catalog) Render(id ID, extensions ...string) (string, bool) {
	definition, ok := c.Definition(id)
	if !ok {
		return "", false
	}
	return definition.Render(extensions...), true
}

func (c *Catalog) IDs() []ID {
	if c == nil {
		return nil
	}
	ids := make([]ID, 0, len(c.definitions))
	for id := range c.definitions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i int, j int) bool {
		return ids[i] < ids[j]
	})
	return ids
}

func DefaultAgent() Definition {
	return Definition{
		ID:          Agent,
		Description: "Default coding-agent instructions for normal user sessions.",
		Sections: []Section{
			{
				Name: "default",
				Text: mustDefaultPrompt("defaults/agent.txt"),
			},
		},
	}
}

func DefaultSummary() Definition {
	return Definition{
		ID:          Summary,
		Description: "Instructions for future conversation or session summarization agents.",
		Sections: []Section{
			{
				Name: "default",
				Text: mustDefaultPrompt("defaults/summary.txt"),
			},
		},
	}
}

func DefaultTitle() Definition {
	return Definition{
		ID:          Title,
		Description: "Instructions for future session title generation agents.",
		Sections: []Section{
			{
				Name: "default",
				Text: mustDefaultPrompt("defaults/title.txt"),
			},
		},
	}
}

func mustDefaultPrompt(path string) string {
	data, err := defaultPromptFiles.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("read embedded prompt %q: %v", path, err))
	}
	return strings.TrimSpace(string(data))
}

func cloneDefinition(definition Definition) Definition {
	definition.Sections = append([]Section(nil), definition.Sections...)
	return definition
}
