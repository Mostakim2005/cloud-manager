//go:build windows

package tray

import "testing"

func TestLoadCustomTrayIcons(t *testing.T) {
	smIcon, lgIcon, custom, cleanup := loadTrayIcons()
	defer cleanup()

	if !custom {
		t.Fatal("expected custom Cloud Manager PNG icon creation to succeed on Windows")
	}
	if smIcon == 0 || lgIcon == 0 {
		t.Fatalf("expected non-zero HICON handles, got sm=%x lg=%x", smIcon, lgIcon)
	}
}
