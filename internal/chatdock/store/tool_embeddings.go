package store

import (
	"encoding/json"
	"time"
)

type ToolEmbeddingRecord struct {
	FullName       string
	SourceHash     string
	EmbeddingModel string
	Embedding      []float64
}

func (s *Store) ToolEmbeddings(model string) (map[string]ToolEmbeddingRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT full_name, source_hash, embedding_model, embedding_json FROM tool_embeddings WHERE prompt = ? AND embedding_model = ?`, s.activePrompt, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ToolEmbeddingRecord{}
	for rows.Next() {
		var item ToolEmbeddingRecord
		var raw string
		if err := rows.Scan(&item.FullName, &item.SourceHash, &item.EmbeddingModel, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &item.Embedding); err != nil || len(item.Embedding) == 0 {
			continue
		}
		out[item.FullName] = item
	}
	return out, rows.Err()
}

func (s *Store) SaveToolEmbedding(item ToolEmbeddingRecord) error {
	raw, err := json.Marshal(item.Embedding)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(`INSERT INTO tool_embeddings(prompt, full_name, source_hash, embedding_model, embedding_json, indexed_at) VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(prompt, full_name, embedding_model) DO UPDATE SET source_hash = excluded.source_hash, embedding_json = excluded.embedding_json, indexed_at = excluded.indexed_at`, s.activePrompt, item.FullName, item.SourceHash, item.EmbeddingModel, string(raw), formatDBTime(time.Now()))
	return err
}

func (s *Store) deleteToolEmbeddingsForPromptLocked(prompt string) error {
	_, err := s.db.Exec(`DELETE FROM tool_embeddings WHERE prompt = ?`, prompt)
	return err
}
