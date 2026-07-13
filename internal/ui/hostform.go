package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bvanhorn/exfil/internal/config"
	"github.com/bvanhorn/exfil/internal/i18n"
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
	Width       int
	Height      int
}

func NewHostFormPane() *HostFormPane {
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
func (hf *HostFormPane) buildHost(loc *i18n.Localizer) (config.Host, error) {
	name := strings.TrimSpace(hf.inputs[fieldName].Value())
	hostname := strings.TrimSpace(hf.inputs[fieldHostname].Value())
	user := strings.TrimSpace(hf.inputs[fieldUser].Value())
	portStr := strings.TrimSpace(hf.inputs[fieldPort].Value())
	remotePath := strings.TrimSpace(hf.inputs[fieldRemotePath].Value())

	if name == "" {
		return config.Host{}, fmt.Errorf("%s", loc.T("err_name_required"))
	}
	if hostname == "" {
		return config.Host{}, fmt.Errorf("%s", loc.T("err_hostname_required"))
	}
	if user == "" {
		return config.Host{}, fmt.Errorf("%s", loc.T("err_user_required"))
	}

	port := config.DefaultPort()
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil || p <= 0 || p > 65535 {
			return config.Host{}, fmt.Errorf("%s", loc.T("err_port_invalid"))
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
func (hf *HostFormPane) Save(loc *i18n.Localizer) (config.Host, error) {
	host, err := hf.buildHost(loc)
	if err != nil {
		hf.errMsg = err.Error()
		return config.Host{}, err
	}

	cfg, err := config.Load()
	if err != nil {
		hf.errMsg = loc.T("err_config_load", err)
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
		hf.errMsg = loc.T("err_config_save", err)
		return config.Host{}, err
	}

	hf.errMsg = ""
	return host, nil
}

func (hf *HostFormPane) View(theme Theme, loc *i18n.Localizer) string {
	labelKeys := []string{"host_label_name", "host_label_hostname", "host_label_port", "host_label_user", "host_label_remotepath"}

	header := loc.T("hostform_header_add")
	if hf.IsEditing() {
		header = loc.T("hostform_header_edit")
	}

	lines := []string{
		gradientText(header, theme.PrimaryColor, theme.SecondaryColor),
		"",
	}

	for f := hostFormField(0); f < fieldCount; f++ {
		labelStyle := theme.BrowserFile
		if f == hf.focused {
			labelStyle = theme.BrowserDir
		}
		label := labelStyle.Render(fmt.Sprintf("%-12s", loc.T(labelKeys[f])))
		lines = append(lines, label+" "+hf.inputs[f].View())
	}

	if hf.errMsg != "" {
		lines = append(lines, "", theme.StatusError.Render(loc.T("error_prefix")+hf.errMsg))
	}

	content := strings.Join(lines, "\n")
	return gradientBox(content, hf.Width, hf.Height-2, theme.PrimaryColor, theme.SecondaryColor)
}
