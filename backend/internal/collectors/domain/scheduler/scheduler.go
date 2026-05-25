package scheduler

import (
	"context"
	"time"
)

type Config struct {
	Name             string
	FrequencySeconds int
}

type JobSchedulerService struct {
	repo       ConfigRepository
	lastRunMap map[string]time.Time
}

type ConfigRepository interface {
	GetActiveConfigs(ctx context.Context) ([]Config, error)
}

func NewJobSchedulerService(repo ConfigRepository) *JobSchedulerService {
	return &JobSchedulerService{
		repo:       repo,
		lastRunMap: make(map[string]time.Time),
	}
}

func jobSchedulerKey(name, scope string) string {
	if scope == "" {
		return name
	}
	return name + "|" + scope
}

// GetJobsToRun returns jobs due for the given scope (e.g. server UUID).
// Each monitored instance has its own last-run schedule for the same job name.
func (s *JobSchedulerService) GetJobsToRun(ctx context.Context, scope string) ([]Config, error) {
	configs, err := s.repo.GetActiveConfigs(ctx)
	if err != nil {
		return nil, err
	}

	var jobsToRun []Config
	now := time.Now()
	for _, cfg := range configs {
		lastRun, ok := s.lastRunMap[jobSchedulerKey(cfg.Name, scope)]
		if !ok || now.Sub(lastRun) >= time.Duration(cfg.FrequencySeconds)*time.Second {
			jobsToRun = append(jobsToRun, cfg)
		}
	}
	return jobsToRun, nil
}

func (s *JobSchedulerService) MarkAsRun(name, scope string) {
	s.lastRunMap[jobSchedulerKey(name, scope)] = time.Now()
}
