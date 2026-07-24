package store

import "chatdock/internal/chatdock/modelprovider"

func (s *Store) loadModelProviderRecordsLocked() ([]modelprovider.Record, error) {
	return modelprovider.LoadRecords(s.db)
}

func (s *Store) saveModelProviderRecordsLocked(records []modelprovider.Record) error {
	return modelprovider.SaveRecords(s.db, records)
}
