package store

import "errors"

var (
	// ErrCorruptedEOTSDb For some reason, db on disk representation have changed
	ErrCorruptedEOTSDb = errors.New("EOTS manager db is corrupted")

	// ErrDuplicateEOTSKeyName The EOTS key name we try to add already exists in db
	ErrDuplicateEOTSKeyName = errors.New("EOTS key name already exists")

	// ErrEOTSKeyNameNotFound The EOTS key name we try to fetch is not found in db
	ErrEOTSKeyNameNotFound = errors.New("EOTS key name not found")

	// ErrSignRecordNotFound sign record not found at given height
	ErrSignRecordNotFound = errors.New("sign record not found")

	// ErrDuplicateSignRecord indicates err if sign record is already saved at given height
	ErrDuplicateSignRecord = errors.New("sign record for given height already exists")
)
