package cmdutil

import (
	"fmt"
	"strconv"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/spf13/cobra"
)

const (
	defaultParallelism = 1
	maxParallelism     = 16
)

// AddParallelismFlag registers the local -p/--parallelism flag for commands
// that support bounded multi-key work.
func AddParallelismFlag(cmd *cobra.Command, target *int) {
	if target == nil {
		panic("cmdutil.AddParallelismFlag: target must not be nil")
	}
	*target = defaultParallelism
	cmd.Flags().VarP(
		parallelismValue{target: target},
		"parallelism",
		"p",
		"Maximum concurrent requests (1-16)",
	)
	ExtendFlag(cmd.Flags(), "parallelism", clib.FlagExtra{
		Group:       "Execution",
		Placeholder: "N",
		Terse:       "max concurrent requests",
	})
}

type parallelismValue struct {
	target *int
}

func (v parallelismValue) String() string {
	if v.target == nil {
		return strconv.Itoa(defaultParallelism)
	}
	return strconv.Itoa(*v.target)
}

func (v parallelismValue) Set(raw string) error {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("parallelism must be numeric: %w", err)
	}
	if value < defaultParallelism || value > maxParallelism {
		return fmt.Errorf("parallelism must be between %d and %d", defaultParallelism, maxParallelism)
	}
	*v.target = value
	return nil
}

func (parallelismValue) Type() string {
	return "int"
}
