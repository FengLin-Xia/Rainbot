package runtime

import (
	"encoding/json"
	"fmt"
	"time"

	bbolt "go.etcd.io/bbolt"

	"github.com/xia-rain/go_agent/internal/llm"
	"github.com/xia-rain/go_agent/internal/memory"
)

var sessionsBucket = []byte("sessions")

// sessionSnapshot is the serializable form of a Session.
type sessionSnapshot struct {
	ID        string        `json:"id"`
	CreatedAt time.Time     `json:"created_at"`
	History   []llm.Message `json:"history"`
	Summary   string        `json:"summary"`
}

// BoltPersister stores session snapshots in a single bbolt file.
type BoltPersister struct {
	db *bbolt.DB
}

func NewBoltPersister(path string) (*BoltPersister, error) {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bolt db: %w", err)
	}
	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(sessionsBucket)
		return err
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("init sessions bucket: %w", err)
	}
	return &BoltPersister{db: db}, nil
}

func (p *BoltPersister) save(snap sessionSnapshot) error {
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return p.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(sessionsBucket).Put([]byte(snap.ID), data)
	})
}

func (p *BoltPersister) loadAll() ([]sessionSnapshot, error) {
	var snaps []sessionSnapshot
	err := p.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(sessionsBucket).ForEach(func(_, v []byte) error {
			var s sessionSnapshot
			if err := json.Unmarshal(v, &s); err != nil {
				return err
			}
			snaps = append(snaps, s)
			return nil
		})
	})
	return snaps, err
}

func (p *BoltPersister) delete(id string) error {
	return p.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(sessionsBucket).Delete([]byte(id))
	})
}

func (p *BoltPersister) Close() error { return p.db.Close() }

// sessionToSnapshot converts a live Session to its serializable form.
// Each sub-component handles its own locking.
func sessionToSnapshot(sess *Session) sessionSnapshot {
	return sessionSnapshot{
		ID:        sess.ID,
		CreatedAt: sess.CreatedAt,
		History:   sess.history.All(),
		Summary:   sess.summary.Get(),
	}
}

// sessionFromSnapshot reconstructs a Session from a stored snapshot.
func sessionFromSnapshot(snap sessionSnapshot) *Session {
	sess := &Session{
		ID:        snap.ID,
		CreatedAt: snap.CreatedAt,
		history:   memory.NewShortTerm(50),
		summary:   memory.NewSummaryMemory(),
	}
	for _, m := range snap.History {
		sess.history.Append(m)
	}
	sess.summary.Set(snap.Summary)
	return sess
}
