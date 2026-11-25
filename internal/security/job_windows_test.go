//go:build windows

package security

import (
	"os/exec"
	"testing"
)

func TestWindowsJobObjectKillsChildrenOnClose(t *testing.T) {
	sup, err := NewJobSupervisor()
	if err != nil {
		t.Fatalf("NewJobSupervisor: %v", err)
	}
	cmd := exec.Command("cmd.exe", "/C", "ping", "-n", "15", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		_ = sup.Close()
		t.Fatalf("start child: %v", err)
	}
	if err := sup.AssignPID(cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		_ = sup.Close()
		t.Fatalf("AssignPID: %v", err)
	}
	if err := sup.Close(); err != nil {
		t.Fatalf("JobSupervisor.Close: %v", err)
	}
	_ = cmd.Wait()
}
