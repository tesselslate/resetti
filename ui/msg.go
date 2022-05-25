package ui

const (
	MsgReload int = iota
)

type MsgStatus struct {
	Status Status
	Text   string
}

type Command int

const (
	CmdQuit Command = iota
	CmdRefresh
)
