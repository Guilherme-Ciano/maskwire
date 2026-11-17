package main

import (
	"encoding/json/v2"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/Guilherme-Ciano/maskwire/internal/audit"
)

type auditFlags struct {
	dir, config, anchors, checkpoints, pubkey string
}

func (f *auditFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.dir, "dir", "audit", "audit directory")
	cmd.Flags().StringVar(&f.config, "config", "", "config file holding the master key (or MASKWIRE_CONFIG / env)")
	cmd.Flags().StringVar(&f.anchors, "anchors", "", "anchor file written by audit.anchor_file")
	cmd.Flags().StringVar(&f.checkpoints, "checkpoints", "", "signed checkpoint file written by audit checkpoint")
	cmd.Flags().StringVar(&f.pubkey, "pubkey", "", "Ed25519 public key that signed the checkpoints")
}

func (f *auditFlags) load() (*audit.Sealer, audit.Anchors, error) {
	sealer, err := auditSealer(f.config)
	if err != nil {
		return nil, nil, err
	}
	anchors := audit.Anchors{}
	if f.anchors != "" {
		a, err := audit.ReadAnchors(f.anchors)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range a {
			anchors[k] = append(anchors[k], v...)
		}
	}
	if f.checkpoints != "" {
		if f.pubkey == "" {
			return nil, nil, errNeedPubkey
		}
		pub, err := audit.LoadPublicKey(f.pubkey)
		if err != nil {
			return nil, nil, err
		}
		c, _, err := audit.ReadCheckpoints(f.checkpoints, pub)
		if err != nil {
			return nil, nil, err
		}
		for k, v := range c {
			anchors[k] = append(anchors[k], v...)
		}
	}
	return sealer, anchors, nil
}

func printf(w io.Writer, format string, a ...any) error {
	if _, err := fmt.Fprintf(w, format, a...); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func auditCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "audit", Short: "Audit log tools"}
	cmd.AddCommand(auditVerifyCmd(), auditReportCmd(), auditKeygenCmd(), auditCheckpointCmd())
	return cmd
}

func auditVerifyCmd() *cobra.Command {
	var f auditFlags
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify the HMAC chain of every tenant and day file, plus anchors and signed checkpoints if given",
		RunE: func(cmd *cobra.Command, _ []string) error {
			sealer, anchors, err := f.load()
			if err != nil {
				return err
			}
			rep, err := audit.VerifyDir(f.dir, sealer, anchors)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for file, head := range rep.Head {
				if err := printf(out, "ok\t%s\thead=%s\n", file, head); err != nil {
					return err
				}
			}
			return printf(out, "verified %d files, %d records\n", rep.Files, rep.Records)
		},
	}
	f.bind(cmd)
	return cmd
}

func auditReportCmd() *cobra.Command {
	var f auditFlags
	var from, to, format string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Verify the log, then summarise recipients, data categories and safeguards per tenant for the DPO",
		RunE: func(cmd *cobra.Command, _ []string) error {
			sealer, anchors, err := f.load()
			if err != nil {
				return err
			}
			sum, err := audit.Summarize(f.dir, sealer, anchors, from, to)
			if err != nil {
				return err
			}
			if format == "json" {
				b, err := json.Marshal(sum, json.Deterministic(true))
				if err != nil {
					return fmt.Errorf("encode report: %w", err)
				}
				return printf(cmd.OutOrStdout(), "%s\n", b)
			}
			return printf(cmd.OutOrStdout(), "%s", sum.Markdown())
		},
	}
	f.bind(cmd)
	cmd.Flags().StringVar(&from, "from", "", "first UTC day to include (YYYY-MM-DD)")
	cmd.Flags().StringVar(&to, "to", "", "last UTC day to include (YYYY-MM-DD)")
	cmd.Flags().StringVar(&format, "format", "markdown", "markdown or json")
	return cmd
}

func auditKeygenCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "keygen",
		Short: "Create an Ed25519 key pair for signing audit checkpoints",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := os.MkdirAll(out, 0o700); err != nil {
				return fmt.Errorf("create key dir: %w", err)
			}
			priv, pub, err := audit.GenerateKey(out)
			if err != nil {
				return err
			}
			return printf(
				cmd.OutOrStdout(),
				"private key: %s (keep it off the audit host)\npublic key: %s\n",
				priv,
				pub,
			)
		},
	}
	cmd.Flags().StringVar(&out, "out", ".", "directory for audit-ed25519.key and audit-ed25519.pub")
	return cmd
}

func auditCheckpointCmd() *cobra.Command {
	var f auditFlags
	var key, out string
	cmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Verify the log and append a signed checkpoint of every file head",
		RunE: func(cmd *cobra.Command, _ []string) error {
			sealer, err := auditSealer(f.config)
			if err != nil {
				return err
			}
			priv, err := audit.LoadPrivateKey(key)
			if err != nil {
				return err
			}
			line, err := audit.Checkpoint(f.dir, sealer, priv, time.Now())
			if err != nil {
				return err
			}
			fh, err := os.OpenFile(filepath.Clean(out), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
			if err != nil {
				return fmt.Errorf("open checkpoint file: %w", err)
			}
			if _, err := fh.Write(line); err != nil {
				return fmt.Errorf("write checkpoint: %w", err)
			}
			if err := fh.Close(); err != nil {
				return fmt.Errorf("close checkpoint file: %w", err)
			}
			return printf(cmd.OutOrStdout(), "checkpoint appended to %s\n", out)
		},
	}
	cmd.Flags().StringVar(&f.dir, "dir", "audit", "audit directory")
	cmd.Flags().StringVar(&f.config, "config", "", "config file holding the master key (or MASKWIRE_CONFIG / env)")
	cmd.Flags().StringVar(&key, "key", "audit-ed25519.key", "Ed25519 private key")
	cmd.Flags().StringVar(&out, "out", "checkpoints.jsonl", "checkpoint file to append to")
	return cmd
}
