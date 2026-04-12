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
	"flag"
	"fmt"
	"log/slog"
	"mezzo/internal/assets"
	"mezzo/internal/config"
	"mezzo/internal/server"
	"net/http"
	"os"
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

	if err := http.ListenAndServe(":"+cfg.Port, srv); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
