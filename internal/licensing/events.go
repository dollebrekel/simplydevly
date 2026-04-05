package licensing

import (
	"time"

	"siply.dev/siply/internal/core"
)

// LicenseChangedEventType is the event type string for license state changes.
const LicenseChangedEventType = "license.changed"

// LicenseChangedEvent is published when the user logs in or out.
type LicenseChangedEvent struct {
	Status    core.LicenseStatus
	OccuredAt time.Time
}

func (e LicenseChangedEvent) Type() string        { return LicenseChangedEventType }
func (e LicenseChangedEvent) Timestamp() time.Time { return e.OccuredAt }

// NewLicenseChangedEvent creates a LicenseChangedEvent with the current time.
func NewLicenseChangedEvent(status core.LicenseStatus) LicenseChangedEvent {
	return LicenseChangedEvent{
		Status:    status,
		OccuredAt: time.Now(),
	}
}
