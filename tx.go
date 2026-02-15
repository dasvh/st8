package st8

// View is a read-only "transaction". Uses a read lock to allow multiple concurrent views.
func (db *DB[T]) View(fn func(s T) error) error {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return fn(db.state)
}

// Update is a read-write "transaction". Uses a write lock to ensure exclusive access.
// If the provided function returns an error, changes are discarded and the error is returned.
func (db *DB[T]) Update(fn func(s *T) error) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	var clone T
	db.buf.Reset()
	defer db.buf.Reset()

	// todo: optimize this by implementing a Clone method on T and using that instead of serialize/deserialize.
	if err := db.serializer.Serialize(&db.buf, db.state); err != nil {
		return err
	}
	if err := db.serializer.Deserialize(&db.buf, &clone); err != nil {
		return err
	}

	if err := fn(&clone); err != nil {
		return err // "rollback"
	}

	if err := db.persist(clone); err != nil {
		return err
	}

	db.state = clone
	return nil
}
