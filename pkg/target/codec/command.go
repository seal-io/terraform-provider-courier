package codec

import (
	"fmt"
	"strings"

	"github.com/apparentlymart/go-shquot/shquot"

	"github.com/seal-io/terraform-provider-courier/utils/strx"
)

func EncodeExecInput(platform, cmd string, args []string) string {
	fn := shquot.POSIXShell
	if strings.EqualFold(platform, "windows") {
		fn = shquot.WindowsArgv
	}

	if len(args) == 0 {
		return fn([]string{cmd})
	}

	cmds := make([]string, len(args)+1)
	cmds[0] = cmd
	copy(cmds[1:], args)
	return fn(cmds)
}

func EncodeShellInput(platform, cmd string, args []string, echo string) string {
	tail := fmt.Sprintf("; echo $?%s;\n", echo)
	if strings.EqualFold(platform, "windows") {
		tail = fmt.Sprintf("; Write-Output $?%s`r`n\n", echo)
	}

	if len(args) == 0 {
		return cmd + tail
	}

	cmds := make([]string, len(args)+1)
	cmds[0] = cmd
	copy(cmds[1:], args)

	return strx.Join(" ", cmds...) + tail
}

func DecodeShellOutput(output *string, echo string) (bool, error) {
	if !strings.HasSuffix(*output, echo) {
		return false, nil
	}

	code := (*output)[:len(*output)-len(echo)]
	if code == "0" {
		return true, nil
	}

	return true, fmt.Errorf("exit code %s", code)
}
