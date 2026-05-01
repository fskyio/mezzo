/*
   Copyright (C) 2026 FSKY <development@fsky.io>

   This file is part of Mezzo

   Mezzo is free software: you can redistribute it and/or modify it under the
   terms of the GNU Affero General Public License as published by the Free
   Software Foundation, either version 3 of the License, or (at your option) any
   later version.

   This program is distributed in the hope that it will be useful, but WITHOUT
   ANY WARRANTY; without even the implied warranty of  MERCHANTABILITY or
   FITNESS FOR A PARTICULAR PURPOSE.  See the GNU Affero General Public License
   for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"mezzo/internal/assets"
	"mezzo/internal/config"
	"mezzo/internal/server"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "Print version and exit")
	verbose := flag.Bool("verbose", false, "Enable debug logging")
	flag.Parse()

	if *showVersion {
		fmt.Printf("%s\n", version)
		return
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	cfg := config.Load()

	srv, err := server.New(assets.Assets, version, cfg)
	if err != nil {
		slog.Error("failed to initialize server", "error", err)
		os.Exit(1)
	}

	slog.Info("starting mezzo", "port", cfg.Port)

	httpSrv := &http.Server{
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ln4, err4 := net.Listen("tcp4", ":"+cfg.Port)
	ln6, err6 := net.Listen("tcp6", ":"+cfg.Port)
	if err4 != nil && err6 != nil {
		slog.Error("failed to listen on any address", "ipv4_error", err4, "ipv6_error", err6)
		os.Exit(1)
	}

	var wg sync.WaitGroup
	listen := func(ln net.Listener) {
		defer wg.Done()
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listener error", "error", err)
		}
	}
	if err4 == nil {
		wg.Add(1)
		go listen(ln4)
	}
	if err6 == nil {
		wg.Add(1)
		go listen(ln6)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		slog.Info("shutting down mezzo")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to shut down gracefully", "error", err)
		}
	}()

	wg.Wait()
}
