package recall

import (
	"fmt"

	rtutils "github.com/OctoSucker/agent/utils"
)

// Must match store/tables.go migrate (TableRecallChunks).
const sqliteTableRecallChunks = "recall_chunks"

func (m *RecallCorpus) loadAll() error {
	rows, err := m.db.Query(fmt.Sprintf(`SELECT text, embedding FROM %s ORDER BY id ASC`, sqliteTableRecallChunks))
	if err != nil {
		return err
	}
	defer rows.Close()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.texts = m.texts[:0]
	m.embeddings = m.embeddings[:0]
	for rows.Next() {
		var text string
		var emb []byte
		if err := rows.Scan(&text, &emb); err != nil {
			return err
		}
		m.texts = append(m.texts, text)
		m.embeddings = append(m.embeddings, rtutils.BlobToFloat32Slice(emb))
	}
	return rows.Err()
}

func (m *RecallCorpus) persistInsert(text string, vec []float32) error {
	if m.db == nil {
		return nil
	}
	var blob interface{}
	if len(vec) > 0 {
		blob = rtutils.Float32SliceToBlob(vec)
	}
	if _, err := m.db.Exec(fmt.Sprintf(`INSERT INTO %s (text, embedding) VALUES (?, ?)`, sqliteTableRecallChunks), text, blob); err != nil {
		return err
	}
	return nil
}
