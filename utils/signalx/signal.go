// SPDX-FileCopyrightText: 2017 Kubernetes.
// SPDX-License-Identifier: Apache-2.0

package signalx

import (
	"context"
	"os"
	"os/signal"
)

var onlyOneSignalHandler = make(chan struct{})

// Context registers for SIGTERM and SIGINT.
// A context is returned which is canceled on one of these signals.
// If a second signal is caught, the program is terminated with exit code 1.
func Context() context.Context {
	close(onlyOneSignalHandler) // Panics when called twice.

	ctx, cancel := context.WithCancel(context.Background())

	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)

	go func() {
		<-c
		cancel()
		<-c
		os.Exit(1) // Second signal. Exit directly.
	}()

	return ctx
}
