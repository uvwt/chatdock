package store

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"strings"
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
	rows, err := s.db.Query(`SELECT full_name, source_hash, embedding_model, embedding_json, embedding_blob FROM tool_embeddings WHERE workspace_id = ? AND embedding_model = ?`, s.activeWorkspace, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]ToolEmbeddingRecord{}
	for rows.Next() {
		var item ToolEmbeddingRecord
		var raw string
		var blob []byte
		if err := rows.Scan(&item.FullName, &item.SourceHash, &item.EmbeddingModel, &raw, &blob); err != nil {
			return nil, err
		}
		if len(blob) > 0 {
			item.Embedding = decodeEmbeddingBlob(blob)
		}
		if len(item.Embedding) == 0 && strings.TrimSpace(raw) != "" {
			_ = json.Unmarshal([]byte(raw), &item.Embedding)
		}
		if len(item.Embedding) == 0 {
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
	blob := encodeEmbeddingBlob(item.Embedding)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.db.Exec(`INSERT INTO tool_embeddings(workspace_id, full_name, source_hash, embedding_model, embedding_json, embedding_blob, indexed_at) VALUES(?, ?, ?, ?, ?, ?, ?) ON CONFLICT(workspace_id, full_name, embedding_model) DO UPDATE SET source_hash = excluded.source_hash, embedding_json = excluded.embedding_json, embedding_blob = excluded.embedding_blob, indexed_at = excluded.indexed_at`, s.activeWorkspace, item.FullName, item.SourceHash, item.EmbeddingModel, string(raw), blob, formatDBTime(time.Now()))
	return err
}

func (s *Store) deleteToolEmbeddingsForWorkspaceLocked(prompt string) error {
	_, err := s.db.Exec(`DELETE FROM tool_embeddings WHERE workspace_id = ?`, prompt)
	return err
}

func (s *Store) migrateToolEmbeddingBlobs() error {
	migrated, err := s.metaValue("tool_embedding_blobs_migrated")
	if err != nil {
		return err
	}
	if migrated == "1" {
		return nil
	}
	rows, err := s.db.Query(`SELECT workspace_id, full_name, embedding_model, embedding_json FROM tool_embeddings WHERE length(embedding_blob) = 0`)
	if err != nil {
		return err
	}
	type item struct{ prompt, fullName, model, raw string }
	items := []item{}
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.prompt, &it.fullName, &it.model, &it.raw); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, it)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, it := range items {
		var vector []float64
		if err := json.Unmarshal([]byte(it.raw), &vector); err != nil || len(vector) == 0 {
			continue
		}
		if _, err := s.db.Exec(`UPDATE tool_embeddings SET embedding_blob = ? WHERE workspace_id = ? AND full_name = ? AND embedding_model = ?`, encodeEmbeddingBlob(vector), it.prompt, it.fullName, it.model); err != nil {
			return err
		}
	}
	return s.setMetaValue("tool_embedding_blobs_migrated", "1")
}

func encodeEmbeddingBlob(values []float64) []byte {
	if len(values) == 0 {
		return nil
	}
	buf := bytes.NewBuffer(make([]byte, 0, len(values)*4))
	for _, value := range values {
		_ = binary.Write(buf, binary.LittleEndian, float32(value))
	}
	return buf.Bytes()
}

func decodeEmbeddingBlob(raw []byte) []float64 {
	if len(raw) == 0 || len(raw)%4 != 0 {
		return nil
	}
	out := make([]float64, 0, len(raw)/4)
	for offset := 0; offset < len(raw); offset += 4 {
		bits := binary.LittleEndian.Uint32(raw[offset : offset+4])
		out = append(out, float64(math.Float32frombits(bits)))
	}
	return out
}
