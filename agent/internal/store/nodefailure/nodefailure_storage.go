package nodefailure

import (
	"fmt"
	"strings"
)

// Must match store/tables.go migrate (TableNodeFailureStats).
const sqliteTableNodeFailureStats = "node_failure_stats"

func (n *NodeFailureStats) dbUpsertFailure(key, capability, tool, fromCap, sig string, now int64) error {
	_, err := n.db.Exec(fmt.Sprintf(`
INSERT INTO %s (dedup_key, capability, tool, from_cap, error_sig, failure_count, last_seen_unix)
VALUES (?, ?, ?, ?, ?, 1, ?)
ON CONFLICT(dedup_key) DO UPDATE SET
	failure_count = failure_count + 1,
	last_seen_unix = excluded.last_seen_unix
`, sqliteTableNodeFailureStats),
		key, capability, tool, fromCap, sig, now)
	return err
}

func (n *NodeFailureStats) dbQueryHintRows(caps []string, minCount, maxLines int) ([]hintRow, error) {
	placeholders := strings.Repeat("?,", len(caps))
	placeholders = strings.TrimSuffix(placeholders, ",")
	args := make([]any, 0, 1+len(caps)*2+1)
	args = append(args, minCount)
	for _, c := range caps {
		args = append(args, c)
	}
	for _, c := range caps {
		args = append(args, c)
	}
	args = append(args, maxLines)
	q := fmt.Sprintf(`
SELECT capability, tool, from_cap, error_sig, failure_count
FROM %s
WHERE failure_count >= ?
  AND (capability IN (%s) OR from_cap IN (%s))
ORDER BY failure_count DESC
LIMIT ?
`, sqliteTableNodeFailureStats, placeholders, placeholders)

	rows, err := n.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []hintRow
	for rows.Next() {
		var capID, tool, fromCap, errSig string
		var cnt int
		if err := rows.Scan(&capID, &tool, &fromCap, &errSig, &cnt); err != nil {
			continue
		}
		out = append(out, hintRow{capID: capID, tool: tool, fromCap: fromCap, errSig: errSig, cnt: cnt})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
