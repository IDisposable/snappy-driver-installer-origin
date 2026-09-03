// Command hwdump prints everything the internal/hardware package can
// detect about the current machine as JSON. It exists to cross-check
// the Go rewrite's hardware detection against a real Windows host: the
// dev environment is WSL, which can't call WMI/SetupAPI directly, but
// can execute a GOOS=windows binary through its PE interop and get
// real answers from the actual machine. See go/scripts/test-windows.sh.
package main

import (
	"encoding/json"
	"os"

	"sdio/internal/hardware"
)

type result struct {
	BaseBoard      *hardware.BaseBoard `json:"base_board,omitempty"`
	BaseBoardError string              `json:"base_board_error,omitempty"`
}

func main() {
	var r result

	bb, err := hardware.GetBaseBoard()
	if err != nil {
		r.BaseBoardError = err.Error()
	} else {
		r.BaseBoard = &bb
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		os.Exit(1)
	}
}
