package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// shellFunctions maps a shell to a `claude` function that starts claude through
// `switchyard run`. Only a plain start (no arguments, or options first) goes through
// switchyard; `claude mcp ...`, `claude auth ...` and the other subcommands reach the
// real claude unchanged.
var shellFunctions = map[string]string{
	"bash": `claude() {
  if [ $# -eq 0 ] || [ "${1#-}" != "$1" ]; then
    switchyard run -- "$@"
  else
    command claude "$@"
  fi
}`,
	"zsh": `claude() {
  if [ $# -eq 0 ] || [ "${1#-}" != "$1" ]; then
    switchyard run -- "$@"
  else
    command claude "$@"
  fi
}`,
	"fish": `function claude
    if test (count $argv) -eq 0; or string match -q -- '-*' $argv[1]
        switchyard run -- $argv
    else
        command claude $argv
    end
end`,
	"powershell": `function claude {
    if ($args.Count -eq 0 -or "$($args[0])".StartsWith('-')) {
        switchyard run -- @args
    } else {
        & (Get-Command claude -CommandType Application | Select-Object -First 1).Source @args
    }
}`,
}

func shellNames() []string {
	names := make([]string, 0, len(shellFunctions))
	for name := range shellFunctions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func newShellInitCmd() *cobra.Command {
	names := strings.Join(shellNames(), "|")
	return &cobra.Command{
		Use:   "shell-init <" + names + ">",
		Short: "Print a shell function that starts claude through switchyard",
		Long: "Print a function named claude for your shell, so that typing claude starts it through\n" +
			"switchyard run and the failover is always on. A plain start (no arguments, or options such\n" +
			"as --model first) goes through switchyard; subcommands such as claude mcp or claude auth go\n" +
			"to the real claude. Add it to your shell's startup file:\n\n" +
			"  bash, zsh   eval \"$(switchyard shell-init bash)\"\n" +
			"  fish        switchyard shell-init fish | source\n" +
			"  powershell  switchyard shell-init powershell | Out-String | Invoke-Expression\n\n" +
			"A prompt on its own (claude \"fix the bug\") is taken for a subcommand and goes to the real\n" +
			"claude; use switchyard run -- \"fix the bug\" for that.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			function, ok := shellFunctions[strings.ToLower(args[0])]
			if !ok {
				return fmt.Errorf("unknown shell %q, use one of: %s", args[0], names)
			}
			println(cmd, function)
			return nil
		},
	}
}
