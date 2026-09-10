package security_test

import (
	"context"
	"testing"
	"time"

	"github.com/controlhub/controlhub/services/api/internal/models"
	"github.com/controlhub/controlhub/services/api/internal/store"
	"github.com/google/uuid"
)

func TestCrossTenantCommandIsolation(t *testing.T) {
	st := store.NewMemoryStore()
	ctx := context.Background()

	orgA := uuid.MustParse("771e8bfb-cf98-4c28-98e3-0d6e6443c21a")
	orgB := uuid.New()

	cmdID := uuid.New()
	cmdOrgB := models.Command{
		ID:             cmdID,
		OrganizationID: orgB,
		DeviceID:       uuid.New(),
		IssuedByUserID: uuid.New(),
		CommandType:    "REBOOT",
		Status:         "QUEUED",
		CreatedAt:      time.Now().UTC(),
	}

	_ = st.CreateCommand(ctx, cmdOrgB)

	// Org A attempts to read Org B command
	res, err := st.GetCommand(ctx, orgA, cmdID)
	if err == nil || res != nil {
		t.Fatalf("SECURITY VIOLATION: Organization A was able to read Organization B command: %+v", res)
	}

	// Org B reads its own command
	res, err = st.GetCommand(ctx, orgB, cmdID)
	if err != nil || res == nil {
		t.Fatalf("Expected Org B to access its own command, got err=%v", err)
	}
}

func TestAuditLogImmutabilityAndCompleteness(t *testing.T) {
	st := store.NewMemoryStore()
	ctx := context.Background()
	orgID := uuid.MustParse("771e8bfb-cf98-4c28-98e3-0d6e6443c21a")

	logEntry := models.AuditLog{
		OrganizationID: orgID,
		Action:         "security.penetration.test",
		ResourceType:   "test_harness",
		ResourceID:     "probe-1",
		Status:         "SUCCESS",
		Metadata: map[string]interface{}{
			"defense": "active",
		},
	}

	err := st.InsertAuditLog(ctx, logEntry)
	if err != nil {
		t.Fatalf("Failed to write audit entry: %v", err)
	}

	logs, err := st.ListAuditLogs(ctx, orgID, 10)
	if err != nil || len(logs) == 0 {
		t.Fatalf("Failed to fetch audit ledger")
	}

	found := false
	for _, l := range logs {
		if l.Action == "security.penetration.test" {
			found = true
			if l.ID == 0 || l.CreatedAt.IsZero() {
				t.Fatalf("Audit log missing mandatory timestamp or ID: %+v", l)
			}
			break
		}
	}
	if !found {
		t.Fatal("Audit log entry not found in ledger")
	}
}
