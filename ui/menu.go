package ui

import (
	"claude-squad/keys"
	"strings"

	"claude-squad/session"

	"github.com/charmbracelet/lipgloss"
)

var keyStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#655F5F",
	Dark:  "#7F7A7A",
})

var descStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#7A7474",
	Dark:  "#9C9494",
})

var sepStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#DDDADA",
	Dark:  "#3C3C3C",
})

var actionGroupStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))

var separator = " • "
var verticalSeparator = " │ "

var menuStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("205"))

// MenuState represents different states the menu can be in
type MenuState int

const (
	StateDefault MenuState = iota
	StateEmpty
	StateNewInstance
	StatePrompt
)

// menuGroup is a run of menu options; groups are separated by a vertical bar.
type menuGroup struct {
	options []keys.KeyName
	// action groups are highlighted.
	action bool
}

type Menu struct {
	groups        []menuGroup
	height, width int
	state         MenuState
	instance      *session.Instance
	activeTab     int

	// keyDown is the key which is pressed. The default is -1.
	keyDown keys.KeyName
}

// systemOptions end every menu that isn't asking for input.
var systemOptions = []keys.KeyName{keys.KeyPrune, keys.KeyHelp, keys.KeyQuit}

var defaultMenuGroups = []menuGroup{
	{options: []keys.KeyName{keys.KeyNew, keys.KeyPrompt}, action: true},
	{options: systemOptions},
}
var newInstanceMenuGroups = []menuGroup{{options: []keys.KeyName{keys.KeySubmitName}}}
var promptMenuGroups = []menuGroup{{options: []keys.KeyName{keys.KeySubmitName}}}

func NewMenu() *Menu {
	return &Menu{
		groups:    defaultMenuGroups,
		state:     StateEmpty,
		activeTab: 0,
		keyDown:   -1,
	}
}

func (m *Menu) Keydown(name keys.KeyName) {
	m.keyDown = name
}

func (m *Menu) ClearKeydown() {
	m.keyDown = -1
}

// SetState updates the menu state and options accordingly
func (m *Menu) SetState(state MenuState) {
	m.state = state
	m.updateOptions()
}

// SetInstance updates the current instance and refreshes menu options
func (m *Menu) SetInstance(instance *session.Instance) {
	m.instance = instance
	// Only change the state if we're not in a special state (NewInstance or Prompt)
	if m.state != StateNewInstance && m.state != StatePrompt {
		if m.instance != nil {
			m.state = StateDefault
		} else {
			m.state = StateEmpty
		}
	}
	m.updateOptions()
}

// SetActiveTab updates the currently active tab
func (m *Menu) SetActiveTab(tab int) {
	m.activeTab = tab
	m.updateOptions()
}

// updateOptions updates the menu options based on current state and instance
func (m *Menu) updateOptions() {
	switch m.state {
	case StateEmpty:
		m.groups = defaultMenuGroups
	case StateDefault:
		if m.instance != nil {
			// When there is an instance, show that instance's options
			m.addInstanceOptions()
		} else {
			// When there is no instance, show the empty state
			m.groups = defaultMenuGroups
		}
	case StateNewInstance:
		m.groups = newInstanceMenuGroups
	case StatePrompt:
		m.groups = promptMenuGroups
	}
}

func (m *Menu) addInstanceOptions() {
	// Loading instances only get minimal options
	if m.instance != nil && m.instance.Status == session.Loading {
		m.groups = []menuGroup{
			{options: []keys.KeyName{keys.KeyNew}},
			{options: systemOptions},
		}
		return
	}

	// Instance management group
	management := []keys.KeyName{keys.KeyNew, keys.KeyKill}

	// Action group
	actions := []keys.KeyName{keys.KeyEnter, keys.KeySubmit}
	if m.instance.Status == session.Paused {
		actions = append(actions, keys.KeyResume)
	} else {
		actions = append(actions, keys.KeyCheckout)
	}

	// System group, with scrolling first when the tab supports it
	var system []keys.KeyName
	if m.activeTab == DiffTab || m.activeTab == TerminalTab {
		system = append(system, keys.KeyShiftUp)
	}
	system = append(system, keys.KeyTab)
	system = append(system, systemOptions...)

	m.groups = []menuGroup{
		{options: management},
		{options: actions, action: true},
		{options: system},
	}
}

// SetSize sets the width of the window. The menu will be centered horizontally within this width.
func (m *Menu) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *Menu) String() string {
	var s strings.Builder

	for g, group := range m.groups {
		if g > 0 {
			s.WriteString(sepStyle.Render(verticalSeparator))
		}
		for i, k := range group.options {
			if i > 0 {
				s.WriteString(sepStyle.Render(separator))
			}
			binding := keys.GlobalkeyBindings[k]

			localKeyStyle, localDescStyle := keyStyle, descStyle
			if group.action {
				localKeyStyle, localDescStyle = actionGroupStyle, actionGroupStyle
			}
			if m.keyDown == k {
				localKeyStyle = localKeyStyle.Underline(true)
				localDescStyle = localDescStyle.Underline(true)
			}
			s.WriteString(localKeyStyle.Render(binding.Help().Key))
			s.WriteString(" ")
			s.WriteString(localDescStyle.Render(binding.Help().Desc))
		}
	}

	centeredMenuText := menuStyle.Render(s.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, centeredMenuText)
}
