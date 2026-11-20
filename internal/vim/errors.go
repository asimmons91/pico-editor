package vim

import "errors"

// ErrInvalidPattern reports a Vim pattern that cannot be translated to a Go
// regular expression.
var ErrInvalidPattern = errors.New("invalid pattern")

const (
	msgNoWriteSinceChange = "E37: No write since last change (add ! to override)"
	msgPatternNotFound    = "E486: Pattern not found"
	msgNoFileName         = "E32: No file name"
	msgNoPreviousCommand  = "E34: No previous command"
	msgNotEditorCommand   = "E492: Not an editor command"
	msgAmbiguousCommand   = "E464: Ambiguous use of user-defined command"
	msgCantOpenFile       = "E484: Can't open file"
	msgNoOtherBuffer      = "E85: There is no other file to edit"
	msgNoBuffer           = "E86: Buffer does not exist"
	msgSingleTab          = "only one code tab is available until M4"
)
