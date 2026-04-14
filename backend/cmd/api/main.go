// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Entry point for the API server component that handles all HTTP requests. Wires the application server with middleware, repositories, and service layers for the monitoring dashboard.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package main

import "github.com/rsharma155/sql_optima/internal/appserver"

func main() {
	appserver.Main()
}
