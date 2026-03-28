package routinggraph

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/OctoSucker/agent/internal/store"
	"github.com/OctoSucker/agent/pkg/ports"
)

func (s *RoutingGraph) loadFromDB() error {
	if s.db == nil {
		return nil
	}
	edges, runs, recent, err := loadRoutingGraphStateSQLite(s.db)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, e := range edges {
		if e != nil {
			s.edges[k] = e
		}
	}
	s.totalRuns = runs
	s.recentTransitions = recent
	return nil
}

func (s *RoutingGraph) persistEdgeAndRecentLocked(k edgeKey, e *portsEdge) error {
	if s.db == nil || e == nil {
		return nil
	}
	if err := upsertRoutingEdgeSQLite(s.db, k, e); err != nil {
		return err
	}
	return saveRoutingRecentSQLite(s.db, s.recentTransitions)
}

func (s *RoutingGraph) persistTotalRunsLocked() error {
	if s.db == nil {
		return nil
	}
	return saveRoutingTotalRunsSQLite(s.db, s.totalRuns)
}

func (s *RoutingGraph) persistAllEdgesLocked() error {
	if s.db == nil {
		return nil
	}
	return replaceAllRoutingEdgesSQLite(s.db, s.edges)
}

func (s *RoutingGraph) persistTrajectoryLocked(path []ports.TransitionStep) error {
	if s.db == nil {
		return nil
	}
	for _, step := range path {
		k := edgeKey{from: step.From, to: step.To}
		e := s.edges[k]
		if e == nil {
			continue
		}
		if err := upsertRoutingEdgeSQLite(s.db, k, e); err != nil {
			return err
		}
	}
	return s.persistTotalRunsLocked()
}

func loadRoutingGraphStateSQLite(db *sql.DB) (map[edgeKey]*portsEdge, int64, []contextTransition, error) {
	if db == nil {
		return nil, 0, nil, nil
	}
	edges := make(map[edgeKey]*portsEdge)
	rows, err := db.Query(fmt.Sprintf(`SELECT from_cap, to_cap, success, failure, cost, latency FROM %s`, store.TableRoutingEdges))
	if err != nil {
		return nil, 0, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var from, to string
		var success, failure, cost, latency float64
		if err := rows.Scan(&from, &to, &success, &failure, &cost, &latency); err != nil {
			return nil, 0, nil, err
		}
		k := edgeKey{from: from, to: to}
		edges[k] = &portsEdge{Success: success, Failure: failure, Cost: cost, Latency: latency}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, nil, err
	}
	var totalRuns int64
	var v sql.NullString
	if err := db.QueryRow(fmt.Sprintf(`SELECT v FROM %s WHERE k = 'total_runs'`, store.TableRoutingMeta)).Scan(&v); err == nil && v.Valid {
		if n, err := strconv.ParseInt(v.String, 10, 64); err == nil {
			totalRuns = n
		}
	}
	var recent []contextTransition
	v = sql.NullString{}
	if err := db.QueryRow(fmt.Sprintf(`SELECT v FROM %s WHERE k = 'recent_transitions'`, store.TableRoutingMeta)).Scan(&v); err == nil && v.Valid && v.String != "" {
		var recentRows []contextTransition
		if json.Unmarshal([]byte(v.String), &recentRows) == nil {
			for _, d := range recentRows {
				recent = append(recent, contextTransition{
					Intent: d.Intent, From: d.From, To: d.To, Outcome: d.Outcome,
				})
			}
		} else {
			// Backward-compatibility for legacy stored rows where outcome used int (0 success, 1 failure).
			type routingRecentDTOLegacy struct {
				Intent  string `json:"intent"`
				From    string `json:"from"`
				To      string `json:"to"`
				Outcome int    `json:"outcome"`
			}
			var legacy []routingRecentDTOLegacy
			if json.Unmarshal([]byte(v.String), &legacy) == nil {
				for _, d := range legacy {
					recent = append(recent, contextTransition{
						Intent: d.Intent, From: d.From, To: d.To, Outcome: d.Outcome == 0,
					})
				}
			}
		}
	}
	return edges, totalRuns, recent, nil
}

func upsertRoutingEdgeSQLite(db *sql.DB, k edgeKey, e *portsEdge) error {
	if db == nil || e == nil {
		return nil
	}
	_, err := db.Exec(fmt.Sprintf(`INSERT INTO %s (from_cap, to_cap, success, failure, cost, latency) VALUES (?,?,?,?,?,?)
		ON CONFLICT(from_cap, to_cap) DO UPDATE SET
			success = excluded.success,
			failure = excluded.failure,
			cost = excluded.cost,
			latency = excluded.latency`, store.TableRoutingEdges),
		k.from, k.to, e.Success, e.Failure, e.Cost, e.Latency)
	return err
}

func saveRoutingRecentSQLite(db *sql.DB, recent []contextTransition) error {
	if db == nil {
		return nil
	}
	b, err := json.Marshal(recent)
	if err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf(`INSERT INTO %s (k, v) VALUES ('recent_transitions', ?)
		ON CONFLICT(k) DO UPDATE SET v = excluded.v`, store.TableRoutingMeta), string(b))
	return err
}

func saveRoutingTotalRunsSQLite(db *sql.DB, n int64) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(fmt.Sprintf(`INSERT INTO %s (k, v) VALUES ('total_runs', ?)
		ON CONFLICT(k) DO UPDATE SET v = excluded.v`, store.TableRoutingMeta), strconv.FormatInt(n, 10))
	return err
}

func replaceAllRoutingEdgesSQLite(db *sql.DB, edges map[edgeKey]*portsEdge) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM %s`, store.TableRoutingEdges)); err != nil {
		return err
	}
	for k, e := range edges {
		if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO %s (from_cap, to_cap, success, failure, cost, latency) VALUES (?,?,?,?,?,?)`, store.TableRoutingEdges),
			k.from, k.to, e.Success, e.Failure, e.Cost, e.Latency); err != nil {
			return err
		}
	}
	return tx.Commit()
}
