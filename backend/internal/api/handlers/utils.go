package handlers

import (
	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/validation"
)

func validateInstanceName(name string) error {
	return validation.ValidateInstanceName(name)
}

func instanceInConfig(cfg *config.Config, name string) bool {
	for _, inst := range cfg.Instances {
		if inst.Name == name {
			return true
		}
	}
	return false
}

func instanceType(cfg *config.Config, name string, want string) bool {
	for _, inst := range cfg.Instances {
		if inst.Name == name {
			return inst.Type == want
		}
	}
	return false
}
