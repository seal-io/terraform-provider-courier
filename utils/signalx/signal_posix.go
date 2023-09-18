// SPDX-FileCopyrightText: 2017 Kubernetes.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package signalx

import (
	"os"
	"syscall"
)

var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}
