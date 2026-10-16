package main

import (
	"testing"
)

func TestSingleInstanceLock(t *testing.T) {
	release1, err := EnsureSingleInstance()
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	_, err = EnsureSingleInstance()
	if err == nil {
		t.Fatal("expected second instance to fail")
	}
	release1()
	release2, err := EnsureSingleInstance()
	if err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	release2()
}
