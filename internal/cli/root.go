package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/davidroman0O/cagent/internal/agent"
	"github.com/davidroman0O/cagent/internal/bench"
	"github.com/davidroman0O/cagent/internal/compat"
	"github.com/davidroman0O/cagent/internal/config"
	"github.com/davidroman0O/cagent/internal/droid"
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
	cmd.AddCommand(NewDroidCommand(version))
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

func NewDroidCommand(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "droid",
		Short:         "Configure and launch Factory Droid through cagent",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
	}
	cmd.SetVersionTemplate("cagent droid {{.Version}}\n")
	cmd.AddCommand(NewDroidSetupCommand(version))
	cmd.AddCommand(NewDroidDoctorCommand(version))
	cmd.AddCommand(NewDroidExecCommand(version))
	cmd.AddCommand(NewDroidLaunchCommand(version))
	return cmd
}

func NewDroidSetupCommand(version string) *cobra.Command {
	opts := droid.NormalizeSetupOptions(droid.SetupOptions{
		SetSessionDefault:  true,
		SetMissionDefaults: true,
		SkipScrutiny:       true,
		SkipUserTesting:    true,
		Backup:             true,
	})
	cmd := &cobra.Command{
		Use:           "setup",
		Short:         "Write Droid custom models, mission defaults, and 1M compaction settings",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := droid.ApplySettingsFile(opts)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "updated %s\n", result.SettingsPath)
			if result.BackupPath != "" {
				fmt.Fprintf(out, "backup %s\n", result.BackupPath)
			}
			fmt.Fprintf(out, "selected model %s\n", result.SelectedModelID)
			fmt.Fprintf(out, "custom models %d\n", len(result.CustomModelIDs))
			fmt.Fprintf(out, "compaction keys %d\n", len(result.CompactionKeys))
			if result.SessionDefaultSet {
				fmt.Fprintln(out, "session default set")
			}
			if result.MissionDefaultsSet {
				fmt.Fprintln(out, "mission defaults set")
			}
			return nil
		},
	}
	cmd.SetVersionTemplate("cagent droid setup {{.Version}}\n")

	flags := cmd.Flags()
	flags.StringVar(&opts.SettingsPath, "settings", opts.SettingsPath, "Factory Droid settings.json path")
	flags.StringVar(&opts.BaseURL, "base-url", opts.BaseURL, "cagent OpenAI-compatible base URL")
	flags.StringVar(&opts.APIToken, "api-token", opts.APIToken, "Droid custom model API token; defaults to CAGENT_TOKEN or local-cagent-token")
	flags.StringVar(&opts.CodexModel, "codex-model", opts.CodexModel, "Codex model to expose through cagent")
	flags.StringVar(&opts.ReasoningEffort, "reasoning-effort", opts.ReasoningEffort, "Droid and Codex reasoning effort")
	flags.IntVar(&opts.MaxContextLimit, "max-context-limit", opts.MaxContextLimit, "Droid custom model maxContextLimit")
	flags.IntVar(&opts.CompactionTokenLimit, "compaction-token-limit", opts.CompactionTokenLimit, "Droid compaction limit for cagent models")
	flags.IntVar(&opts.SafeMaxOutputTokens, "safe-max-output-tokens", opts.SafeMaxOutputTokens, "safe Droid maxOutputTokens profile")
	flags.IntVar(&opts.MaxOutputTokens, "max-output-tokens", opts.MaxOutputTokens, "aggressive Droid maxOutputTokens profile")
	flags.BoolVar(&opts.SetSessionDefault, "set-session-default", opts.SetSessionDefault, "set Droid's default interactive model to cagent")
	flags.BoolVar(&opts.SetMissionDefaults, "set-mission-defaults", opts.SetMissionDefaults, "set Droid orchestrator, worker, and validator models to cagent")
	flags.BoolVar(&opts.SkipScrutiny, "skip-scrutiny", opts.SkipScrutiny, "skip Droid's automatic scrutiny validator for faster out-of-box missions")
	flags.BoolVar(&opts.SkipUserTesting, "skip-user-testing", opts.SkipUserTesting, "skip Droid's automatic user-testing validator for faster out-of-box missions")
	flags.BoolVar(&opts.Backup, "backup", opts.Backup, "write a timestamped settings backup before updating")
	return cmd
}

func NewDroidDoctorCommand(version string) *cobra.Command {
	opts := droid.NormalizeSetupOptions(droid.SetupOptions{
		SetSessionDefault:  true,
		SetMissionDefaults: true,
		SkipScrutiny:       true,
		SkipUserTesting:    true,
	})
	cmd := &cobra.Command{
		Use:           "doctor",
		Short:         "Check whether Droid is configured for cagent mission mode",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := droid.CheckSettingsFile(opts)
			if err != nil {
				return err
			}
			for _, check := range report.Checks {
				status := "ok"
				if !check.OK {
					status = "fix"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s\n", status, check.Message)
			}
			if !report.OK {
				return fmt.Errorf("Droid settings are not cagent-ready; run cagent droid setup")
			}
			return nil
		},
	}
	cmd.SetVersionTemplate("cagent droid doctor {{.Version}}\n")

	flags := cmd.Flags()
	flags.StringVar(&opts.SettingsPath, "settings", opts.SettingsPath, "Factory Droid settings.json path")
	flags.StringVar(&opts.BaseURL, "base-url", opts.BaseURL, "expected cagent base URL")
	flags.StringVar(&opts.APIToken, "api-token", opts.APIToken, "expected API token; defaults to CAGENT_TOKEN or local-cagent-token")
	flags.StringVar(&opts.CodexModel, "codex-model", opts.CodexModel, "expected Codex model")
	flags.StringVar(&opts.ReasoningEffort, "reasoning-effort", opts.ReasoningEffort, "expected reasoning effort")
	flags.IntVar(&opts.CompactionTokenLimit, "compaction-token-limit", opts.CompactionTokenLimit, "expected compaction limit")
	flags.IntVar(&opts.MaxContextLimit, "max-context-limit", opts.MaxContextLimit, "expected maxContextLimit")
	flags.BoolVar(&opts.SkipScrutiny, "skip-scrutiny", opts.SkipScrutiny, "expected skipScrutiny value")
	flags.BoolVar(&opts.SkipUserTesting, "skip-user-testing", opts.SkipUserTesting, "expected skipUserTesting value")
	return cmd
}

func NewDroidExecCommand(version string) *cobra.Command {
	droidBin := "droid"
	printOnly := false
	opts := droid.ExecOptions{
		Mission:         true,
		Model:           droid.DefaultSelectedModelID(droid.DefaultCodexModel, droid.DefaultReasoningEffort),
		ReasoningEffort: droid.DefaultReasoningEffort,
	}
	cmd := &cobra.Command{
		Use:           "exec [prompt]",
		Short:         "Run droid exec with cagent mission model overrides",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			droidArgs := droid.ExecArgs(opts, args)
			if printOnly {
				fmt.Fprintln(cmd.OutOrStdout(), shellCommand(droidBin, droidArgs))
				return nil
			}
			return runExternal(cmd.Context(), droidBin, droidArgs)
		},
	}
	cmd.SetVersionTemplate("cagent droid exec {{.Version}}\n")

	flags := cmd.Flags()
	flags.StringVar(&droidBin, "droid-bin", droidBin, "Droid binary path")
	flags.BoolVar(&printOnly, "print", printOnly, "print the droid command without running it")
	flags.StringVar(&opts.SettingsPath, "settings", opts.SettingsPath, "runtime settings file to pass to Droid")
	flags.StringVar(&opts.CWD, "cwd", opts.CWD, "working directory for Droid")
	flags.StringVar(&opts.Model, "model", opts.Model, "Droid orchestrator model id")
	flags.StringVar(&opts.ReasoningEffort, "reasoning-effort", opts.ReasoningEffort, "Droid orchestrator reasoning effort")
	flags.BoolVar(&opts.Mission, "mission", opts.Mission, "run droid exec in mission mode")
	flags.StringVar(&opts.Auto, "auto", opts.Auto, "Droid autonomy level")
	flags.StringVar(&opts.WorkerModel, "worker-model", opts.WorkerModel, "Droid mission worker model id")
	flags.StringVar(&opts.WorkerReasoningEffort, "worker-reasoning-effort", opts.WorkerReasoningEffort, "Droid mission worker reasoning effort")
	flags.StringVar(&opts.ValidatorModel, "validator-model", opts.ValidatorModel, "Droid mission validator model id")
	flags.StringVar(&opts.ValidatorReasoningEffort, "validator-reasoning-effort", opts.ValidatorReasoningEffort, "Droid mission validator reasoning effort")
	flags.BoolVar(&opts.ListTools, "list-tools", opts.ListTools, "list Droid tools for the selected model")
	return cmd
}

func NewDroidLaunchCommand(version string) *cobra.Command {
	droidBin := "droid"
	settingsPath := ""
	cwd := ""
	printOnly := false
	cmd := &cobra.Command{
		Use:           "launch [prompt]",
		Short:         "Launch interactive Droid after cagent droid setup",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			droidArgs := droid.LaunchArgs(settingsPath, cwd, args)
			if printOnly {
				fmt.Fprintln(cmd.OutOrStdout(), shellCommand(droidBin, droidArgs))
				return nil
			}
			return runExternal(cmd.Context(), droidBin, droidArgs)
		},
	}
	cmd.SetVersionTemplate("cagent droid launch {{.Version}}\n")
	flags := cmd.Flags()
	flags.StringVar(&droidBin, "droid-bin", droidBin, "Droid binary path")
	flags.StringVar(&settingsPath, "settings", settingsPath, "runtime settings file to pass to Droid")
	flags.StringVar(&cwd, "cwd", cwd, "working directory for Droid")
	flags.BoolVar(&printOnly, "print", printOnly, "print the droid command without running it")
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

func runExternal(ctx context.Context, bin string, args []string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func shellCommand(bin string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, strconv.Quote(bin))
	for _, arg := range args {
		parts = append(parts, strconv.Quote(arg))
	}
	return strings.Join(parts, " ")
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
