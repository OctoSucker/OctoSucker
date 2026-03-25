package skill

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/OctoSucker/agent/pkg/ports"
	rtutils "github.com/OctoSucker/agent/utils"
)

func mustUnmarshalStringSlice(src []byte, field string) ([]string, error) {
	if len(strings.TrimSpace(string(src))) == 0 {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal(src, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	return out, nil
}

// Table names must stay aligned with store/tables.go migrate.
const (
	sqliteTableSkills             = "skills"
	sqliteTableSkillVariants      = "skill_variants"
	sqliteTableSkillLearnProgress = "skill_learn_progress"
)

func (r *SkillRegistry) loadSkillsFromDB() error {
	if r.db == nil {
		return nil
	}
	rows, err := r.db.Query(fmt.Sprintf(`SELECT name, keywords_json, caps_json, path_json, embedding, attempts, successes, last_used_unix FROM %s ORDER BY name`, sqliteTableSkills))
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
		kw, err := mustUnmarshalStringSlice([]byte(kwj), "keywords_json")
		if err != nil {
			return fmt.Errorf("skill %s: %w", name, err)
		}
		caps, err := mustUnmarshalStringSlice([]byte(capsj), "caps_json")
		if err != nil {
			return fmt.Errorf("skill %s: %w", name, err)
		}
		path, err := mustUnmarshalStringSlice([]byte(pathj), "path_json")
		if err != nil {
			return fmt.Errorf("skill %s: %w", name, err)
		}
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
	vrows, err := r.db.Query(fmt.Sprintf(`SELECT skill_name, variant_id, plan_json, params_json, attempts, successes, last_used_unix FROM %s ORDER BY skill_name, variant_id`, sqliteTableSkillVariants))
	if err != nil {
		return fmt.Errorf("skill_variants load: %w", err)
	}
	defer vrows.Close()
	for vrows.Next() {
		var sn, vid, planj, paramsj string
		var va, vs int
		var vlu int64
		if err := vrows.Scan(&sn, &vid, &planj, &paramsj, &va, &vs, &vlu); err != nil {
			return err
		}
		e := byName[sn]
		if e == nil {
			return fmt.Errorf("skill_variants: unknown skill_name %q for variant %q", sn, vid)
		}
		var plan ports.Plan
		if err := json.Unmarshal([]byte(planj), &plan); err != nil {
			return fmt.Errorf("skill %s variant %s plan_json: %w", sn, vid, err)
		}
		var params []SkillParamSpec
		if strings.TrimSpace(paramsj) != "" {
			if err := json.Unmarshal([]byte(paramsj), &params); err != nil {
				return fmt.Errorf("skill %s variant %s params_json: %w", sn, vid, err)
			}
		}
		pl := plan
		e.Variants = append(e.Variants, SkillPlanVariant{ID: vid, Plan: &pl, Params: params, Attempts: va, Successes: vs, LastUsedUnix: vlu})
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
func (r *SkillRegistry) persistSkillsDBLocked() error {
	if r.db == nil {
		return nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM %s`, sqliteTableSkillVariants)); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM %s`, sqliteTableSkills)); err != nil {
		return err
	}
	for i := range r.entries {
		if err := insertSkillTx(tx, r.entries[i]); err != nil {
			return err
		}
	}
	return tx.Commit()
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
	_, err = tx.Exec(fmt.Sprintf(`INSERT INTO %s (name, keywords_json, caps_json, path_json, embedding, attempts, successes, last_used_unix) VALUES (?,?,?,?,?,?,?,?)`, sqliteTableSkills),
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
		paramsj, err := json.Marshal(v.Params)
		if err != nil {
			return err
		}
		vlu := v.LastUsedUnix
		if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO %s (skill_name, variant_id, plan_json, params_json, attempts, successes, last_used_unix) VALUES (?,?,?,?,?,?,?)`, sqliteTableSkillVariants),
			e.Name, v.ID, string(planj), string(paramsj), v.Attempts, v.Successes, vlu); err != nil {
			return err
		}
	}
	return nil
}

// BumpSkillLearnSuccessCount increments the qualifying-success counter for this capability path and returns the new total.
func BumpSkillLearnSuccessCount(db *sql.DB, capKey string) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("skill.BumpSkillLearnSuccessCount: nil db")
	}
	now := time.Now().Unix()
	_, err := db.Exec(fmt.Sprintf(`
INSERT INTO %s (cap_key, success_count, last_success_unix) VALUES (?, 1, ?)
ON CONFLICT(cap_key) DO UPDATE SET
	success_count = success_count + 1,
	last_success_unix = excluded.last_success_unix
`, sqliteTableSkillLearnProgress), capKey, now)
	if err != nil {
		return 0, err
	}
	var n int
	if err := db.QueryRow(fmt.Sprintf(`SELECT success_count FROM %s WHERE cap_key = ?`, sqliteTableSkillLearnProgress), capKey).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// ResetSkillLearnSuccessCount clears the counter after a skill has been merged in (next extract needs N more successes).
func ResetSkillLearnSuccessCount(db *sql.DB, capKey string) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(fmt.Sprintf(`UPDATE %s SET success_count = 0 WHERE cap_key = ?`, sqliteTableSkillLearnProgress), capKey)
	return err
}
