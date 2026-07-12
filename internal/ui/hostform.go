package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bvanhorn/exfil/internal/config"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// hostFormField indexes the fields in the form, in tab order.
type hostFormField int

const (
	fieldName hostFormField = iota
	fieldHostname
	fieldPort
	fieldUser
	fieldRemotePath
	fieldCount
)

// HostFormPane is the Add/Edit Host screen. It holds one textinput.Model per
// field and tracks which host (if any) is being edited.
type HostFormPane struct {
	inputs      []textinput.Model
	focused     hostFormField
	editingName string // original Name of the host being edited; empty when adding
	isEditing   bool
	errMsg      string
	theme       Theme
	Width       int
	Height      int
}

func NewHostFormPane(theme Theme) *HostFormPane {
	labels := map[hostFormField]string{
		fieldName:       "Name",
		fieldHostname:   "Hostname",
		fieldPort:       "Port",
		fieldUser:       "User",
		fieldRemotePath: "Remote Path",
	}

	inputs := make([]textinput.Model, fieldCount)
	for f := hostFormField(0); f < fieldCount; f++ {
		ti := textinput.New()
		ti.Prompt = ""
		ti.Placeholder = labels[f]
		ti.CharLimit = 256
		inputs[f] = ti
	}

	return &HostFormPane{
		inputs: inputs,
		theme:  theme,
	}
}

// ResetForAdd clears all fields for adding a brand new host.
func (hf *HostFormPane) ResetForAdd() {
	for f := hostFormField(0); f < fieldCount; f++ {
		hf.inputs[f].SetValue("")
	}
	hf.inputs[fieldPort].SetValue(strconv.Itoa(config.DefaultPort()))
	hf.isEditing = false
	hf.editingName = ""
	hf.errMsg = ""
	hf.focused = fieldName
	hf.refreshFocus()
}

// ResetForEdit populates the form from an existing host. The host's original
// Name is remembered so Save can find it again in a freshly-loaded config,
// rather than trusting a positional index that may go stale if hosts.yaml
// changes between opening the picker and saving.
func (hf *HostFormPane) ResetForEdit(host config.Host) {
	hf.inputs[fieldName].SetValue(host.Name)
	hf.inputs[fieldHostname].SetValue(host.Hostname)
	port := host.Port
	if port == 0 {
		port = config.DefaultPort()
	}
	hf.inputs[fieldPort].SetValue(strconv.Itoa(port))
	hf.inputs[fieldUser].SetValue(host.User)
	hf.inputs[fieldRemotePath].SetValue(host.RemotePath)
	hf.isEditing = true
	hf.editingName = host.Name
	hf.errMsg = ""
	hf.focused = fieldName
	hf.refreshFocus()
}

// IsEditing reports whether the form is editing an existing host (vs adding).
func (hf *HostFormPane) IsEditing() bool {
	return hf.isEditing
}

func (hf *HostFormPane) refreshFocus() {
	for f := hostFormField(0); f < fieldCount; f++ {
		if f == hf.focused {
			hf.inputs[f].Focus()
		} else {
			hf.inputs[f].Blur()
		}
	}
}

// NextField moves focus to the next field, wrapping around.
func (hf *HostFormPane) NextField() {
	hf.focused = (hf.focused + 1) % fieldCount
	hf.refreshFocus()
}

// PrevField moves focus to the previous field, wrapping around.
func (hf *HostFormPane) PrevField() {
	hf.focused = (hf.focused - 1 + fieldCount) % fieldCount
	hf.refreshFocus()
}

// HandleKey forwards a key message to the focused input.
func (hf *HostFormPane) HandleKey(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	hf.inputs[hf.focused], cmd = hf.inputs[hf.focused].Update(msg)
	return cmd
}

// buildHost validates the form fields and returns the resulting Host.
func (hf *HostFormPane) buildHost() (config.Host, error) {
	name := strings.TrimSpace(hf.inputs[fieldName].Value())
	hostname := strings.TrimSpace(hf.inputs[fieldHostname].Value())
	user := strings.TrimSpace(hf.inputs[fieldUser].Value())
	portStr := strings.TrimSpace(hf.inputs[fieldPort].Value())
	remotePath := strings.TrimSpace(hf.inputs[fieldRemotePath].Value())

	if name == "" {
		return config.Host{}, fmt.Errorf("name is required")
	}
	if hostname == "" {
		return config.Host{}, fmt.Errorf("hostname is required")
	}
	if user == "" {
		return config.Host{}, fmt.Errorf("user is required")
	}

	port := config.DefaultPort()
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil || p <= 0 || p > 65535 {
			return config.Host{}, fmt.Errorf("port must be a number between 1 and 65535")
		}
		port = p
	}

	return config.Host{
		Name:       name,
		Hostname:   hostname,
		Port:       port,
		User:       user,
		RemotePath: remotePath,
	}, nil
}

// Save validates the form, then loads hosts.yaml, adds or updates the host,
// and writes it back. Returns the saved host on success.
func (hf *HostFormPane) Save() (config.Host, error) {
	host, err := hf.buildHost()
	if err != nil {
		hf.errMsg = err.Error()
		return config.Host{}, err
	}

	cfg, err := config.Load()
	if err != nil {
		hf.errMsg = fmt.Sprintf("failed to load hosts.yaml: %v", err)
		return config.Host{}, err
	}

	replaced := false
	if hf.IsEditing() {
		for i := range cfg.Hosts {
			if cfg.Hosts[i].Name == hf.editingName {
				cfg.Hosts[i] = host
				replaced = true
				break
			}
		}
	}
	if !replaced {
		cfg.Hosts = append(cfg.Hosts, host)
	}

	if err := cfg.Save(); err != nil {
		hf.errMsg = fmt.Sprintf("failed to save hosts.yaml: %v", err)
		return config.Host{}, err
	}

	hf.errMsg = ""
	return host, nil
}

func (hf *HostFormPane) View() string {
	labels := []string{"Name", "Hostname", "Port", "User", "Remote Path"}

	title := "Add Host"
	if hf.IsEditing() {
		title = "Edit Host"
	}

	lines := []string{
		hf.theme.PaneTitle.Render(fmt.Sprintf(" %s - [Tab/Shift+Tab] move  [Enter] save  [Esc] cancel ", title)),
		"",
	}

	for f := hostFormField(0); f < fieldCount; f++ {
		labelStyle := hf.theme.BrowserFile
		if f == hf.focused {
			labelStyle = hf.theme.BrowserDir
		}
		label := labelStyle.Render(fmt.Sprintf("%-12s", labels[f]))
		lines = append(lines, label+" "+hf.inputs[f].View())
	}

	if hf.errMsg != "" {
		lines = append(lines, "", hf.theme.StatusError.Render("Error: "+hf.errMsg))
	}

	return strings.Join(lines, "\n")
}
