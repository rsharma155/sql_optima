package scheduler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockConfigRepo struct {
	mock.Mock
}

func (m *MockConfigRepo) GetActiveConfigs(ctx context.Context) ([]Config, error) {
	args := m.Called(ctx)
	return args.Get(0).([]Config), args.Error(1)
}

func TestJobSchedulerService_GetJobsToRun(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockConfigRepo)
	scheduler := NewJobSchedulerService(mockRepo)

	configs := []Config{
		{Name: "Job1", FrequencySeconds: 30},
		{Name: "Job2", FrequencySeconds: 60},
	}

	mockRepo.On("GetActiveConfigs", ctx).Return(configs, nil).Once()

	// 1. Initial run: all jobs should run
	jobs, err := scheduler.GetJobsToRun(ctx, "server-a")
	assert.NoError(t, err)
	assert.Len(t, jobs, 2)
	assert.Equal(t, "Job1", jobs[0].Name)
	assert.Equal(t, "Job2", jobs[1].Name)

	// Mark Job1 as run for server-a only
	scheduler.MarkAsRun("Job1", "server-a")

	// 2. Immediate second check: Job1 should NOT run, Job2 SHOULD run (same scope)
	mockRepo.On("GetActiveConfigs", ctx).Return(configs, nil).Once()
	jobs, err = scheduler.GetJobsToRun(ctx, "server-a")
	assert.NoError(t, err)
	assert.Len(t, jobs, 1)
	assert.Equal(t, "Job2", jobs[0].Name)

	// Job1 still due for a different instance
	mockRepo.On("GetActiveConfigs", ctx).Return(configs, nil).Once()
	jobs, err = scheduler.GetJobsToRun(ctx, "server-b")
	assert.NoError(t, err)
	assert.Len(t, jobs, 2)

	mockRepo.On("GetActiveConfigs", ctx).Return([]Config{}, assert.AnError).Once()
	_, err = scheduler.GetJobsToRun(ctx, "server-a")
	assert.Error(t, err)
}
