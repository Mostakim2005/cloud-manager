package routing

import (
	"testing"
	"time"

	"github.com/Mostakim2005/cloud-manager/internal/config"
	"github.com/Mostakim2005/cloud-manager/internal/db"
)

func TestPriorityOrderingAndQuotaSpillover(t *testing.T) {
	rules := []db.RoutingRule{
		{ID: 1, Priority: 10, Name: "Recordings to YouTube", OriginMatch: "recordings", ExtGlob: "*.mp4", TargetProvider: "youtube", TargetRemote: "youtube-private", TargetPlaylist: "PL1", Enabled: true},
		{ID: 2, Priority: 20, Name: "PDFs to Mega", OriginMatch: "*", ExtGlob: "*.pdf", TargetProvider: "mega", TargetRemote: "mega-1", Enabled: true},
		{ID: 3, Priority: 30, Name: "Large files to GDrive", OriginMatch: "*", ExtGlob: "*", MinSizeBytes: 100 * 1024 * 1024, TargetProvider: "gdrive", TargetRemote: "gdrive-1", Enabled: true},
	}
	accounts := []db.Account{
		{ID: "mega-1", Provider: "mega", FreeBytes: 600 * 1024 * 1024}, // Only 100 MiB above 500 MiB safety floor
		{ID: "mega-2", Provider: "mega", FreeBytes: 5 * 1024 * 1024 * 1024},
		{ID: "gdrive-1", Provider: "gdrive", FreeBytes: 10 * 1024 * 1024 * 1024},
	}

	tracker := NewQuotaReservationTracker()
	settings := config.DefaultSettings("")
	day := time.Date(2026, 11, 25, 14, 0, 0, 0, time.Local)

	// 150 MiB PDF matches rule 2 (priority 20 < 30), then spills from mega-1 to mega-2!
	sim := Evaluate(rules, accounts, tracker, settings, "textbook.pdf", "pc", 150*1024*1024, day)
	if sim.MatchedRuleID != 2 {
		t.Fatalf("expected rule 2 first-match-wins, got %d (%s)", sim.MatchedRuleID, sim.MatchedRuleName)
	}
	if !sim.SpilloverApplied || sim.TargetRemote != "mega-2" {
		t.Fatalf("expected spillover to mega-2, got %+v", sim)
	}
	if sim.BandwidthCapKBps != settings.BandwidthDayKBps {
		t.Fatalf("expected daytime bandwidth cap %d, got %d", settings.BandwidthDayKBps, sim.BandwidthCapKBps)
	}

	// Verify concurrent reservation prevents overdraft.
	if !tracker.Reserve(accounts[1], 4*1024*1024*1024) {
		t.Fatalf("expected reservation on mega-2 to succeed")
	}
	if tracker.Reserve(accounts[1], 1*1024*1024*1024) {
		t.Fatalf("expected second reservation exceeding 500 MiB safety floor to fail")
	}
	tracker.Release("mega-2", 4*1024*1024*1024)
	if !tracker.Reserve(accounts[1], 1*1024*1024*1024) {
		t.Fatalf("expected reservation after release to succeed")
	}
}
