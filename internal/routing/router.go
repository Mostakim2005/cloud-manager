package routing

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/db"
)

const SafetyFloorBytes int64 = 500 * 1024 * 1024 // 500 MiB floor

// QuotaReservationTracker prevents concurrent uploads from overdrafting near-full accounts.
type QuotaReservationTracker struct {
	mu       sync.Mutex
	reserved map[string]int64
}

// NewQuotaReservationTracker initializes an in-memory reservation ledger.
func NewQuotaReservationTracker() *QuotaReservationTracker {
	return &QuotaReservationTracker{reserved: make(map[string]int64)}
}

// Reserve attempts to reserve sizeBytes on account taking into account the 500 MiB floor.
func (q *QuotaReservationTracker) Reserve(account db.Account, sizeBytes int64) bool {
	if account.Provider == "youtube" {
		return true
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	avail := account.FreeBytes - q.reserved[account.ID] - SafetyFloorBytes
	if avail < sizeBytes {
		return false
	}
	q.reserved[account.ID] += sizeBytes
	return true
}

// Available returns remaining dispatchable bytes for accountId.
func (q *QuotaReservationTracker) Available(account db.Account) int64 {
	if account.Provider == "youtube" {
		return 1 << 60
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	avail := account.FreeBytes - q.reserved[account.ID] - SafetyFloorBytes
	if avail < 0 {
		return 0
	}
	return avail
}

// Release releases previously reserved bytes when a job finishes or fails.
func (q *QuotaReservationTracker) Release(accountID string, sizeBytes int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.reserved[accountID] -= sizeBytes
	if q.reserved[accountID] < 0 {
		q.reserved[accountID] = 0
	}
}

// SimulationResult details what a routing dry-run decided and why.
type SimulationResult struct {
	FileName         string `json:"fileName"`
	OriginType       string `json:"originType"`
	SizeBytes        int64  `json:"sizeBytes"`
	MatchedRuleID    int64  `json:"matchedRuleId"`
	MatchedRuleName  string `json:"matchedRuleName"`
	TargetProvider   string `json:"targetProvider"`
	TargetRemote     string `json:"targetRemote"`
	TargetPlaylist   string `json:"targetPlaylist,omitempty"`
	SpilloverApplied bool   `json:"spilloverApplied"`
	SpilloverReason  string `json:"spilloverReason,omitempty"`
	BandwidthCapKBps int    `json:"bandwidthCapKbps"`
}

// Evaluate selects the first matching rule ordered strictly priority ASC and applies quota spillover.
func Evaluate(
	rules []db.RoutingRule,
	accounts []db.Account,
	tracker *QuotaReservationTracker,
	settings config.Settings,
	fileName string,
	originType string,
	sizeBytes int64,
	now time.Time,
) SimulationResult {
	var matched *db.RoutingRule
	for i := range rules {
		r := rules[i]
		if !r.Enabled {
			continue
		}
		if matchesRule(r, fileName, originType, sizeBytes) {
			matched = &r
			break // Strict priority ASC first-match-wins
		}
	}
	if matched == nil {
		matched = &db.RoutingRule{
			ID:             0,
			Name:           "Built-In Safety Fallback",
			TargetProvider: "gdrive",
			TargetRemote:   "gdrive-1",
			SpilloverPool:  "gdrive-pool",
		}
	}

	targetRemote := matched.TargetRemote
	targetProvider := matched.TargetProvider
	spilled := false
	spillReason := ""

	if targetProvider != "youtube" {
		chosen, didSpill, reason := pickAccountWithSpillover(accounts, tracker, targetProvider, targetRemote, sizeBytes, now)
		if chosen != "" {
			targetRemote = chosen
		}
		spilled = didSpill
		spillReason = reason
	}

	return SimulationResult{
		FileName:         filepath.Base(fileName),
		OriginType:       originType,
		SizeBytes:        sizeBytes,
		MatchedRuleID:    matched.ID,
		MatchedRuleName:  matched.Name,
		TargetProvider:   targetProvider,
		TargetRemote:     targetRemote,
		TargetPlaylist:   matched.TargetPlaylist,
		SpilloverApplied: spilled,
		SpilloverReason:  spillReason,
		BandwidthCapKBps: BandwidthCapKBps(settings, now),
	}
}

func pickAccountWithSpillover(
	accounts []db.Account,
	tracker *QuotaReservationTracker,
	provider string,
	preferredID string,
	sizeBytes int64,
	now time.Time,
) (string, bool, string) {
	nowUnix := now.UTC().Unix()
	var preferred *db.Account
	for i := range accounts {
		if accounts[i].ID == preferredID || accounts[i].RcloneRemote == preferredID {
			preferred = &accounts[i]
			break
		}
	}
	hasRoom := func(a db.Account) bool {
		if a.CooldownUntil > nowUnix {
			return false
		}
		if tracker != nil {
			return tracker.Available(a) >= sizeBytes
		}
		return (a.FreeBytes - SafetyFloorBytes) >= sizeBytes
	}

	if preferred != nil && hasRoom(*preferred) {
		return preferred.ID, false, ""
	}
	for _, a := range accounts {
		if a.Provider != provider {
			continue
		}
		if preferred != nil && a.ID == preferred.ID {
			continue
		}
		if hasRoom(a) {
			reason := fmt.Sprintf("primary target %s below 500 MiB floor or cooling down", preferredID)
			return a.ID, true, reason
		}
	}
	return preferredID, false, ""
}

func matchesRule(r db.RoutingRule, fileName, originType string, sizeBytes int64) bool {
	if r.OriginMatch != "" && r.OriginMatch != "*" && !strings.EqualFold(r.OriginMatch, originType) {
		return false
	}
	if r.MinSizeBytes > 0 && sizeBytes < r.MinSizeBytes {
		return false
	}
	if r.MaxSizeBytes > 0 && sizeBytes > r.MaxSizeBytes {
		return false
	}
	if r.ExtGlob == "" || r.ExtGlob == "*" {
		return true
	}
	base := strings.ToLower(filepath.Base(fileName))
	for _, pat := range strings.Split(r.ExtGlob, ";") {
		pat = strings.TrimSpace(strings.ToLower(pat))
		if pat == "" {
			continue
		}
		if ok, _ := filepath.Match(pat, base); ok {
			return true
		}
	}
	return false
}

// BandwidthCapKBps returns the active KB/s cap for the given hour in local time (0 = unlimited).
func BandwidthCapKBps(s config.Settings, now time.Time) int {
	h := now.Hour()
	start := s.BandwidthDayStart
	end := s.BandwidthDayEnd
	inDay := false
	if start <= end {
		inDay = h >= start && h < end
	} else {
		inDay = h >= start || h < end
	}
	if inDay {
		return s.BandwidthDayKBps
	}
	return s.BandwidthNightKBps
}
