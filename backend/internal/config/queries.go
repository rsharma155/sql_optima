package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type QueriesYaml struct {
	CollectorName string     `yaml:"collector_name"`
	Metrics       []QueryDef `yaml:"metrics"`
}

type QueryDef struct {
	MetricName string   `yaml:"metric_name"`
	Type       string   `yaml:"type"`
	Help       string   `yaml:"help"`
	Query      string   `yaml:"query"`
	Values     []string `yaml:"values"`
	KeyLabels  []string `yaml:"key_labels"`
}

var GlobalQueries *QueriesYaml

// LoadQueries safely maps Prometheus syntax natively abstracting all execution models
func LoadQueries(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var q QueriesYaml
	if err := yaml.Unmarshal(data, &q); err != nil {
		return err
	}

	GlobalQueries = &q
	return nil
}
