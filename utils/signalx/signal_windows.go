// SPDX-FileCopyrightText: 2017 Kubernetes.
// SPDX-License-Identifier: Apache-2.0

package signalx

import (
	"os"
)

var shutdownSignals = []os.Signal{os.Interrupt}
