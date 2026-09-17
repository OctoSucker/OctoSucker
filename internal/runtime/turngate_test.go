package runtime

import "testing"

func TestTurnGateSerializesSameKey(t *testing.T) {
	gate := newTurnGate(2)
	release, ok := gate.tryAcquire("same")
	if !ok {
		t.Fatal("first acquire failed")
	}
	if _, ok := gate.tryAcquire("same"); ok {
		t.Fatal("same key acquired concurrently")
	}
	release()
	if releaseAgain, ok := gate.tryAcquire("same"); !ok {
		t.Fatal("key was not released")
	} else {
		releaseAgain()
	}
}

func TestTurnGateAllowsIndependentKeysUpToLimit(t *testing.T) {
	gate := newTurnGate(2)
	releaseA, ok := gate.tryAcquire("a")
	if !ok {
		t.Fatal("acquire a failed")
	}
	releaseB, ok := gate.tryAcquire("b")
	if !ok {
		t.Fatal("acquire b failed")
	}
	if _, ok := gate.tryAcquire("c"); ok {
		t.Fatal("acquired beyond global limit")
	}
	releaseA()
	if releaseC, ok := gate.tryAcquire("c"); !ok {
		t.Fatal("slot was not released")
	} else {
		releaseC()
	}
	releaseB()
}

func TestTurnGateReleaseIsIdempotent(t *testing.T) {
	gate := newTurnGate(1)
	release, ok := gate.tryAcquire("a")
	if !ok {
		t.Fatal("acquire failed")
	}
	release()
	release()
	if releaseB, ok := gate.tryAcquire("b"); !ok {
		t.Fatal("idempotent release leaked the slot")
	} else {
		releaseB()
	}
}
