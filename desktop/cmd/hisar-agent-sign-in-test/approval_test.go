package main

import "testing"

func TestApprovalSelectorFindsTheRenamedWindow(t *testing.T) {
	selector := (approvalWindow{}).selector("role=AXButton;title=Approve once")
	if selector != "window=Approve request;role=AXButton;title=Approve once" {
		t.Fatalf("approval selector = %q", selector)
	}
}
