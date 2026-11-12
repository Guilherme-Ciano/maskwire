package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Guilherme-Ciano/maskwire/internal/mockllm"
)

var errProbe = errors.New("probe failed")

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("mockllm", flag.ContinueOnError)
	addr := fs.String("addr", ":9000", "listen address")
	chunkMin := fs.Int("chunk-min", 1, "minimum SSE chunk size in bytes")
	chunkMax := fs.Int("chunk-max", 7, "maximum SSE chunk size in bytes")
	chunkDelay := fs.Duration("chunk-delay", 0, "delay between SSE chunks")
	latency := fs.Duration("latency", 0, "fixed latency before answering")
	dim := fs.Int("dim", 1536, "embedding dimension")
	seed := fs.Uint64("seed", 1, "chunking seed")
	health := fs.String("healthcheck", "", "probe this URL and exit 0 on 200")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if *health != "" {
		return probe(*health)
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{
		Addr: *addr,
		Handler: mockllm.New(mockllm.Options{
			ChunkMin: *chunkMin, ChunkMax: *chunkMax, ChunkDelay: *chunkDelay,
			Latency: *latency, Dim: *dim, Seed: *seed, Log: log,
		}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		log.InfoContext(ctx, "mockllm listening", slog.String("addr", *addr))
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("listen: %w", err)
		}
		return nil
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

func probe(url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("build probe: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("probe: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", errProbe, resp.StatusCode)
	}
	return nil
}
