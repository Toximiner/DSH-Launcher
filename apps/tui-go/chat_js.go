//go:build js && wasm

package main

import "context"

// The static adapter never opens a connection to a backend.
func followChat(ctx context.Context, login, id string, out chan<- chatNotice) { <-ctx.Done() }

func followApprovalEvents(ctx context.Context, login, id string, out chan<- approvalNotice) {
	<-ctx.Done()
}
