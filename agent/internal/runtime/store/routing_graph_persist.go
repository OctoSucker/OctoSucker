package store

import (
	"database/sql"
	"encoding/json"
	"strconv"

	"github.com/OctoSucker/agent/pkg/ports"
)

type routingRecentDTO struct {
	Intent  string `json:"intent"`
	From    string `json:"from"`
	To      string `json:"to"`
	Outcome int    `json:"outcome"`
}

func (s *RoutingGraph) loadFromDB() error {
	if s.db == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT from_cap, to_cap, success, failure, cost, latency FROM routing_edges`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var from, to string
		var success, failure, cost, latency float64
		if err := rows.Scan(&from, &to, &success, &failure, &cost, &latency); err != nil {
			return err
		}
		k := edgeKey{from: from, to: to}
		s.edges[k] = &portsEdge{Success: success, Failure: failure, Cost: cost, Latency: latency}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var v sql.NullString
	if err := s.db.QueryRow(`SELECT v FROM routing_meta WHERE k = 'total_runs'`).Scan(&v); err == nil && v.Valid {
		if n, err := strconv.ParseInt(v.String, 10, 64); err == nil {
			s.totalRuns = n
		}
	}
	v = sql.NullString{}
	if err := s.db.QueryRow(`SELECT v FROM routing_meta WHERE k = 'recent_transitions'`).Scan(&v); err == nil && v.Valid && v.String != "" {
		var dtos []routingRecentDTO
		if json.Unmarshal([]byte(v.String), &dtos) == nil {
			for _, d := range dtos {
				s.recentTransitions = append(s.recentTransitions, contextTransition{
					Intent: d.Intent, From: d.From, To: d.To, Outcome: d.Outcome,
				})
			}
		}
	}
	return nil
}

func (s *RoutingGraph) persistEdgeAndRecentLocked(k edgeKey, e *portsEdge) {
	if s.db == nil || e == nil {
		return
	}
	_, _ = s.db.Exec(`INSERT INTO routing_edges (from_cap, to_cap, success, failure, cost, latency) VALUES (?,?,?,?,?,?)
		ON CONFLICT(from_cap, to_cap) DO UPDATE SET
			success = excluded.success,
			failure = excluded.failure,
			cost = excluded.cost,
			latency = excluded.latency`,
		k.from, k.to, e.Success, e.Failure, e.Cost, e.Latency)
	s.persistRecentLocked()
}

func (s *RoutingGraph) persistRecentLocked() {
	if s.db == nil {
		return
	}
	dtos := make([]routingRecentDTO, 0, len(s.recentTransitions))
	for _, t := range s.recentTransitions {
		dtos = append(dtos, routingRecentDTO{
			Intent: t.Intent, From: t.From, To: t.To, Outcome: t.Outcome,
		})
	}
	b, err := json.Marshal(dtos)
	if err != nil {
		return
	}
	_, _ = s.db.Exec(`INSERT INTO routing_meta (k, v) VALUES ('recent_transitions', ?)
		ON CONFLICT(k) DO UPDATE SET v = excluded.v`, string(b))
}

func (s *RoutingGraph) persistTotalRunsLocked() {
	if s.db == nil {
		return
	}
	_, _ = s.db.Exec(`INSERT INTO routing_meta (k, v) VALUES ('total_runs', ?)
		ON CONFLICT(k) DO UPDATE SET v = excluded.v`, strconv.FormatInt(s.totalRuns, 10))
}

func (s *RoutingGraph) persistAllEdgesLocked() {
	if s.db == nil {
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM routing_edges`); err != nil {
		return
	}
	for k, e := range s.edges {
		if _, err := tx.Exec(`INSERT INTO routing_edges (from_cap, to_cap, success, failure, cost, latency) VALUES (?,?,?,?,?,?)`,
			k.from, k.to, e.Success, e.Failure, e.Cost, e.Latency); err != nil {
			return
		}
	}
	_ = tx.Commit()
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
		_, _ = s.db.Exec(`INSERT INTO routing_edges (from_cap, to_cap, success, failure, cost, latency) VALUES (?,?,?,?,?,?)
			ON CONFLICT(from_cap, to_cap) DO UPDATE SET
				success = excluded.success,
				failure = excluded.failure,
				cost = excluded.cost,
				latency = excluded.latency`,
			k.from, k.to, e.Success, e.Failure, e.Cost, e.Latency)
	}
	s.persistTotalRunsLocked()
}
