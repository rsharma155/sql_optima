package application

import (
	"context"
	"testing"
	"time"

	"github.com/rsharma155/sql_optima/internal/collectors/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockMSSQLRepo struct {
	mock.Mock
}

func (m *MockMSSQLRepo) FetchSnapshot(ctx context.Context, lastWatermark time.Time) ([]domain.MSSQLQuerySnapshot, error) {
	args := m.Called(ctx, lastWatermark)
	return args.Get(0).([]domain.MSSQLQuerySnapshot), args.Error(1)
}

func (m *MockMSSQLRepo) FetchSessionEnrichment(ctx context.Context) ([]domain.MSSQLSessionEnrichment, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.MSSQLSessionEnrichment), args.Error(1)
}

func (m *MockMSSQLRepo) GetSqlServerStartTime(ctx context.Context) (time.Time, error) {
	args := m.Called(ctx)
	return args.Get(0).(time.Time), args.Error(1)
}

type MockPGRepo struct {
	mock.Mock
}

func (m *MockPGRepo) FetchSnapshot(ctx context.Context) ([]domain.PGQuerySnapshot, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.PGQuerySnapshot), args.Error(1)
}

func (m *MockPGRepo) FetchActivityEnrichment(ctx context.Context) ([]domain.PGActivityEnrichment, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.PGActivityEnrichment), args.Error(1)
}

type MockWriter struct {
	mock.Mock
}

func (m *MockWriter) WriteMSSQLMetrics(ctx context.Context, metrics []domain.MSSQLCombinedMetric) error {
	args := m.Called(ctx, metrics)
	return args.Error(0)
}

func (m *MockWriter) WritePGMetrics(ctx context.Context, metrics []domain.PGCombinedMetric) error {
	args := m.Called(ctx, metrics)
	return args.Error(0)
}

func (m *MockWriter) GetInstanceState(ctx context.Context, instanceID string) (time.Time, time.Time, error) {
	args := m.Called(ctx, instanceID)
	return args.Get(0).(time.Time), args.Get(1).(time.Time), args.Error(2)
}

func (m *MockWriter) SaveMetrics(ctx context.Context, instanceID string, snapshots []domain.MSSQLQuerySnapshot, pollTime time.Time, sqlStartTime time.Time) error {
	args := m.Called(ctx, instanceID, snapshots, pollTime, sqlStartTime)
	return args.Error(0)
}

func TestCollectMSSQL_Workflow(t *testing.T) {
	app := NewCollectorApp(nil, nil, nil, nil, nil)
	instanceID := "test-instance"

	now := time.Now().UTC()
	startTime := now.Add(-24 * time.Hour)
	lastPoll := now.Add(-1 * time.Minute)

	snapshots := []domain.MSSQLQuerySnapshot{
		{DBID: 5, DatabaseName: "testdb", TotalExecutions: 100, LastExecutionTime: now.Add(-10 * time.Second)},
	}

	mockMSSQL := new(MockMSSQLRepo)
	mockMSSQL.On("GetSqlServerStartTime", mock.Anything).Return(startTime, nil)
	mockMSSQL.On("FetchSnapshot", mock.Anything, lastPoll).Return(snapshots, nil)

	mockWriter := new(MockWriter)
	mockWriter.On("GetInstanceState", mock.Anything, instanceID).Return(lastPoll, startTime, nil)
	mockWriter.On("SaveMetrics", mock.Anything, instanceID, snapshots, mock.Anything, startTime).Return(nil)

	app.mssqlRepo = mockMSSQL
	app.writer = mockWriter

	app.collectMSSQL(context.Background(), instanceID)

	mockMSSQL.AssertExpectations(t)
	mockWriter.AssertExpectations(t)
}

func TestCollectMSSQL_RestartDetection(t *testing.T) {
	app := NewCollectorApp(nil, nil, nil, nil, nil)
	instanceID := "test-instance"

	now := time.Now().UTC()
	prevStartTime := now.Add(-24 * time.Hour)
	currStartTime := now.Add(-1 * time.Hour) // Restarted
	lastPoll := now.Add(-1 * time.Minute)

	snapshots := []domain.MSSQLQuerySnapshot{
		{DBID: 5, DatabaseName: "testdb", TotalExecutions: 50, LastExecutionTime: now.Add(-10 * time.Second)},
	}

	mockMSSQL := new(MockMSSQLRepo)
	mockMSSQL.On("GetSqlServerStartTime", mock.Anything).Return(currStartTime, nil)
	// Watermark should be reset (zero time) because restart was detected
	mockMSSQL.On("FetchSnapshot", mock.Anything, time.Time{}).Return(snapshots, nil)

	mockWriter := new(MockWriter)
	mockWriter.On("GetInstanceState", mock.Anything, instanceID).Return(lastPoll, prevStartTime, nil)
	mockWriter.On("SaveMetrics", mock.Anything, instanceID, snapshots, mock.Anything, currStartTime).Return(nil)

	app.mssqlRepo = mockMSSQL
	app.writer = mockWriter

	app.collectMSSQL(context.Background(), instanceID)

	mockMSSQL.AssertExpectations(t)
	mockWriter.AssertExpectations(t)
}

func TestCollectPG_DeltaResetHandling(t *testing.T) {
	app := NewCollectorApp(nil, nil, nil, nil, nil)
	instanceID := "test-instance"

	// Initial collection
	snap1 := []domain.PGQuerySnapshot{
		{QueryID: 1, Calls: 100},
	}
	mockPG := new(MockPGRepo)
	mockPG.On("FetchSnapshot", mock.Anything).Return(snap1, nil).Once()
	app.pgRepo = mockPG

	app.collectPG(context.Background(), instanceID)
	assert.Equal(t, int64(100), app.pgPrevCounters[instanceID][1].Calls)

	// Second collection - delta
	snap2 := []domain.PGQuerySnapshot{
		{QueryID: 1, Calls: 110},
	}
	mockPG.On("FetchSnapshot", mock.Anything).Return(snap2, nil).Once()
	mockPG.On("FetchActivityEnrichment", mock.Anything).Return([]domain.PGActivityEnrichment{}, nil)

	mockWriter := new(MockWriter)
	mockWriter.On("WritePGMetrics", mock.Anything, mock.MatchedBy(func(m []domain.PGCombinedMetric) bool {
		return len(m) == 1 && m[0].Calls == 10
	})).Return(nil)
	app.writer = mockWriter

	app.collectPG(context.Background(), instanceID)
}
