// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Main entry point for the SQL Optima server application. Starts the dual-engine API and static server on configurable port (default 8080).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package main

import "github.com/rsharma155/sql_optima/internal/appserver"

func main() {
	appserver.Main()
}
