package application

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// splitCSVPath is filepath.Split with one correction: a bare relative
// filename ("crosswalk.csv", no directory component) splits to dir="",
// which os.OpenRoot (csvReader's confinement mechanism) does not treat as
// "the current directory" the way many other os functions do — normalizing
// to "." keeps a plain relative --crosswalk/--aggregate-members flag value
// working.
func splitCSVPath(csvPath string) (dir, file string) {
	dir, file = filepath.Split(csvPath)
	if dir == "" {
		dir = "."
	}
	return dir, file
}

// loadCrosswalk reads eurosl_crosswalk.csv (name,concept_id) into a
// name -> concept-ids dictionary. More than one row for a name is not
// malformed — it is how an ambiguous name is recorded — so every id is kept
// for resolveRow to judge. A row with an empty name or concept_id is
// malformed (skipped, counted): a blank concept_id sitting alongside a real
// one would otherwise misclassify the name as ambiguous ({"", "wcvp:..."}
// counts as two distinct ids) instead of simply unresolved.
func loadCrosswalk(ctx context.Context, csvPath string) (map[string][]string, int, error) {
	dir, file := splitCSVPath(csvPath)
	crosswalk := map[string][]string{}
	skipped := 0
	skip := newRowSkipper(&skipped, file, "crosswalk entry")
	err := readAll(ctx, dir, file, ',', []string{nameField, "concept_id"}, skip,
		func(idx map[string]int, r []string, line int) error {
			name := r[idx[nameField]]
			id := r[idx["concept_id"]]
			if name == "" || id == "" {
				skip(line, fmt.Errorf("empty name or concept_id"))
				return nil
			}
			crosswalk[name] = append(crosswalk[name], id)
			return nil
		})
	if err != nil {
		return nil, 0, err
	}
	return crosswalk, skipped, nil
}

// aggregateMember is one member species of an aggregate, as read from
// aggregate_members.csv.
type aggregateMember struct {
	conceptID string
	name      string
}

// loadAggregateMembers reads aggregate_members.csv (aggregate_concept_id,
// member_concept_id, member_name) into a dictionary keyed by the aggregate's
// own concept id. A missing file is not an error — it just means no
// derivation runs this pass, logged so the operator can tell "no aggregates"
// apart from "forgot the file".
func loadAggregateMembers(ctx context.Context, csvPath string) (map[string][]aggregateMember, int, error) {
	if _, err := os.Stat(csvPath); err != nil {
		if os.IsNotExist(err) {
			slog.Warn("aggregate_members.csv not found: skipping aggregate-member derivation", "path", csvPath)
			return map[string][]aggregateMember{}, 0, nil
		}
		return nil, 0, fmt.Errorf("checking %s: %w", csvPath, err)
	}

	dir, file := splitCSVPath(csvPath)
	members := map[string][]aggregateMember{}
	skipped := 0
	skip := newRowSkipper(&skipped, file, "aggregate member")
	err := readAll(ctx, dir, file, ',',
		[]string{"aggregate_concept_id", "member_concept_id", "member_name"}, skip,
		func(idx map[string]int, r []string, line int) error {
			aggregateID := r[idx["aggregate_concept_id"]]
			memberID := r[idx["member_concept_id"]]
			memberName := r[idx["member_name"]]
			if aggregateID == "" || memberID == "" || memberName == "" {
				skip(line, fmt.Errorf("empty aggregate_concept_id, member_concept_id or member_name"))
				return nil
			}
			members[aggregateID] = append(members[aggregateID], aggregateMember{
				conceptID: memberID,
				name:      memberName,
			})
			return nil
		})
	if err != nil {
		return nil, 0, err
	}
	return members, skipped, nil
}
