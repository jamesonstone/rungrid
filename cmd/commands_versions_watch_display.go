package cmd

import (
	"fmt"
	"io"

	"github.com/jamesonstone/rungrid/internal/versions"
)

const (
	versionsWatchEnter  = "\033[?1049h\033[?25l"
	versionsWatchRedraw = "\033[2J\033[H"
	versionsWatchExit   = "\033[?25h\033[?1049l"
)

type versionsWatchDisplay struct {
	writer     io.Writer
	fullScreen bool
	openScreen bool
}

func newVersionsWatchDisplay(writer io.Writer, fullScreen bool) *versionsWatchDisplay {
	return &versionsWatchDisplay{writer: writer, fullScreen: fullScreen}
}

func (d *versionsWatchDisplay) open() {
	if !d.fullScreen || d.openScreen {
		return
	}
	d.openScreen = true
	_, _ = fmt.Fprint(d.writer, versionsWatchEnter)
}

func (d *versionsWatchDisplay) render(snapshot versions.Snapshot) {
	if d.fullScreen {
		_, _ = fmt.Fprint(d.writer, versionsWatchRedraw)
	}
	versions.WriteHuman(d.writer, snapshot)
}

func (d *versionsWatchDisplay) close() {
	if !d.openScreen {
		return
	}
	_, _ = fmt.Fprint(d.writer, versionsWatchExit)
	d.openScreen = false
}
