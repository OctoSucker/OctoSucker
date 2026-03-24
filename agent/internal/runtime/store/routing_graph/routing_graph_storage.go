package routinggraph

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/OctoSucker/agent/pkg/ports"
)

// Table names must stay aligned with store migrate (store/tables.go).
const (
	sqliteTableRoutingEdges = "routing_edges"
	sqliteTableRoutingMeta  = "routing_meta"
)

type routingRecentDTO struct {
	Intent  string `json:"intent"`
	From    string `json:"from"`
	To      string `json:"to"`
	Outcome int    `json:"outcome"`
}

func (s *RoutingGraph) loadFromDB() error {
	if s == nil || s.db == nil {
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

func (s *RoutingGraph) persistEdgeAndRecentLocked(k edgeKey, e *portsEdge) {
	if s.db == nil || e == nil {
		return
	}
	_ = upsertRoutingEdgeSQLite(s.db, k, e)
	_ = saveRoutingRecentSQLite(s.db, s.recentTransitions)
}

func (s *RoutingGraph) persistTotalRunsLocked() {
	if s.db == nil {
		return
	}
	_ = saveRoutingTotalRunsSQLite(s.db, s.totalRuns)
}

func (s *RoutingGraph) persistAllEdgesLocked() {
	if s.db == nil {
		return
	}
	_ = replaceAllRoutingEdgesSQLite(s.db, s.edges)
}

func (s *RoutingGraph) persistTrajectoryLocked(path []ports.TransitionStep) {
	if s.db == nil {
		return
	}
	for _, step := range path {
		k := edgeKey{from: step.From, to: step.To}
		e := s.edges[k]
		if e == nil {
			continue
		}
		_ = upsertRoutingEdgeSQLite(s.db, k, e)
	}
	s.persistTotalRunsLocked()
}

func loadRoutingGraphStateSQLite(db *sql.DB) (map[edgeKey]*portsEdge, int64, []contextTransition, error) {
	if db == nil {
		return nil, 0, nil, nil
	}
	edges := make(map[edgeKey]*portsEdge)
	rows, err := db.Query(fmt.Sprintf(`SELECT from_cap, to_cap, success, failure, cost, latency FROM %s`, sqliteTableRoutingEdges))
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
	if err := db.QueryRow(fmt.Sprintf(`SELECT v FROM %s WHERE k = 'total_runs'`, sqliteTableRoutingMeta)).Scan(&v); err == nil && v.Valid {
		if n, err := strconv.ParseInt(v.String, 10, 64); err == nil {
			totalRuns = n
		}
	}
	var recent []contextTransition
	v = sql.NullString{}
	if err := db.QueryRow(fmt.Sprintf(`SELECT v FROM %s WHERE k = 'recent_transitions'`, sqliteTableRoutingMeta)).Scan(&v); err == nil && v.Valid && v.String != "" {
		var dtos []routingRecentDTO
		if json.Unmarshal([]byte(v.String), &dtos) == nil {
			for _, d := range dtos {
				recent = append(recent, contextTransition{
					Intent: d.Intent, From: d.From, To: d.To, Outcome: d.Outcome,
				})
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
			latency = excluded.latency`, sqliteTableRoutingEdges),
		k.from, k.to, e.Success, e.Failure, e.Cost, e.Latency)
	return err
}

func saveRoutingRecentSQLite(db *sql.DB, recent []contextTransition) error {
	if db == nil {
		return nil
	}
	dtos := make([]routingRecentDTO, 0, len(recent))
	for _, t := range recent {
		dtos = append(dtos, routingRecentDTO{
			Intent: t.Intent, From: t.From, To: t.To, Outcome: t.Outcome,
		})
	}
	b, err := json.Marshal(dtos)
	if err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf(`INSERT INTO %s (k, v) VALUES ('recent_transitions', ?)
		ON CONFLICT(k) DO UPDATE SET v = excluded.v`, sqliteTableRoutingMeta), string(b))
	return err
}

func saveRoutingTotalRunsSQLite(db *sql.DB, n int64) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(fmt.Sprintf(`INSERT INTO %s (k, v) VALUES ('total_runs', ?)
		ON CONFLICT(k) DO UPDATE SET v = excluded.v`, sqliteTableRoutingMeta), strconv.FormatInt(n, 10))
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
	if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM %s`, sqliteTableRoutingEdges)); err != nil {
		return err
	}
	for k, e := range edges {
		if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO %s (from_cap, to_cap, success, failure, cost, latency) VALUES (?,?,?,?,?,?)`, sqliteTableRoutingEdges),
			k.from, k.to, e.Success, e.Failure, e.Cost, e.Latency); err != nil {
			return err
		}
	}
	return tx.Commit()
}
