package security_test

import (
	"testing"

	"github.com/google/uuid"
)

// DummyDevice represents an isolated tenant resource
type DummyDevice struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Hostname       string
}

// MockTenantRepository simulates tenant query isolation logic
type MockTenantRepository struct {
	devices []DummyDevice
}

func (r *MockTenantRepository) GetDevice(requesterOrgID, deviceID uuid.UUID) (*DummyDevice, error) {
	for _, d := range r.devices {
		// Strict tenant isolation guard
		if d.ID == deviceID && d.OrganizationID == requesterOrgID {
			return &d, nil
		}
	}
	return nil, nil // Not found or cross-tenant access denied
}

func TestCrossTenantDeviceAccessBlocked(t *testing.T) {
	orgA := uuid.New()
	orgB := uuid.New()

	deviceOrgB := DummyDevice{
		ID:             uuid.New(),
		OrganizationID: orgB,
		Hostname:       "CONFIDENTIAL-FINANCE-PC",
	}

	repo := &MockTenantRepository{
		devices: []DummyDevice{deviceOrgB},
	}

	// Organization A attempts to query Organization B's device
	res, err := repo.GetDevice(orgA, deviceOrgB.ID)
	if err != nil {
		t.Fatalf("Unexpected query error: %v", err)
	}

	if res != nil {
		t.Fatalf("SECURITY VIOLATION: Organization A was able to read Organization B device: %+v", res)
	}

	// Organization B queries its own device
	res, err = repo.GetDevice(orgB, deviceOrgB.ID)
	if err != nil {
		t.Fatalf("Unexpected query error: %v", err)
	}

	if res == nil || res.ID != deviceOrgB.ID {
		t.Fatalf("Expected Organization B to access its own device, got nil")
	}
}
