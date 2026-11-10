package service

import (
	"sync"
	"testing"
)

// Sessions opened and closed at once on one connection, as when a client drops
// several of them together, must not race on godbus's export table.
func TestConcurrentSessionsExportSafely(t *testing.T) {
	conn, cleanup := startTestBus(t)
	defer cleanup()
	m := NewSessionManager(conn)

	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				s, _, err := m.CreateSession("plain", nil, "test")
				if err != nil {
					t.Error(err)
					return
				}
				s.close()
			}
		}()
	}
	wg.Wait()
}
