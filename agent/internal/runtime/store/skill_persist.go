package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	rtutils "github.com/OctoSucker/agent/utils"
	"github.com/OctoSucker/agent/pkg/ports"
)

func (r *SkillRegistry) loadSkillsFromDB() error {
	if r.db == nil {
		return nil
	}
	rows, err := r.db.Query(`SELECT name, keywords_json, caps_json, path_json, embedding, attempts, successes, last_used_unix FROM skills ORDER BY name`)
	if err != nil {
		return fmt.Errorf("skills load: %w", err)
	}
	defer rows.Close()
	byName := make(map[string]*SkillEntry)
	var order []string
	for rows.Next() {
		var name, kwj, capsj, pathj string
		var emb []byte
		var att, suc int
		var lu int64
		if err := rows.Scan(&name, &kwj, &capsj, &pathj, &emb, &att, &suc, &lu); err != nil {
			return err
		}
		var kw, caps, path []string
		_ = json.Unmarshal([]byte(kwj), &kw)
		_ = json.Unmarshal([]byte(capsj), &caps)
		_ = json.Unmarshal([]byte(pathj), &path)
		e := &SkillEntry{
			Name:             name,
			Keywords:         kw,
			Capabilities:     caps,
			Path:             path,
			TriggerEmbedding: rtutils.BlobToFloat32Slice(emb),
			Variants:         nil,
			Attempts:         att,
			Successes:        suc,
			LastUsedAt:       time.Unix(lu, 0),
		}
		byName[name] = e
		order = append(order, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	vrows, err := r.db.Query(`SELECT skill_name, variant_id, plan_json, attempts, successes FROM skill_variants ORDER BY skill_name, variant_id`)
	if err != nil {
		return fmt.Errorf("skill_variants load: %w", err)
	}
	defer vrows.Close()
	for vrows.Next() {
		var sn, vid, planj string
		var va, vs int
		if err := vrows.Scan(&sn, &vid, &planj, &va, &vs); err != nil {
			return err
		}
		e := byName[sn]
		if e == nil {
			continue
		}
		var plan ports.Plan
		if err := json.Unmarshal([]byte(planj), &plan); err != nil {
			continue
		}
		pl := plan
		e.Variants = append(e.Variants, SkillPlanVariant{ID: vid, Plan: &pl, Attempts: va, Successes: vs})
	}
	if err := vrows.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = make([]SkillEntry, 0, len(order))
	for _, name := range order {
		if e := byName[name]; e != nil {
			r.entries = append(r.entries, *e)
		}
	}
	return nil
}

// persistSkillsDBLocked replaces persisted skills; caller must hold r.mu (write lock).
func (r *SkillRegistry) persistSkillsDBLocked() {
	if r.db == nil {
		return
	}
	tx, err := r.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM skill_variants`); err != nil {
		return
	}
	if _, err := tx.Exec(`DELETE FROM skills`); err != nil {
		return
	}
	for i := range r.entries {
		if err := insertSkillTx(tx, r.entries[i]); err != nil {
			return
		}
	}
	_ = tx.Commit()
}

func insertSkillTx(tx *sql.Tx, e SkillEntry) error {
	kwj, err := json.Marshal(e.Keywords)
	if err != nil {
		return err
	}
	capsj, err := json.Marshal(e.Capabilities)
	if err != nil {
		return err
	}
	pathj, err := json.Marshal(e.Path)
	if err != nil {
		return err
	}
	var emb interface{}
	if len(e.TriggerEmbedding) > 0 {
		emb = rtutils.Float32SliceToBlob(e.TriggerEmbedding)
	}
	lu := e.LastUsedAt.Unix()
	if e.LastUsedAt.IsZero() {
		lu = 0
	}
	_, err = tx.Exec(`INSERT INTO skills (name, keywords_json, caps_json, path_json, embedding, attempts, successes, last_used_unix) VALUES (?,?,?,?,?,?,?,?)`,
		e.Name, string(kwj), string(capsj), string(pathj), emb, e.Attempts, e.Successes, lu)
	if err != nil {
		return err
	}
	for _, v := range e.Variants {
		if v.ID == "" || v.Plan == nil {
			continue
		}
		planj, err := json.Marshal(v.Plan)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO skill_variants (skill_name, variant_id, plan_json, attempts, successes) VALUES (?,?,?,?,?)`,
			e.Name, v.ID, string(planj), v.Attempts, v.Successes); err != nil {
			return err
		}
	}
	return nil
}
