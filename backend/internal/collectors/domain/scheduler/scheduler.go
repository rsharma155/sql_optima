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

func (s *JobSchedulerService) GetJobsToRun(ctx context.Context) ([]Config, error) {
	configs, err := s.repo.GetActiveConfigs(ctx)
	if err != nil {
		return nil, err
	}

	var jobsToRun []Config
	now := time.Now()
	for _, cfg := range configs {
		lastRun, ok := s.lastRunMap[cfg.Name]
		if !ok || now.Sub(lastRun) >= time.Duration(cfg.FrequencySeconds)*time.Second {
			jobsToRun = append(jobsToRun, cfg)
		}
	}
	return jobsToRun, nil
}

func (s *JobSchedulerService) MarkAsRun(name string) {
	s.lastRunMap[name] = time.Now()
}
