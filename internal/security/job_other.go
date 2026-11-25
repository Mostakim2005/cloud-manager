//go:build !windows

package security

import "sync"

// JobSupervisor is a portable no-op process tracker on non-Windows platforms.
type JobSupervisor struct {
	mu   sync.Mutex
	pids []int
}

// NewJobSupervisor creates a portable process tracker.
func NewJobSupervisor() (*JobSupervisor, error) {
	return &JobSupervisor{}, nil
}

// AssignPID records the supervised child PID for non-Windows inspection.
func (j *JobSupervisor) AssignPID(pid int) error {
	if j == nil || pid <= 0 {
		return nil
	}
	j.mu.Lock()
	j.pids = append(j.pids, pid)
	j.mu.Unlock()
	return nil
}

// Close clears tracked child PIDs.
func (j *JobSupervisor) Close() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	j.pids = nil
	j.mu.Unlock()
	return nil
}
