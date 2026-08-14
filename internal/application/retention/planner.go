package retention

import (
	"github.com/dbvault/dbvault/internal/domain"
	"sort"
	"time"
)

type Planner struct {
	Policy   domain.RetentionPolicy
	Location *time.Location
}

func New(policy domain.RetentionPolicy, loc *time.Location) Planner {
	if policy.KeepLast == 0 {
		policy = domain.DefaultRetentionPolicy()
	}
	if loc == nil {
		loc = time.UTC
	}
	return Planner{Policy: policy, Location: loc}
}
func (p Planner) Plan(snaps []domain.Snapshot) domain.RetentionDecision {
	eligible := []domain.Snapshot{}
	for _, s := range snaps {
		if s.Status == domain.SnapshotCommitted || s.Status == domain.SnapshotProtected {
			eligible = append(eligible, s)
		}
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].CreatedAt.After(eligible[j].CreatedAt) })
	keep := map[domain.SnapshotID][]domain.RetentionClass{}
	add := func(id domain.SnapshotID, c domain.RetentionClass) { keep[id] = append(keep[id], c) }
	for i, s := range eligible {
		if i < p.Policy.KeepLast {
			add(s.ID, domain.RetentionRecent)
		}
		if i == 0 {
			add(s.ID, domain.RetentionLatest)
		}
		if s.Status == domain.SnapshotProtected {
			add(s.ID, domain.RetentionProtected)
		}
		if s.RestoreTestedAt != nil && p.Policy.KeepLatestRestoreTested {
			add(s.ID, domain.RetentionRestoreTest)
		}
	}
	p.bucket(eligible, p.Policy.Daily, func(t time.Time) string { return t.In(p.Location).Format("2006-01-02") }, domain.RetentionDaily, add)
	p.bucket(eligible, p.Policy.Weekly, func(t time.Time) string {
		y, w := t.In(p.Location).ISOWeek()
		return string(rune(y)) + "-" + string(rune(w))
	}, domain.RetentionWeekly, add)
	p.bucket(eligible, p.Policy.Monthly, func(t time.Time) string { return t.In(p.Location).Format("2006-01") }, domain.RetentionMonthly, add)
	tomb := []domain.SnapshotID{}
	for _, s := range eligible {
		if len(keep[s.ID]) == 0 {
			tomb = append(tomb, s.ID)
		}
	}
	return domain.RetentionDecision{Keep: keep, Tombstone: tomb}
}
func (p Planner) bucket(snaps []domain.Snapshot, max int, key func(time.Time) string, class domain.RetentionClass, add func(domain.SnapshotID, domain.RetentionClass)) {
	if max <= 0 {
		return
	}
	seen := map[string]bool{}
	count := 0
	for _, s := range snaps {
		k := key(s.CreatedAt)
		if !seen[k] {
			seen[k] = true
			add(s.ID, class)
			count++
			if count >= max {
				return
			}
		}
	}
}
