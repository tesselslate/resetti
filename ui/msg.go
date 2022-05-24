package ui

const (
	MsgReload int = iota
)

type MsgStatus struct {
	Status Status
	Text   string
}
