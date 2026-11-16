//go:build js && wasm

package main

import "context"

// The static adapter never opens a connection to a backend.
func followChat(ctx context.Context, login, id string, out chan<- chatNotice, addresses ...sessionReadAddress) {
	<-ctx.Done()
}

func followToolOutput(ctx context.Context, login, sessionID, jobID string, out chan<- toolOutput) {
	<-ctx.Done()
}

func followApprovalEvents(ctx context.Context, login, id string, out chan<- approvalNotice) {
	<-ctx.Done()
}

func followArchive(ctx context.Context, login string, state *archiveState) { <-ctx.Done() }
