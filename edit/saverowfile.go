package edit

import (
	"os"

	"github.com/jmigpin/editor/ui"
)

func saveRowsFiles(ed *Editor) {
	for _, c := range ed.ui.Layout.Cols.Cols {
		for _, r := range c.Rows {
			saveRowFile2(ed, r, true)
		}
	}
}
func saveRowFile(ed *Editor, row *ui.Row) {
	saveRowFile2(ed, row, false)
}

func saveRowFile2(ed *Editor, row *ui.Row, tolerant bool) {
	tsd := ed.rowToolbarStringData(row)
	// file might not exist yet, so getting from filepath
	filename := tsd.FirstPartFilepath()

	// best effort to disable/enable filesstates watcher, ignore errors
	_ = ed.fs.Remove(filename)
	defer func() {
		_ = ed.fs.Add(filename)
	}()

	// save
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		ed.Error(err)
		return
	}
	defer f.Close()
	data := []byte(row.TextArea.Text())
	_, err = f.Write(data)
	if err != nil {
		ed.Error(err)
		return
	}

	row.Square.SetDirty(false)
	row.Square.SetCold(false)
}
