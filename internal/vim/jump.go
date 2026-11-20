package vim

import (
	"github.com/asimmons91/pico-editor/internal/editor"
)

const jumpListLimit = 100

type jumpEntry struct {
	Path string
	Pos  editor.Pos
}

type jumpList struct {
	back    []jumpEntry
	forward []jumpEntry
}

// recordJump pushes the cursor before a jump. A new jump truncates the forward
// branch, matching Vim's per-session jump list.
func (v *Vim) recordJump(ed *editor.Editor) {
	if ed == nil {
		return
	}
	v.jumps.back = append(v.jumps.back, jumpEntry{Path: ed.Buffer.Path, Pos: ed.Cursor.Pos})
	v.jumps.back = trimJumps(v.jumps.back)
	v.jumps.forward = nil
}

func trimJumps(entries []jumpEntry) []jumpEntry {
	if len(entries) > jumpListLimit {
		return entries[len(entries)-jumpListLimit:]
	}
	return entries
}

func (v *Vim) jump(ed *editor.Editor, delta int) {
	current := jumpEntry{Path: ed.Buffer.Path, Pos: ed.Cursor.Pos}
	var entry jumpEntry
	if delta < 0 {
		if len(v.jumps.back) == 0 {
			v.ringBell()
			return
		}
		entry = v.jumps.back[len(v.jumps.back)-1]
		v.jumps.back = v.jumps.back[:len(v.jumps.back)-1]
		v.jumps.forward = append(v.jumps.forward, current)
		v.jumps.forward = trimJumps(v.jumps.forward)
	} else {
		if len(v.jumps.forward) == 0 {
			v.ringBell()
			return
		}
		entry = v.jumps.forward[len(v.jumps.forward)-1]
		v.jumps.forward = v.jumps.forward[:len(v.jumps.forward)-1]
		v.jumps.back = append(v.jumps.back, current)
		v.jumps.back = trimJumps(v.jumps.back)
	}
	if entry.Path != "" && ed.Buffer.Path != "" && entry.Path != ed.Buffer.Path {
		// Cross-file jumps are wired to the buffer layer in M5.
		v.info("cross-file jump is not available until M5")
		v.ringBell()
		return
	}
	ed.SetCursor(entry.Pos)
}
