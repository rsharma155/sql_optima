package hot

import "testing"

func TestComputePgHealthScore_StatusBands(t *testing.T) {
	score, status := ComputePgHealthScore(PgHealthInputs{
		ReplicationLagSeconds: 0,
		XIDWraparoundPct:      1,
		DeadTupleRatioPct:     1,
		CheckpointReqRatio:    0.1,
		WALRateMBPerMin:       10,
		BlockingSessions:      0,
	})
	if score < 90 || status != "Healthy" {
		t.Fatalf("expected Healthy >=90, got %d %s", score, status)
	}

	score2, status2 := ComputePgHealthScore(PgHealthInputs{
		ReplicationLagSeconds: 30,
		XIDWraparoundPct:      60,
		DeadTupleRatioPct:     10,
		CheckpointReqRatio:    1.0,
		WALRateMBPerMin:       200,
		BlockingSessions:      1,
	})
	if !(score2 >= 70 && score2 <= 89) || status2 != "Watch" {
		t.Fatalf("expected Watch 70-89, got %d %s", score2, status2)
	}

	score3, status3 := ComputePgHealthScore(PgHealthInputs{
		ReplicationLagSeconds: 120,
		XIDWraparoundPct:      95,
		DeadTupleRatioPct:     50,
		CheckpointReqRatio:    2.0,
		WALRateMBPerMin:       1000,
		BlockingSessions:      10,
	})
	if score3 >= 70 || status3 != "At Risk" {
		t.Fatalf("expected At Risk <70, got %d %s", score3, status3)
	}
}

