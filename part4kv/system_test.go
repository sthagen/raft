package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fortytw2/leaktest"
)

func sleepMs(n int) {
	time.Sleep(time.Duration(n) * time.Millisecond)
}

func TestSetupHarness(t *testing.T) {
	h := NewHarness(t, 3)
	defer h.Shutdown()
	sleepMs(80)
}

func TestClientRequestBeforeConsensus(t *testing.T) {
	h := NewHarness(t, 3)
	defer h.Shutdown()
	sleepMs(10)

	// The client will keep cycling between the services until a leader is found.
	c1 := h.NewClient()
	h.CheckPut(c1, "llave", "cosa")
	sleepMs(80)
}

func TestBasicPutGetSingleClient(t *testing.T) {
	// Basic smoke test: send one Put, followed by one Get from a single client.
	h := NewHarness(t, 3)
	defer h.Shutdown()
	h.CheckSingleLeader()

	c1 := h.NewClient()
	h.CheckPut(c1, "llave", "cosa")

	h.CheckGet(c1, "llave", "cosa")
	sleepMs(80)
}

func TestPutPrevValue(t *testing.T) {
	h := NewHarness(t, 3)
	defer h.Shutdown()
	h.CheckSingleLeader()

	c1 := h.NewClient()
	// Make sure we get the expected found/prev values before and after Put
	prev, found := h.CheckPut(c1, "llave", "cosa")
	if found || prev != "" {
		t.Errorf(`got found=%v, prev=%v, want false/""`, found, prev)
	}

	prev, found = h.CheckPut(c1, "llave", "frodo")
	if !found || prev != "cosa" {
		t.Errorf(`got found=%v, prev=%v, want true/"cosa"`, found, prev)
	}

	// A different key...
	prev, found = h.CheckPut(c1, "mafteah", "davar")
	if found || prev != "" {
		t.Errorf(`got found=%v, prev=%v, want false/""`, found, prev)
	}
}

func TestBasicPutGetDifferentClients(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	h := NewHarness(t, 3)
	defer h.Shutdown()
	h.CheckSingleLeader()

	c1 := h.NewClient()
	h.CheckPut(c1, "k", "v")

	c2 := h.NewClient()
	h.CheckGet(c2, "k", "v")
	sleepMs(80)
}

func TestParallelClientsPutsAndGets(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	// Test that we can submit multiple PUT and GET requests in parallel
	h := NewHarness(t, 3)
	defer h.Shutdown()
	h.CheckSingleLeader()

	n := 9
	for i := range n {
		go func() {
			c := h.NewClient()
			_, f := h.CheckPut(c, fmt.Sprintf("key%v", i), fmt.Sprintf("value%v", i))
			if f {
				t.Errorf("got key found for %d, want false", i)
			}
		}()
	}
	sleepMs(150)

	for i := range n {
		go func() {
			c := h.NewClient()
			h.CheckGet(c, fmt.Sprintf("key%v", i), fmt.Sprintf("value%v", i))
		}()
	}
	sleepMs(150)
}

func TestDisconnectLeaderAfterPuts(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	h := NewHarness(t, 3)
	defer h.Shutdown()
	lid := h.CheckSingleLeader()

	// Submit some PUT commands.
	n := 4
	for i := range n {
		c := h.NewClient()
		_, f := h.CheckPut(c, fmt.Sprintf("key%v", i), fmt.Sprintf("value%v", i))
		if f {
			t.Errorf("got key found for %d, want false", i)
		}
	}

	h.DisconnectServiceFromPeers(lid)
	sleepMs(300)
	newlid := h.CheckSingleLeader()

	if newlid == lid {
		t.Errorf("got the same leader")
	}

	// GET commands expecting to get the right values
	c := h.NewClient()
	for i := range n {
		h.CheckGet(c, fmt.Sprintf("key%v", i), fmt.Sprintf("value%v", i))
	}

	// At the end of the test, reconnect the peers to avoid a goroutine leak.
	// In real scenarios, we expect that services will eventually be reconnected,
	// and if not - a single goroutine leaked is not an issue since the server
	// will end up being killed anyway.
	h.ReconnectServiceToPeers(lid)
	sleepMs(200)
}

func TestDisconnectLeaderAndFollower(t *testing.T) {
	h := NewHarness(t, 3)
	defer h.Shutdown()
	lid := h.CheckSingleLeader()

	// Submit some PUT commands.
	n := 4
	for i := range n {
		c := h.NewClient()
		_, f := h.CheckPut(c, fmt.Sprintf("key%v", i), fmt.Sprintf("value%v", i))
		if f {
			t.Errorf("got key found for %d, want false", i)
		}
	}

	// Disconnect leader and one other server; the cluster loses consensus
	// and client requests should now time out.
	h.DisconnectServiceFromPeers(lid)
	otherId := (lid + 1) % 3
	h.DisconnectServiceFromPeers(otherId)
	sleepMs(100)

	c := h.NewClient()
	ctx, cancel := context.WithTimeout(h.ctx, 500*time.Millisecond)
	_, _, err := c.Get(ctx, "key0")
	if err == nil {
		t.Errorf("want error when no leader, got nil")
	}
	cancel()

	// Reconnect one server, but not the old leader. We should still get all
	// the right data back.
	h.ReconnectServiceToPeers(otherId)
	h.CheckSingleLeader()
	for i := range n {
		h.CheckGet(c, fmt.Sprintf("key%v", i), fmt.Sprintf("value%v", i))
	}

	// Reconnect the old leader. We should still get all the right data back.
	h.ReconnectServiceToPeers(lid)
	h.CheckSingleLeader()
	for i := range n {
		h.CheckGet(c, fmt.Sprintf("key%v", i), fmt.Sprintf("value%v", i))
	}
	sleepMs(100)

	// TODO: try to make leaktest pass here - need better shutdown of service
	// goroutines.
}

func TestCrashFollower(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	h := NewHarness(t, 3)
	defer h.Shutdown()
	lid := h.CheckSingleLeader()

	// Submit some PUT commands.
	n := 3
	for i := range n {
		c := h.NewClient()
		_, f := h.CheckPut(c, fmt.Sprintf("key%v", i), fmt.Sprintf("value%v", i))
		if f {
			t.Errorf("got key found for %d, want false", i)
		}
	}

	// Crash a non-leader
	otherId := (lid + 1) % 3
	h.CrashService(otherId)

	// Talking directly to the leader should still work...
	for i := range n {
		c := h.NewClientSingleService(lid)
		h.CheckGet(c, fmt.Sprintf("key%v", i), fmt.Sprintf("value%v", i))
	}

	// Talking to the remaining live servers should also work
	for i := range n {
		c := h.NewClient()
		h.CheckGet(c, fmt.Sprintf("key%v", i), fmt.Sprintf("value%v", i))
	}
}

func TestCrashLeader(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	h := NewHarness(t, 3)
	defer h.Shutdown()
	lid := h.CheckSingleLeader()

	// Submit some PUT commands.
	n := 3
	for i := range n {
		c := h.NewClient()
		_, f := h.CheckPut(c, fmt.Sprintf("key%v", i), fmt.Sprintf("value%v", i))
		if f {
			t.Errorf("got key found for %d, want false", i)
		}
	}

	// Crash a leader and wait for the cluster to establish a new leader.
	h.CrashService(lid)
	h.CheckSingleLeader()

	// Talking to the remaining live servers should get the right data.
	for i := range n {
		c := h.NewClient()
		h.CheckGet(c, fmt.Sprintf("key%v", i), fmt.Sprintf("value%v", i))
	}
}

func TestCrashThenRestartLeader(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	h := NewHarness(t, 3)
	defer h.Shutdown()
	lid := h.CheckSingleLeader()

	// Submit some PUT commands.
	n := 3
	for i := range n {
		c := h.NewClient()
		_, f := h.CheckPut(c, fmt.Sprintf("key%v", i), fmt.Sprintf("value%v", i))
		if f {
			t.Errorf("got key found for %d, want false", i)
		}
	}

	// Crash a leader and wait for the cluster to establish a new leader.
	h.CrashService(lid)
	h.CheckSingleLeader()

	// Talking to the remaining live servers should get the right data.
	for i := range n {
		c := h.NewClient()
		h.CheckGet(c, fmt.Sprintf("key%v", i), fmt.Sprintf("value%v", i))
	}

	// Now restart the old leader: it will join the cluster and get all the
	// data.
	h.RestartService(lid)

	// Get data from services in different orders.
	for range 5 {
		c := h.NewClientWithRandomAddrsOrder()
		for j := range n {
			h.CheckGet(c, fmt.Sprintf("key%v", j), fmt.Sprintf("value%v", j))
		}
	}
}
