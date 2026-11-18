package editorleaf

import "testing"

// ESC x passes the raw input as one element, command name included. The new
// name of a rename is what follows the name; the whole input used to be taken
// as the name.
func TestRenameNewName(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"lsp-rename-symbol NewName"}, "NewName"},
		{[]string{"lsp-rename-symbol   NewName  "}, "NewName"},
		{[]string{"LSP-RENAME-SYMBOL NewName"}, "NewName"}, // command names are matched ignoring case
		{[]string{"NewName"}, "NewName"},                   // the name alone
		{[]string{"lsp-rename-symbol"}, ""},                // no name typed
		{[]string{"lsp-rename-symbol "}, ""},
		{[]string{}, ""},
		{[]string{"lsp-rename-symbol", "NewName"}, "NewName"},
	} {
		if got := renameNewName(tc.args); got != tc.want {
			t.Errorf("renameNewName(%q) = %q, want %q", tc.args, got, tc.want)
		}
	}
}
