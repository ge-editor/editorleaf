package editorleaf

import (
	"github.com/ge-editor/gecore/excommand"
)

var exCmd = excommand.NewExCommandRegistry()

func init() {

	exCmd.Register(&excommand.ExCommand{
		Name:        "split",
		Description: "Split editor",
		Children: []*excommand.ExCommand{
			{
				Name: "vertical",
				Run: func(ctx excommand.Context, args []string) error {
					return nil
					// return SplitVertical(ctx)
				},
			},
			{
				Name: "horizontal",
				Run: func(ctx excommand.Context, args []string) error {
					return nil
					// return SplitHorizontal(ctx)
				},
			},
		},
	})

}
