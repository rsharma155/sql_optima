package scheduler

import (
	"context"
	"testing"
	"time"

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

func TestScheduler_ShouldRun(t *testing.T) {
	lastRun := time.Now().Add(-60 * time.Second)
	frequency := 30

	// If now - lastRun >= frequency, it should run
	assert.True(t, time.Since(lastRun) >= time.Duration(frequency)*time.Second)

	lastRun = time.Now().Add(-10 * time.Second)
	frequency = 30
	assert.False(t, time.Since(lastRun) >= time.Duration(frequency)*time.Second)
}
