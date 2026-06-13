/*
***** WHAT THIS FILE DOES *****
- Prints that the service is starting
- Connects to Docker Desktop (via sandbox.NewSandbox())
- Confirms the sandbox is ready
*/


package main

import (
	"fmt"
	"submission-engine/sandbox"
)

func main() {
	fmt.Println("IICPC Platform - Submission Engine starting...")

	// create a new sandbox
	sb, err := sandbox.NewSandbox()
	if err != nil {
		fmt.Printf("Failed to create sandbox: %v\n", err)
		return
	}

	fmt.Println("Sandbox initialized successfully!")
	fmt.Printf("Ready to run submissions: %v\n", sb)
	fmt.Println("Submission Engine running and waiting for submissions...")
}