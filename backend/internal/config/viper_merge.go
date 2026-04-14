// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Viper-based config merging for optional server.yaml, security.yaml, collectors.yaml with environment variable binding.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package config

import (
	"strings"

	"github.com/spf13/viper"
)

// MergeViperConfigs loads optional configs/server.yaml, configs/security.yaml, configs/collectors.yaml
// and binds environment variables (SQL_OPTIMA_ prefix). Safe to call when files are missing.
func MergeViperConfigs() error {
	v := viper.New()
	v.SetEnvPrefix("SQL_OPTIMA")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	v.AddConfigPath("configs")
	v.AddConfigPath("../configs")
	v.AddConfigPath(".")
	v.SetConfigType("yaml")

	for _, name := range []string{"server", "security", "collectors"} {
		v.SetConfigName(name)
		_ = v.MergeInConfig()
	}
	return nil
}
