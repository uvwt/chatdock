package store

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
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
	rows, err := s.db.Query(`SELECT full_name, source_hash, embedding_model, embedding_json, embedding_blob FROM tool_embeddings WHERE embedding_model = ?`, model)
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
			if err := json.Unmarshal([]byte(raw), &item.Embedding); err != nil {
				return nil, fmt.Errorf("decode tool embedding %s: %w", item.FullName, err)
			}
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
	_, err = s.db.Exec(`INSERT INTO tool_embeddings(full_name, source_hash, embedding_model, embedding_json, embedding_blob, indexed_at) VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(full_name, embedding_model) DO UPDATE SET source_hash = excluded.source_hash, embedding_json = excluded.embedding_json, embedding_blob = excluded.embedding_blob, indexed_at = excluded.indexed_at`, item.FullName, item.SourceHash, item.EmbeddingModel, string(raw), blob, formatDBTime(time.Now()))
	return err
}

func deleteToolEmbeddingsWith(writer sqlWriter) error {
	_, err := writer.Exec(`DELETE FROM tool_embeddings`)
	return err
}

func encodeEmbeddingBlob(values []float64) []byte {
	if len(values) == 0 {
		return nil
	}
	blob := make([]byte, len(values)*4)
	for index, value := range values {
		binary.LittleEndian.PutUint32(blob[index*4:], math.Float32bits(float32(value)))
	}
	return blob
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
