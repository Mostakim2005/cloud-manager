package telemetry

import (
	"sync"
	"time"
)

// Event represents a redacted live telemetry tick or state transition.
type Event struct {
	Timestamp string         `json:"ts"`
	Kind      string         `json:"kind"`
	Component string         `json:"component"`
	Message   string         `json:"message"`
	Progress  *TransferTick  `json:"progress,omitempty"`
	Meta      map[string]any `json:"meta,omitempty"`
}

// TransferTick holds live throughput metrics stored strictly in memory (never thrashing SQLite WAL).
type TransferTick struct {
	JobID      int64   `json:"jobId"`
	Label      string  `json:"label"`
	BytesDone  int64   `json:"bytesDone"`
	BytesTotal int64   `json:"bytesTotal"`
	SpeedBps   float64 `json:"speedBps"`
	Percent    float64 `json:"percent"`
	Status     string  `json:"status"`
}

// Snapshot is broadcast to SSE listeners at <= 5 Hz (every 200 ms).
type Snapshot struct {
	Timestamp     string          `json:"ts"`
	TotalSpeedBps float64         `json:"totalSpeedBps"`
	ActiveJobs    []TransferTick  `json:"activeJobs"`
	RecentEvents  []Event         `json:"recentEvents"`
	AdapterStates map[string]bool `json:"adapterStates"`
}

// Hub maintains bounded in-memory telemetry and pushes <= 5 Hz snapshots to SSE clients.
// When zero SSE subscribers are connected, no periodic ticker runs (0% idle CPU).
type Hub struct {
	mu          sync.Mutex
	ring        []Event
	ringCap     int
	active      map[int64]TransferTick
	adapters    map[string]bool
	subs        map[chan Snapshot]struct{}
	dirty       bool
	wakeCh      chan struct{}
	stopCh      chan struct{}
	minInterval time.Duration
}

// NewHub initializes the telemetry ring buffer and demand-driven <= 5 Hz broadcast loop.
func NewHub() *Hub {
	h := &Hub{
		ringCap:     250,
		active:      make(map[int64]TransferTick),
		adapters:    make(map[string]bool),
		subs:        make(map[chan Snapshot]struct{}),
		wakeCh:      make(chan struct{}, 1),
		stopCh:      make(chan struct{}),
		minInterval: 200 * time.Millisecond, // Strictly <= 5 Hz
	}
	go h.loop()
	return h
}

func (h *Hub) loop() {
	var ticker *time.Ticker
	var tickCh <-chan time.Time
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()

	for {
		select {
		case <-h.stopCh:
			return
		case <-h.wakeCh:
			h.mu.Lock()
			hasSubs := len(h.subs) > 0
			h.mu.Unlock()
			if hasSubs && ticker == nil {
				ticker = time.NewTicker(h.minInterval)
				tickCh = ticker.C
			} else if !hasSubs && ticker != nil {
				ticker.Stop()
				ticker = nil
				tickCh = nil
			}
		case <-tickCh:
			h.mu.Lock()
			if len(h.subs) == 0 {
				if ticker != nil {
					ticker.Stop()
					ticker = nil
					tickCh = nil
				}
				h.mu.Unlock()
				continue
			}
			if !h.dirty {
				h.mu.Unlock()
				continue
			}
			snap := h.snapshotLocked()
			h.dirty = false
			subs := make([]chan Snapshot, 0, len(h.subs))
			for ch := range h.subs {
				subs = append(subs, ch)
			}
			h.mu.Unlock()

			for _, ch := range subs {
				select {
				case ch <- snap:
				default:
				}
			}
		}
	}
}

func (h *Hub) signalWake() {
	select {
	case h.wakeCh <- struct{}{}:
	default:
	}
}

// Close shuts down the telemetry loop cleanly.
func (h *Hub) Close() {
	select {
	case <-h.stopCh:
	default:
		close(h.stopCh)
	}
}

// Emit appends a bounded event to the in-memory ring buffer.
func (h *Hub) Emit(ev Event) {
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.ring) >= h.ringCap {
		copy(h.ring, h.ring[1:])
		h.ring[len(h.ring)-1] = ev
	} else {
		h.ring = append(h.ring, ev)
	}
	if len(h.subs) > 0 {
		h.dirty = true
	}
}

// UpdateTransfer updates an in-flight job progress tick in memory without touching SQLite.
func (h *Hub) UpdateTransfer(tick TransferTick) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if tick.Status == "SYNCED" || tick.Status == "FAILED" || tick.Status == "CANCELLED" {
		delete(h.active, tick.JobID)
	} else {
		if tick.BytesTotal > 0 {
			tick.Percent = float64(tick.BytesDone) * 100 / float64(tick.BytesTotal)
		}
		h.active[tick.JobID] = tick
	}
	if len(h.subs) > 0 {
		h.dirty = true
	}
}

// SetAdapterState updates live availability for an adapter.
func (h *Hub) SetAdapterState(name string, online bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.adapters[name] = online
	if len(h.subs) > 0 {
		h.dirty = true
	}
}

// Snapshot returns the current telemetry snapshot.
func (h *Hub) Snapshot() Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.snapshotLocked()
}

func (h *Hub) snapshotLocked() Snapshot {
	active := make([]TransferTick, 0, len(h.active))
	var totalBps float64
	for _, t := range h.active {
		active = append(active, t)
		if t.Status == "RUNNING" {
			totalBps += t.SpeedBps
		}
	}
	events := make([]Event, len(h.ring))
	copy(events, h.ring)
	adapters := make(map[string]bool, len(h.adapters))
	for k, v := range h.adapters {
		adapters[k] = v
	}
	return Snapshot{
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		TotalSpeedBps: totalBps,
		ActiveJobs:    active,
		RecentEvents:  events,
		AdapterStates: adapters,
	}
}

// Subscribe registers a client SSE channel and wakes the <= 5 Hz ticker on demand.
func (h *Hub) Subscribe() (<-chan Snapshot, func()) {
	ch := make(chan Snapshot, 8)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	snap := h.snapshotLocked()
	h.mu.Unlock()
	h.signalWake()

	select {
	case ch <- snap:
	default:
	}

	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
		h.signalWake()
	}
}
