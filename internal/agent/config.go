package agent

import (
	"time"

	"github.com/spf13/viper"
)

// WorkflowConfig holds per-workflow configuration.
// Either Command or PromptFile should be set:
//   - Command: a raw shell command (run via sh -c)
//   - PromptFile: path to a markdown skill/prompt file passed to `claude --print`
type WorkflowConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Command    string `mapstructure:"command"`
	PromptFile string `mapstructure:"prompt_file"`
}

// AgentConfig holds all agent-related configuration loaded from viper.
type AgentConfig struct {
	Enabled              bool                      `mapstructure:"enabled"`
	IdleThresholdMinutes int                       `mapstructure:"idle_threshold_minutes"`
	CooldownMinutes      int                       `mapstructure:"cooldown_minutes"`
	MaxPatchSizeKB       int                       `mapstructure:"max_patch_size_kb"`
	Workflows            map[string]WorkflowConfig `mapstructure:"workflows"`
	DisabledRepos        []string                  `mapstructure:"disabled_repos"`
}

// LoadConfig reads agent configuration from viper with sensible defaults.
func LoadConfig() AgentConfig {
	viper.SetDefault("agent.enabled", true)
	viper.SetDefault("agent.idle_threshold_minutes", 20)
	viper.SetDefault("agent.cooldown_minutes", 60)
	viper.SetDefault("agent.max_patch_size_kb", 1024)
	viper.SetDefault("agent.workflows.code_review.enabled", false)
	viper.SetDefault("agent.workflows.code_review.command", "")
	viper.SetDefault("agent.workflows.code_review.prompt_file", "")
	viper.SetDefault("agent.workflows.test_generation.enabled", false)
	viper.SetDefault("agent.workflows.test_generation.prompt_file", "")
	viper.SetDefault("agent.disabled_repos", []string{})

	cfg := AgentConfig{
		Enabled:              viper.GetBool("agent.enabled"),
		IdleThresholdMinutes: viper.GetInt("agent.idle_threshold_minutes"),
		CooldownMinutes:      viper.GetInt("agent.cooldown_minutes"),
		MaxPatchSizeKB:       viper.GetInt("agent.max_patch_size_kb"),
		DisabledRepos:        viper.GetStringSlice("agent.disabled_repos"),
		Workflows:            make(map[string]WorkflowConfig),
	}

	// Load workflow configs. Viper doesn't natively unmarshal nested maps well,
	// so we read known workflows explicitly.
	// Load known workflows + any custom ones from config.
	knownWorkflows := []string{"code_review", "test_generation"}
	// Also discover any extra workflow keys from viper.
	allWorkflows := make(map[string]bool)
	for _, wf := range knownWorkflows {
		allWorkflows[wf] = true
	}
	if sub := viper.GetStringMap("agent.workflows"); sub != nil {
		for wf := range sub {
			allWorkflows[wf] = true
		}
	}
	for wf := range allWorkflows {
		cfg.Workflows[wf] = WorkflowConfig{
			Enabled:    viper.GetBool("agent.workflows." + wf + ".enabled"),
			Command:    viper.GetString("agent.workflows." + wf + ".command"),
			PromptFile: viper.GetString("agent.workflows." + wf + ".prompt_file"),
		}
	}

	return cfg
}

// IsRepoDisabled returns true if the given repo path is in the disabled list.
func (c AgentConfig) IsRepoDisabled(repoPath string) bool {
	for _, r := range c.DisabledRepos {
		if r == repoPath {
			return true
		}
	}
	return false
}

// IdleThreshold returns the idle threshold as a time.Duration.
func (c AgentConfig) IdleThreshold() time.Duration {
	return time.Duration(c.IdleThresholdMinutes) * time.Minute
}

// Cooldown returns the cooldown period as a time.Duration.
func (c AgentConfig) Cooldown() time.Duration {
	return time.Duration(c.CooldownMinutes) * time.Minute
}
