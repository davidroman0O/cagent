package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/davidroman0O/cagent/internal/agent"
	"github.com/davidroman0O/cagent/internal/bench"
	"github.com/davidroman0O/cagent/internal/compat"
	"github.com/davidroman0O/cagent/internal/config"
	"github.com/davidroman0O/cagent/internal/modelcaps"
	"github.com/davidroman0O/cagent/internal/provider"
	aruntime "github.com/davidroman0O/cagent/internal/runtime"
	"github.com/davidroman0O/cagent/internal/server"
	"github.com/spf13/cobra"
)

func NewRoot(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "cagent",
		Short:         "OpenAI-compatible gateway for coding-agent CLIs",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return RunServe(cmd.Context(), config.Load(), log.Default())
		},
	}
	cmd.SetVersionTemplate("cagent {{.Version}}\n")
	cmd.AddCommand(NewServeCommand(version))
	cmd.AddCommand(NewBenchCommand(version))
	cmd.AddCommand(NewModelsCommand(version))
	return cmd
}

func NewServeCommand(version string) *cobra.Command {
	cfg := config.Load()
	tokenFlag := ""
	cmd := &cobra.Command{
		Use:           "serve",
		Short:         "Run the cagent HTTP server",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if tokenFlag != "" {
				cfg.Token = tokenFlag
			}
			if cfg.QueueLimit < cfg.MaxConcurrent {
				cfg.QueueLimit = cfg.MaxConcurrent
			}
			return RunServe(cmd.Context(), cfg, log.Default())
		},
	}
	cmd.SetVersionTemplate("cagent serve {{.Version}}\n")

	flags := cmd.Flags()
	flags.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP listen address")
	flags.StringVar(&tokenFlag, "token", "", "API token; defaults to CAGENT_TOKEN when unset")
	flags.StringVar(&cfg.CodexBin, "codex-bin", cfg.CodexBin, "Codex binary path")
	flags.StringVar(&cfg.DefaultCWD, "default-cwd", cfg.DefaultCWD, "default working directory for agent turns")
	flags.StringVar(&cfg.DefaultModel, "default-model", cfg.DefaultModel, "default cagent model id")
	flags.StringVar(&cfg.DefaultReasoningEffort, "default-reasoning-effort", cfg.DefaultReasoningEffort, "default reasoning effort: low, medium, high, or xhigh")
	flags.IntVar(&cfg.CodexModelContextWindow, "model-context-window", cfg.CodexModelContextWindow, "Codex model_context_window override")
	flags.IntVar(&cfg.CodexModelAutoCompactTokenLimit, "model-auto-compact-token-limit", cfg.CodexModelAutoCompactTokenLimit, "Codex model_auto_compact_token_limit override")
	flags.IntVar(&cfg.MaxConcurrent, "max-concurrent", cfg.MaxConcurrent, "maximum concurrent agent turns")
	flags.IntVar(&cfg.QueueLimit, "queue-limit", cfg.QueueLimit, "queued turn limit")
	flags.DurationVar(&cfg.RequestTimeout, "request-timeout", cfg.RequestTimeout, "per-request timeout")
	flags.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "runtime data directory")
	return cmd
}

func NewBenchCommand(version string) *cobra.Command {
	cfg := bench.Config{
		Mode:     bench.ModeCodexCLI,
		BaseURL:  "http://localhost:8080/v1",
		APIToken: os.Getenv("CAGENT_TOKEN"),
		CodexBin: os.Getenv("CAGENT_CODEX_BIN"),
		Timeout:  20 * time.Minute,
	}
	mode := string(cfg.Mode)
	targetsValue := "8192,16384,32768"
	apiTokenFlag := ""
	cmd := &cobra.Command{
		Use:           "bench",
		Aliases:       []string{"benchmark"},
		Short:         "Benchmark Codex CLI or cagent Responses output behavior",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			targets, err := parseTargets(targetsValue)
			if err != nil {
				return err
			}
			if apiTokenFlag != "" {
				cfg.APIToken = apiTokenFlag
			}
			cfg.Mode = bench.Mode(mode)
			cfg.Targets = targets
			report := bench.Run(cmd.Context(), cfg)
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(report)
		},
	}
	cmd.SetVersionTemplate("cagent bench {{.Version}}\n")

	flags := cmd.Flags()
	flags.StringVar(&mode, "mode", mode, "benchmark mode: codex-cli or responses")
	flags.StringVar(&cfg.BaseURL, "base-url", cfg.BaseURL, "cagent base URL for responses mode")
	flags.StringVar(&apiTokenFlag, "api-token", "", "cagent API token; defaults to CAGENT_TOKEN when unset")
	flags.StringVar(&cfg.CodexBin, "codex-bin", cfg.CodexBin, "Codex binary path")
	flags.StringVar(&cfg.Model, "model", cfg.Model, "model id; empty lets Codex config choose in codex-cli mode")
	flags.StringVar(&cfg.ReasoningEffort, "reasoning-effort", cfg.ReasoningEffort, "reasoning effort")
	flags.IntVar(&cfg.ModelContextWindow, "model-context-window", cfg.ModelContextWindow, "Codex model_context_window override")
	flags.IntVar(&cfg.ModelAutoCompactTokenLimit, "model-auto-compact-token-limit", cfg.ModelAutoCompactTokenLimit, "Codex model_auto_compact_token_limit override")
	flags.StringVar(&cfg.CWD, "cwd", cfg.CWD, "working directory")
	flags.StringVar(&targetsValue, "targets", targetsValue, "comma-separated requested output token targets")
	flags.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "per-target timeout")
	return cmd
}

func NewModelsCommand(version string) *cobra.Command {
	codexBin := os.Getenv("CAGENT_CODEX_BIN")
	configPath := ""
	format := "markdown"
	timeout := 15 * time.Second
	cmd := &cobra.Command{
		Use:           "models",
		Aliases:       []string{"model"},
		Short:         "Print the local Codex model capability matrix",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			caps, err := modelcaps.Build(ctx, codexBin, configPath)
			if err != nil {
				return err
			}
			switch format {
			case "markdown":
				_, err = fmt.Fprint(cmd.OutOrStdout(), modelcaps.MarkdownTable(caps))
				return err
			case "json":
				data, err := json.MarshalIndent(caps, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return err
			default:
				return fmt.Errorf("unknown format: %s", format)
			}
		},
	}
	cmd.SetVersionTemplate("cagent models {{.Version}}\n")

	flags := cmd.Flags()
	flags.StringVar(&codexBin, "codex-bin", codexBin, "Codex binary path")
	flags.StringVar(&configPath, "config", configPath, "Codex config.toml path")
	flags.StringVar(&format, "format", format, "output format: markdown or json")
	flags.DurationVar(&timeout, "timeout", timeout, "catalog extraction timeout")
	return cmd
}

func RunServe(ctx context.Context, cfg config.Config, logger *log.Logger) error {
	if logger == nil {
		logger = log.Default()
	}
	codexProvider, err := provider.NewCodexProviderWithOptions(provider.CodexOptions{
		Bin:                        cfg.CodexBin,
		ModelContextWindow:         cfg.CodexModelContextWindow,
		ModelAutoCompactTokenLimit: cfg.CodexModelAutoCompactTokenLimit,
	})
	if err != nil {
		return fmt.Errorf("codex provider: %w", err)
	}
	manager, err := aruntime.NewManager([]agent.Provider{codexProvider}, aruntime.Options{
		DefaultProvider: "codex",
		DataDir:         cfg.DataDir,
		MaxConcurrent:   cfg.MaxConcurrent,
		QueueLimit:      cfg.QueueLimit,
	})
	if err != nil {
		return fmt.Errorf("runtime: %w", err)
	}
	srv := server.New(server.Options{
		Manager: manager,
		Defaults: compat.AgentDefaults{
			Model:           cfg.DefaultModel,
			ReasoningEffort: cfg.DefaultReasoningEffort,
			CWD:             cfg.DefaultCWD,
		},
		Token:                      cfg.Token,
		Timeout:                    cfg.RequestTimeout,
		ModelContextWindow:         cfg.CodexModelContextWindow,
		ModelAutoCompactTokenLimit: cfg.CodexModelAutoCompactTokenLimit,
		Logger:                     logger,
	})

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Printf("cagent listening addr=%s", cfg.Addr)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func parseTargets(value string) ([]int, error) {
	parts := strings.Split(value, ",")
	targets := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parsed, err := strconv.Atoi(part)
		if err != nil {
			return nil, err
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("target must be positive: %d", parsed)
		}
		targets = append(targets, parsed)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("at least one target is required")
	}
	return targets, nil
}
