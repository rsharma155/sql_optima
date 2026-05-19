// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Health Intelligence Engine
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package config




import (
	"io"
	"log"
	"os"
	"time"
)

var Logger *log.Logger

func SetupLogging() {
	flags := log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile
	Logger = log.New(os.Stdout, "[SQLOPTIMA] ", flags)
	log.SetFlags(flags)
	log.SetOutput(os.Stdout)
}

func SetupFileLogging(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	mw := io.MultiWriter(os.Stdout, f)
	flags := log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile
	Logger = log.New(mw, "[SQLOPTIMA] ", flags)
	log.SetFlags(flags)
	log.SetOutput(mw)
	return nil
}

func Timestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}