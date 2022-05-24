// Package ui implements the UI for resetti.
package ui

import (
	"fmt"
	"resetti/mc"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	gloss "github.com/charmbracelet/lipgloss"
)

type Model struct {
	instances  []mc.Instance
	status     Status
	statusText string
}

func NewModel() Model {
	return Model{
		instances:  []mc.Instance{},
		status:     StatusUnknown,
		statusText: "",
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+r", "f5":
			return m, func() tea.Msg { return MsgReload }
		}
	case mc.Instance:
		m.instances[msg.Id] = msg
	case []mc.Instance:
		m.instances = msg
	case MsgStatus:
		m.status = msg.Status
		m.statusText = msg.Text
	}

	return m, nil
}

func (m Model) View() string {
	style := statusStyles[m.status]
	out := style.style.Render("\n  STATUS: " + style.title)
	if m.statusText != "" {
		out += style.style.Render(" | " + m.statusText)
	}
	out += "\n"

	out += cyanStyle.Render("  ID  Version  State    ")
	out += grayStyle.Render(fmt.Sprintf("%d instances\n", len(m.instances)))
	for _, i := range m.instances {
		str := "  " + pad(strconv.Itoa(i.Id), 4)
		str += pad(i.Version.String(), 9)
		str += i.State.String() + "\n"
		out += gloss.NewStyle().Render(str)
	}
	out += grayStyle.Render("\n  ctrl+c: quit    ctrl+r: reload\n\n")

	return out
}

func pad(str string, length int) string {
	toAdd := length - len(str)
	for i := 0; i < toAdd; i++ {
		str += " "
	}
	return str
}

type Status int

const (
	StatusUnknown Status = iota
	StatusBusy
	StatusOk
	StatusFail
)

type StatusStyle struct {
	title string
	style gloss.Style
}

var statusStyles = map[Status]StatusStyle{
	StatusUnknown: {
		title: "???",
		style: gloss.NewStyle().Foreground(gloss.Color("15")),
	},
	StatusBusy: {
		title: "busy",
		style: gloss.NewStyle().Foreground(gloss.Color("11")),
	},
	StatusOk: {
		title: "ok",
		style: gloss.NewStyle().Foreground(gloss.Color("10")),
	},
	StatusFail: {
		title: "fail",
		style: gloss.NewStyle().Foreground(gloss.Color("9")),
	},
}

var cyanStyle = gloss.NewStyle().Bold(true).Foreground(gloss.Color("14"))
var grayStyle = gloss.NewStyle().Foreground(gloss.Color("#aaaaaa"))
