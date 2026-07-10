package core

import (
	"fmt"

	harnessconfig "github.com/shxntanu/epsilon/core/config"
	"github.com/shxntanu/epsilon/core/contextwindow"
	"github.com/shxntanu/epsilon/core/events"
	"github.com/shxntanu/epsilon/core/permissions"
	"github.com/shxntanu/epsilon/core/prompts"
	"github.com/shxntanu/epsilon/core/skills"
	"github.com/shxntanu/epsilon/core/slash"
	"github.com/shxntanu/epsilon/core/tools"
	"github.com/shxntanu/epsilon/core/types"
)

type Option func(*Harness) error

func WithEventBufferSize(size int) Option {
	return func(h *Harness) error {
		h.eventBufferSize = size
		return nil
	}
}

func WithSessionIDGenerator(fn func() (string, error)) Option {
	return func(h *Harness) error {
		h.newSessionID = fn
		return nil
	}
}

func WithProvider(provider types.Provider) Option {
	return func(h *Harness) error {
		h.provider = provider
		return nil
	}
}

func WithPermissionBroker(broker permissions.Broker) Option {
	return func(h *Harness) error {
		h.permissionBroker = broker
		return nil
	}
}

func WithSlashCommand(command slash.Command) Option {
	return func(h *Harness) error {
		h.slashRegistry.Register(command)
		return nil
	}
}

func WithContextWindow(config contextwindow.Config) Option {
	return func(h *Harness) error {
		h.contextTracker = contextwindow.NewTracker(config)
		return nil
	}
}

func WithContextWindowTokens(maxTokens int) Option {
	return func(h *Harness) error {
		h.contextTracker = contextwindow.NewTracker(contextwindow.Config{
			MaxTokens: maxTokens,
		})
		return nil
	}
}

func WithSelectedModelInfo(model types.ModelInfo) Option {
	return func(h *Harness) error {
		h.selectedModel = &model
		if h.model == "" {
			h.model = model.ID
		}
		h.contextTracker = contextwindow.NewTracker(
			contextwindow.ConfigFromModelInfo(model, contextwindow.Config{}))
		return nil
	}
}

func WithRuntimeSettings(settings harnessconfig.Settings) Option {
	return func(h *Harness) error {
		h.configSettings = settings
		h.model = settings.Model
		h.effort = settings.Effort
		h.planMode = settings.PlanMode
		h.systemPrompt = settings.SystemPrompt
		return nil
	}
}

func WithPlanMode(enabled bool) Option {
	return func(h *Harness) error {
		h.planMode = enabled
		h.configSettings.PlanMode = enabled
		return nil
	}
}

func WithSystemPrompt(prompt string) Option {
	return func(h *Harness) error {
		h.systemPrompt = prompt
		h.configSettings.SystemPrompt = prompt
		return nil
	}
}

func WithPromptCatalog(catalog *prompts.Catalog) Option {
	return func(h *Harness) error {
		if catalog == nil {
			return fmt.Errorf("prompt catalog is nil")
		}
		h.promptCatalog = catalog
		return nil
	}
}

func WithPromptDefinition(definition prompts.Definition) Option {
	return func(h *Harness) error {
		if h.promptCatalog == nil {
			h.promptCatalog = prompts.DefaultCatalog()
		}
		return h.promptCatalog.Register(definition)
	}
}

func WithWorkspaceContext(workspaceRoot string) Option {
	return func(h *Harness) error {
		agentsMD, err := skills.LoadAgentsMD(workspaceRoot)
		if err != nil {
			return fmt.Errorf("load AGENTS.md: %w", err)
		}

		registry, err := skills.LoadRegistry(workspaceRoot)
		if err != nil {
			return fmt.Errorf("load skills registry: %w", err)
		}

		h.agentsMD = agentsMD
		h.skillRegistry = registry
		h.workspaceRoot = workspaceRoot
		return nil
	}
}

func WithConfigStore(store *harnessconfig.Store) Option {
	return func(h *Harness) error {
		h.configStore = store
		return nil
	}
}

func WithEventStore(store events.Store) Option {
	return func(h *Harness) error {
		h.eventStore = store
		return nil
	}
}

func WithSessionDir(dir string) Option {
	return func(h *Harness) error {
		store, err := events.NewJSONLStore(dir)
		if err != nil {
			return fmt.Errorf("create JSONL event store: %w", err)
		}

		h.eventStore = store
		return nil
	}
}

func WithTool(tool tools.Tool) Option {
	return func(h *Harness) error {
		return h.toolRegistry.Register(tool)
	}
}

func WithTools(toolList ...tools.Tool) Option {
	return func(h *Harness) error {
		for _, tool := range toolList {
			if err := h.toolRegistry.Register(tool); err != nil {
				return err
			}
		}

		return nil
	}
}

func WithDefaultTools(workspaceRoot string) Option {
	return func(h *Harness) error {
		readFileTool, err := tools.NewReadFileTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create read_file tool: %w", err)
		}

		writeFileTool, err := tools.NewWriteFileTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create write_file tool: %w", err)
		}

		grepTool, err := tools.NewGrepTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create grep_search tool: %w", err)
		}

		ripgrepTool, err := tools.NewRipgrepTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create ripgrep tool: %w", err)
		}

		listDirTool, err := tools.NewListDirTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create list_dir tool: %w", err)
		}

		fileTreeTool, err := tools.NewFileTreeTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create file_tree tool: %w", err)
		}

		gitStatusTool, err := tools.NewGitStatusTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create git_status tool: %w", err)
		}

		gitDiffTool, err := tools.NewGitDiffTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create git_diff tool: %w", err)
		}

		bashTool, err := tools.NewBashTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create bash tool: %w", err)
		}

		patchTool, err := tools.NewPatchTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create apply_patch tool: %w", err)
		}

		return WithTools(readFileTool, writeFileTool, grepTool, ripgrepTool, listDirTool,
			fileTreeTool, gitStatusTool, gitDiffTool, bashTool, patchTool)(h)
	}
}

func WithReadOnlyDefaultTools(workspaceRoot string) Option {
	return func(h *Harness) error {
		readFileTool, err := tools.NewReadFileTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create read_file tool: %w", err)
		}

		grepTool, err := tools.NewGrepTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create grep_search tool: %w", err)
		}

		ripgrepTool, err := tools.NewRipgrepTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create ripgrep tool: %w", err)
		}

		listDirTool, err := tools.NewListDirTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create list_dir tool: %w", err)
		}

		fileTreeTool, err := tools.NewFileTreeTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create file_tree tool: %w", err)
		}

		gitStatusTool, err := tools.NewGitStatusTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create git_status tool: %w", err)
		}

		gitDiffTool, err := tools.NewGitDiffTool(workspaceRoot)
		if err != nil {
			return fmt.Errorf("create git_diff tool: %w", err)
		}

		return h.toolRegistry.RegisterAll(
			readFileTool,
			grepTool,
			ripgrepTool,
			listDirTool,
			fileTreeTool,
			gitStatusTool,
			gitDiffTool,
		)
	}
}
