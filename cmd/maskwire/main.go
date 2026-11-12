package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Guilherme-Ciano/maskwire/internal/app"
	"github.com/Guilherme-Ciano/maskwire/internal/audit"
	"github.com/Guilherme-Ciano/maskwire/internal/config"
	"github.com/Guilherme-Ciano/maskwire/internal/detect"
	"github.com/Guilherme-Ciano/maskwire/internal/entity"
	"github.com/Guilherme-Ciano/maskwire/internal/mask"
	"github.com/Guilherme-Ciano/maskwire/internal/policy"
	"github.com/Guilherme-Ciano/maskwire/internal/telemetry"
	"github.com/Guilherme-Ciano/maskwire/internal/token"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var errUnhealthy = errors.New("health check failed")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := newRoot().ExecuteContext(ctx)
	stop()
	if err != nil {
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "maskwire",
		Short:         "Reversible, streaming-safe PII pseudonymisation proxy for LLM APIs",
		SilenceUsage:  true,
		SilenceErrors: false,
		Version:       fmt.Sprintf("%s (commit %s, built %s)", version, commit, date),
	}
	root.AddCommand(serveCmd(), detectCmd(), auditCmd(), hashKeyCmd(), healthcheckCmd())
	return root
}

func configPath(flag string) string {
	if flag != "" {
		return flag
	}
	return os.Getenv("MASKWIRE_CONFIG")
}

func serveCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the proxy",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath(path), os.Environ)
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("invalid configuration: %w", err)
			}
			log := telemetry.NewLogger(os.Stdout, cfg.Log.Level, cfg.Log.Format)
			a, err := app.New(cfg, log)
			if err != nil {
				return err
			}
			return a.Run(cmd.Context())
		},
	}
	cmd.Flags().StringVar(&path, "config", "", "path to the YAML config file (or MASKWIRE_CONFIG)")
	return cmd
}

func detectCmd() *cobra.Command {
	var tenant string
	cmd := &cobra.Command{
		Use:   "detect",
		Short: "Mask stdin with an ephemeral key and print the result and entity counts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDetect(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), tenant)
		},
	}
	cmd.Flags().StringVar(&tenant, "tenant", "cli", "tenant id used to derive the ephemeral token key")
	return cmd
}

func runDetect(stdin io.Reader, stdout, stderr io.Writer, tenant string) error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	kr, err := token.NewKeyring(key, 1)
	if err != nil {
		return err
	}
	tk, err := kr.Tenant(tenant)
	if err != nil {
		return err
	}
	in, err := io.ReadAll(io.LimitReader(stdin, 16<<20))
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	s := mask.NewEngine(detect.New(detect.Options{}), 1<<20).NewSession(&mask.Tenant{
		ID: tenant, Key: tk, Policy: policy.New(policy.Enforce, policy.Mask),
	})
	out, err := s.MaskText(nil, in)
	if err != nil {
		return err
	}
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	if _, err := stdout.Write(out); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	for t, n := range s.Counters().Detected {
		if n > 0 {
			if _, err := fmt.Fprintf(stderr, "%s\t%d\n", entity.Type(t).String(), n); err != nil {
				return fmt.Errorf("write counts: %w", err)
			}
		}
	}
	return nil
}

func auditCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "audit", Short: "Audit log tools"}
	var dir string
	verify := &cobra.Command{
		Use:   "verify [file...]",
		Short: "Verify the hash chain of audit files (a directory or explicit files)",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if len(args) > 0 {
				for _, f := range args {
					n, head, err := audit.VerifyFile(f)
					if err != nil {
						return err
					}
					fmt.Fprintf(out, "ok\t%s\trecords=%d\thead=sha256:%x\n", f, n, head)
				}
				return nil
			}
			rep, err := audit.VerifyDir(dir)
			if err != nil {
				return err
			}
			for f, head := range rep.Head {
				fmt.Fprintf(out, "ok\t%s\thead=%s\n", f, head)
			}
			fmt.Fprintf(out, "verified %d files, %d records\n", rep.Files, rep.Records)
			return nil
		},
	}
	verify.Flags().StringVar(&dir, "dir", "audit", "audit directory")
	cmd.AddCommand(verify)
	return cmd
}

func hashKeyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hash-key",
		Short: "Read a tenant API key from stdin and print the sha256 to put in tenants[].key_sha256",
		RunE: func(cmd *cobra.Command, _ []string) error {
			line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return fmt.Errorf("read key: %w", err)
			}
			key := strings.TrimSpace(line)
			if key == "" {
				return errors.New("empty key")
			}
			fmt.Fprintln(cmd.OutOrStdout(), config.HashKey(key))
			return nil
		},
	}
}

func healthcheckCmd() *cobra.Command {
	var url string
	cmd := &cobra.Command{
		Use:   "healthcheck",
		Short: "Exit 0 when the readiness endpoint answers 200 (for distroless container healthchecks)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
			if err != nil {
				return fmt.Errorf("build request: %w", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("health request: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("%w: status %d", errUnhealthy, resp.StatusCode)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&url, "url", "http://127.0.0.1:9090/readyz", "readiness URL")
	return cmd
}
