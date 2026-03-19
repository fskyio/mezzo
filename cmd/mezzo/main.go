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
	"log"
	"mezzo/internal/assets"
	"mezzo/internal/server"
	"net/http"
	"os"
)

var version = "dev"

func main() {
	port := flag.String("port", "8006", "Port to run on")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("%s\n", version)
		return
	}

	if envPort := os.Getenv("PORT"); envPort != "" {
		*port = envPort
	}

	srv, err := server.New(assets.Assets, version)

	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	log.Printf("Starting Mezzo on :%s", *port)

	if err := http.ListenAndServe(":"+*port, srv); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
