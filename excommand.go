package editorleaf

import (
	"context"
	"strings"

	"github.com/ge-editor/gecore"
	"github.com/ge-editor/gecore/excommand"
	"github.com/ge-editor/utils"
)

var exCommands = excommand.NewRegistry()

func RegisterGlobal(cmd excommand.ExCommand) {
	exCommands.Register(cmd)
}

// Implements

type Command struct {
	CommandName        string
	CommandAliases     []string
	CommandDescription string
	Run                func(ctx context.Context, args []string) (handled bool, err error)
}

func (c *Command) Name() string        { return c.CommandName }
func (c *Command) Aliases() []string   { return c.CommandAliases }
func (c *Command) Description() string { return c.CommandDescription }

func (c *Command) Execute(ctx context.Context, args []string) (bool, error) {
	if c.Run == nil {
		return false, nil
	}
	return c.Run(ctx, args)
}

/*
func init() {

	RegisterGlobal(&Command{
		CommandName:        "test",
		CommandDescription: "Exit the editor 2",
		Run: func(ctx context.Context, args []string) (bool, error) {
			// 終了処理
			return true, nil
		},
	})

} */

////////////////////////////////

func (e *Editorleaf) Commands() []excommand.ExCommand {
	return []excommand.ExCommand{
		&Command{
			CommandName:        "test 2",
			CommandDescription: "Exit the editor 2",
			Run: func(ctx context.Context, args []string) (bool, error) {
				// 終了処理
				return true, nil
			},
		},

		// ! （shell-command / M-!）
		// ショートカット: M-! (async-shell-command は M-&)
		// 機能: Emacsのミニバッファから直接、OSのシェルコマンドを1回きりで実行するためのコマンドです。
		// 動作: M-! を押すとプロンプトが出るので、そこで ls -la や git status などのコマンドを入力して実行します。出力結果は専用のウィンドウ（*Shell Command Output*）に表示されます。
		&Command{
			CommandName:        "!",
			CommandDescription: "Shell command",
			Run: func(ctx context.Context, args []string) (bool, error) {
				result, err := utils.ExecShellCommand(strings.Join(args, " "))
				// gelog.Debug("Run shell command")
				if err != nil {
					gecore.Echo.AddText(err.Error())
					return true, err
				}
				// gecore.Echo.AddText("result", result)
				if result != "" {
					e.InsertString(result)
				}
				return true, nil
			},
		},

		// | （shell-command-on-region / M-|）
		// リージョン（選択範囲）を選択した状態で
		// 機能: バッファ内で選択した範囲（リージョン）のテキストを、外部のシェルコマンドの標準入力（stdin）として流し込むためのコマンドです。
		&Command{
			CommandName:        "|",
			CommandDescription: "Shell command oneregion",
			Run: func(ctx context.Context, args []string) (bool, error) {
				return true, nil
			},
		},
	}
}
