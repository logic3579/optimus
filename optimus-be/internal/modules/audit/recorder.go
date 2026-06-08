package audit

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	"optimus-be/internal/models"
)

// Event is the in-memory shape callers build before persisting.
// Payload may be any JSON-serializable value; nil → empty object.
type Event struct {
	UserID     *uint64
	Action     string
	TargetType string
	TargetID   string
	Payload    any
	IP         string
	UserAgent  string
}

type Recorder struct {
	db *gorm.DB
}

func NewRecorder(db *gorm.DB) *Recorder { return &Recorder{db: db} }

// Record inserts a single audit row. Returns the underlying DB error; callers
// usually log-and-ignore rather than propagate, so that an audit-write failure
// doesn't break the user-visible operation.
func (r *Recorder) Record(ctx context.Context, e Event) error {
	payload := []byte("{}")
	if e.Payload != nil {
		b, err := json.Marshal(e.Payload)
		if err != nil {
			return err
		}
		payload = b
	}
	return r.db.WithContext(ctx).Create(&models.AuditLog{
		UserID:     e.UserID,
		Action:     e.Action,
		TargetType: e.TargetType,
		TargetID:   e.TargetID,
		Payload:    payload,
		IP:         e.IP,
		UserAgent:  e.UserAgent,
	}).Error
}

// WithTx returns a Recorder bound to the given transaction.
func (r *Recorder) WithTx(tx *gorm.DB) *Recorder { return &Recorder{db: tx} }
