package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/keygen"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"

	"github.com/abhigyan-mohanta/system/internal/store"
)

type authState string

const (
	authLogin    authState = "login"
	authRegister authState = "register"
	authMain     authState = "main"
	authSettings authState = "settings"
)

type model struct {
	authState authState
	renderer  *lipgloss.Renderer

	// Login/register form
	loginUsername string
	loginPassword string
	loginFocus    int // 0 = username, 1 = password
	authError     string

	// Main app (when logged in)
	userData    *store.UserData
	cursor      int
	addingHabit *string
	lastToast   string // "Quest complete!", "Level Up!", etc. — cleared on next key

	// Settings
	settingsResetHour int  // Temporary value while editing
	settingsSaved     bool // Show save confirmation
}

func initialModel(sess ssh.Session) model {
	r := bubbletea.MakeRenderer(sess)
	return model{
		authState:     authLogin,
		renderer:      r,
		loginUsername: "",
		loginPassword: "",
		loginFocus:    0,
		authError:     "",
		userData:      nil,
		cursor:        0,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Login or register form
	if m.authState == authLogin || m.authState == authRegister {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "ctrl+c", "q":
				if m.authState == authRegister {
					m.authState = authLogin
					m.authError = ""
					m.loginUsername = ""
					m.loginPassword = ""
					m.loginFocus = 0
					return m, nil
				}
				return m, tea.Quit
			case "esc":
				if m.authState == authRegister {
					m.authState = authLogin
					m.authError = ""
					m.loginUsername = ""
					m.loginPassword = ""
					m.loginFocus = 0
				}
				return m, nil
			case "tab", "enter":
				if msg.String() == "enter" && m.loginFocus == 1 {
					// Submit
					m.authError = ""
					if m.authState == authLogin {
						u, err := store.AuthUser(m.loginUsername, m.loginPassword)
						if err != nil {
							m.authError = err.Error()
							return m, nil
						}
						m.userData = u
						m.authState = authMain
						m.loginPassword = ""
					} else {
						u, err := store.CreateUser(m.loginUsername, m.loginPassword)
						if err != nil {
							m.authError = err.Error()
							return m, nil
						}
						m.userData = u
						m.authState = authMain
						m.loginUsername = ""
						m.loginPassword = ""
					}
					return m, nil
				}
				m.loginFocus = 1 - m.loginFocus
				return m, nil
			case "backspace":
				if m.loginFocus == 0 && len(m.loginUsername) > 0 {
					m.loginUsername = m.loginUsername[:len(m.loginUsername)-1]
				}
				if m.loginFocus == 1 && len(m.loginPassword) > 0 {
					m.loginPassword = m.loginPassword[:len(m.loginPassword)-1]
				}
				return m, nil
			case "r":
				if m.authState == authLogin {
					m.authState = authRegister
					m.authError = ""
					return m, nil
				}
				fallthrough
			default:
				if len(msg.String()) == 1 && msg.Type == tea.KeyRunes {
					if m.loginFocus == 0 {
						m.loginUsername += msg.String()
					} else {
						m.loginPassword += msg.String()
					}
				}
				return m, nil
			}
		}
		return m, nil
	}

	// Settings view
	if m.authState == authSettings {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "esc":
				// Cancel and return to main
				m.authState = authMain
				m.settingsSaved = false
				return m, nil
			case "enter":
				// Save and return to main
				if err := m.userData.UpdateDayResetHour(m.settingsResetHour); err == nil {
					_ = store.SaveUser(m.userData)
					m.settingsSaved = true
					m.lastToast = "Settings saved!"
				}
				m.authState = authMain
				return m, nil
			case "up", "k":
				// Increment hour with wraparound
				m.settingsResetHour++
				if m.settingsResetHour > 23 {
					m.settingsResetHour = 0
				}
				return m, nil
			case "down", "j":
				// Decrement hour with wraparound
				m.settingsResetHour--
				if m.settingsResetHour < 0 {
					m.settingsResetHour = 23
				}
				return m, nil
			}
		}
		return m, nil
	}

	// Main app
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.addingHabit != nil {
			switch msg.String() {
			case "enter":
				name := strings.TrimSpace(*m.addingHabit)
				if name != "" {
					m.userData.AddHabit(name)
					_ = store.SaveUser(m.userData)
				}
				m.addingHabit = nil
				return m, nil
			case "esc":
				m.addingHabit = nil
				return m, nil
			case "backspace":
				if len(*m.addingHabit) > 0 {
					s := (*m.addingHabit)[:len(*m.addingHabit)-1]
					m.addingHabit = &s
				}
				return m, nil
			default:
				if len(msg.String()) == 1 && msg.Type == tea.KeyRunes {
					s := *m.addingHabit + msg.String()
					m.addingHabit = &s
				}
				return m, nil
			}
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			m.lastToast = ""
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			m.lastToast = ""
			if m.cursor < len(m.userData.Habits)-1 {
				m.cursor++
			}
		case " ":
			if len(m.userData.Habits) > 0 && m.cursor >= 0 && m.cursor < len(m.userData.Habits) {
				h := m.userData.Habits[m.cursor]
				levelBefore := m.userData.Level
				gainedEXP := m.userData.ToggleToday(h.ID)
				_ = store.SaveUser(m.userData)
				if gainedEXP {
					if m.userData.Level > levelBefore {
						m.lastToast = "DING! You have leveled up."
					} else {
						m.lastToast = "The conditions have been met. +10 EXP"
					}
				} else {
					m.lastToast = ""
				}
			}
		case "a":
			m.lastToast = ""
			s := ""
			m.addingHabit = &s
		case "d", "x":
			m.lastToast = ""
			if len(m.userData.Habits) > 0 && m.cursor >= 0 && m.cursor < len(m.userData.Habits) {
				m.userData.RemoveHabit(m.cursor)
				if m.cursor >= len(m.userData.Habits) {
					m.cursor = len(m.userData.Habits) - 1
				}
				if m.cursor < 0 {
					m.cursor = 0
				}
				_ = store.SaveUser(m.userData)
			}
		case "s":
			// Open settings
			m.lastToast = ""
			m.settingsResetHour = m.userData.DayResetHour
			m.settingsSaved = false
			m.authState = authSettings
		}
	}

	return m, nil
}

// renderTimeBar creates a progress bar showing time until next reset
func renderTimeBar(timeUntil time.Duration, accent, dim, reward lipgloss.Style) string {
	totalHours := 24.0
	hoursLeft := timeUntil.Hours()
	minutesLeft := int(timeUntil.Minutes()) % 60

	// Calculate progress (0 to 24 blocks)
	barWidth := 24
	filledBlocks := int((hoursLeft / totalHours) * float64(barWidth))
	if filledBlocks < 0 {
		filledBlocks = 0
	}
	if filledBlocks > barWidth {
		filledBlocks = barWidth
	}

	bar := strings.Repeat("█", filledBlocks) + strings.Repeat("░", barWidth-filledBlocks)
	timeStr := fmt.Sprintf("%dh %dm until reset", int(hoursLeft), minutesLeft)

	return accent.Render("Time ") + dim.Render("[") + reward.Render(bar) + dim.Render("] ") + dim.Render(timeStr)
}

// Solo Leveling–inspired colors (system blue, gold rewards, dim text)
func soloStyles(r *lipgloss.Renderer) (systemTitle, accent, dim, reward, errStyle, toastStyle lipgloss.Style, boxBorder lipgloss.Style) {
	systemBlue := lipgloss.Color("39") // bright blue
	dimGray := lipgloss.Color("245")
	gold := lipgloss.Color("220")
	red := lipgloss.Color("203")
	white := lipgloss.Color("255")
	systemTitle = r.NewStyle().Bold(true).Foreground(systemBlue)
	accent = r.NewStyle().Foreground(systemBlue)
	dim = r.NewStyle().Foreground(dimGray)
	reward = r.NewStyle().Bold(true).Foreground(gold)
	errStyle = r.NewStyle().Foreground(red)
	toastStyle = r.NewStyle().Bold(true).Foreground(gold).Padding(0, 1)
	boxBorder = r.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(systemBlue).
		Padding(0, 2)
	_ = white
	return
}

// Stats derived from level (Solo Leveling style: STR, VIT, AGI, INT)
func statsFromLevel(level int) (str, vit, agi, intel int) {
	base := 10
	if level <= 0 {
		level = 1
	}
	return base + level, base + level, base + level, base + level
}

// Dynamic box drawing: innerWidth is the width of the interior (dashes in top/bottom).
// boxLine uses "│ " + content + pad + " │", so interior = 1 + contentWidth + pad + 1.
// We need innerWidth >= contentWidth + 2 (the two spaces). So set innerWidth = maxContentWidth + 2.
const (
	boxMargin       = "  "
	boxMinInner     = 36
	boxPaddingRunes = 2 // two spaces inside each line (after │ and before │)
)

func boxTop(innerWidth int) string {
	if innerWidth < 2 {
		innerWidth = boxMinInner
	}
	return boxMargin + "┌" + strings.Repeat("─", innerWidth) + "┐"
}

func boxBottom(innerWidth int) string {
	if innerWidth < 2 {
		innerWidth = boxMinInner
	}
	return boxMargin + "└" + strings.Repeat("─", innerWidth) + "┘"
}

// boxLine renders one line; content is already styled (may include ANSI). lipgloss.Width strips ANSI.
func boxLine(content string, innerWidth int, accentStyle lipgloss.Style) string {
	if innerWidth < 2 {
		innerWidth = boxMinInner
	}
	w := lipgloss.Width(content)
	pad := innerWidth - 2 - w // one space after │, one before │
	if pad < 0 {
		pad = 0
	}
	return boxMargin + accentStyle.Render("│ ") + content + strings.Repeat(" ", pad) + accentStyle.Render(" │")
}

const (
	maxQuestNameRunes = 32 // truncate long names so full line fits in box
	maxQuestBoxWidth  = 56 // cap Daily Quests box width
)

// truncateQuestName shortens name to max runes and appends "…" if truncated.
func truncateQuestName(name string, maxRunes int) string {
	runes := []rune(name)
	if len(runes) <= maxRunes {
		return name
	}
	return string(runes[:maxRunes]) + "…"
}

func (m model) View() string {
	r := m.renderer
	titleStyle, accent, dim, reward, errStyle, toastStyle, boxBorder := soloStyles(r)
	systemTitle := func(s string) string { return titleStyle.Render(s) }

	// Login screen — "Identify yourself."
	if m.authState == authLogin {
		var b strings.Builder
		b.WriteString(systemTitle("◆  S Y S T E M"))
		b.WriteString(dim.Render("  —  Identify yourself."))
		b.WriteString("\n\n")
		b.WriteString(accent.Render("  Username  ") + dim.Render("› ") + m.loginUsername + "_")
		b.WriteString("\n")
		b.WriteString(accent.Render("  Password  ") + dim.Render("› ") + strings.Repeat("•", len(m.loginPassword)) + "_")
		b.WriteString("\n\n")
		if m.authError != "" {
			b.WriteString(errStyle.Render("  ⚠ "+m.authError) + "\n\n")
		}
		b.WriteString(dim.Render("  [Tab] next  [Enter] login  [r] register  [q] quit"))
		return boxBorder.Render(b.String())
	}

	// Register screen — "Register as a new Hunter."
	if m.authState == authRegister {
		var b strings.Builder
		b.WriteString(systemTitle("◆  S Y S T E M"))
		b.WriteString(dim.Render("  —  Register as a new Hunter."))
		b.WriteString("\n\n")
		b.WriteString(accent.Render("  Username  ") + dim.Render("› ") + m.loginUsername + "_")
		b.WriteString("\n")
		b.WriteString(accent.Render("  Password  ") + dim.Render("› ") + strings.Repeat("•", len(m.loginPassword)) + "_")
		b.WriteString("\n\n")
		if m.authError != "" {
			b.WriteString(errStyle.Render("  ⚠ "+m.authError) + "\n\n")
		}
		b.WriteString(dim.Render("  [Tab] next  [Enter] create  [Esc] back  [q] quit"))
		return boxBorder.Render(b.String())
	}

	// Settings view
	if m.authState == authSettings {
		var b strings.Builder
		b.WriteString(systemTitle("◆  S Y S T E M"))
		b.WriteString(dim.Render("  —  Settings"))
		b.WriteString("\n\n")
		b.WriteString(accent.Render("  Day Reset Time Configuration"))
		b.WriteString("\n\n")
		b.WriteString(dim.Render("  Your daily quests will reset at this hour each day."))
		b.WriteString("\n")
		b.WriteString(dim.Render("  This allows you to customize based on your timezone."))
		b.WriteString("\n\n")

		// Display current hour with up/down arrows
		hourStr := fmt.Sprintf("%02d:00", m.settingsResetHour)
		b.WriteString("  " + dim.Render("▲") + "\n")
		b.WriteString("  " + accent.Render("Reset Hour: ") + reward.Render(hourStr) + "\n")
		b.WriteString("  " + dim.Render("▼") + "\n\n")

		b.WriteString(dim.Render("  Use [") + accent.Render("↑") + dim.Render("/") + accent.Render("k") + dim.Render("] and [") + accent.Render("↓") + dim.Render("/") + accent.Render("j") + dim.Render("] to adjust"))
		b.WriteString("\n")
		b.WriteString(dim.Render("  [Enter] save  [Esc] cancel  [q] quit"))
		return boxBorder.Render(b.String())
	}

	// Main app: loading
	if m.userData == nil {
		return boxBorder.Render(systemTitle("◆  S Y S T E M") + "\n\n" + dim.Render("  Loading..."))
	}

	// Main app: new daily quest prompt
	if m.addingHabit != nil {
		var b strings.Builder
		b.WriteString(systemTitle("◆  S Y S T E M"))
		b.WriteString(dim.Render("  —  New Daily Quest"))
		b.WriteString("\n\n")
		b.WriteString(accent.Render("  Quest name  ") + dim.Render("› ") + *m.addingHabit + "_")
		b.WriteString("\n\n")
		b.WriteString(dim.Render("  [Enter] accept  [Esc] cancel"))
		return boxBorder.Render(b.String())
	}

	// Main app: daily quests + stats
	u := m.userData
	expNeed := u.EXPForNextLevel()
	expIn := u.EXPInCurrentLevel()
	expPct := 0
	if expNeed > 0 {
		expPct = (expIn * 24) / expNeed
		if expPct > 24 {
			expPct = 24
		}
	}
	expBar := strings.Repeat("█", expPct) + strings.Repeat("░", 24-expPct)
	str, vit, agi, intel := statsFromLevel(u.Level)

	var b strings.Builder
	b.WriteString(systemTitle("◆  S Y S T E M"))
	b.WriteString(dim.Render("  —  Daily Quests"))
	b.WriteString(dim.Render("  │  Hunter: ") + accent.Render(u.Username))
	b.WriteString("\n")
	b.WriteString(dim.Render("  Complete your daily quests to level up."))
	b.WriteString("\n\n")

	// Stats panel (Solo Leveling style) — dynamic box
	statusLine1 := accent.Render("Level ") + reward.Render(fmt.Sprintf("%d", u.Level)) +
		accent.Render("   STR ") + reward.Render(fmt.Sprintf("%d", str)) +
		accent.Render("  VIT ") + reward.Render(fmt.Sprintf("%d", vit)) +
		accent.Render("  AGI ") + reward.Render(fmt.Sprintf("%d", agi)) +
		accent.Render("  INT ") + reward.Render(fmt.Sprintf("%d", intel))
	statusLine2 := accent.Render("EXP  ") + dim.Render("[") + reward.Render(expBar) + dim.Render("] ") +
		reward.Render(fmt.Sprintf("%d/100", expIn))
	// Add time bar
	timeUntil := u.TimeUntilReset()
	timeBarLine := renderTimeBar(timeUntil, accent, dim, reward)

	// Calculate box width from all lines
	statusInner := lipgloss.Width(statusLine1)
	if w2 := lipgloss.Width(statusLine2); w2 > statusInner {
		statusInner = w2
	}
	if w3 := lipgloss.Width(timeBarLine); w3 > statusInner {
		statusInner = w3
	}
	statusInner += boxPaddingRunes
	if statusInner < boxMinInner {
		statusInner = boxMinInner
	}
	b.WriteString(accent.Render(boxTop(statusInner)) + "\n")
	b.WriteString(accent.Render(boxLine(accent.Render("Status"), statusInner, accent)) + "\n")
	b.WriteString(accent.Render(boxLine(statusLine1, statusInner, accent)) + "\n")
	b.WriteString(accent.Render(boxLine(statusLine2, statusInner, accent)) + "\n")
	b.WriteString(accent.Render(boxLine(timeBarLine, statusInner, accent)) + "\n")
	b.WriteString(accent.Render(boxBottom(statusInner)) + "\n\n")

	// Toast (quest complete / level up)
	if m.lastToast != "" {
		b.WriteString(toastStyle.Render("  ▶ "+m.lastToast) + "\n\n")
	}

	// Daily Quests panel — dynamic box from content width (+ 2 for spaces inside boxLine)
	questTitle := accent.Render("Daily Quests")
	questInner := lipgloss.Width(questTitle) + boxPaddingRunes
	if questInner < boxMinInner {
		questInner = boxMinInner
	}
	if len(u.Habits) == 0 {
		emptyLine := dim.Render("No quests. Press [a] to add.")
		if w := lipgloss.Width(emptyLine) + boxPaddingRunes; w > questInner {
			questInner = w
		}
		if questInner > maxQuestBoxWidth {
			questInner = maxQuestBoxWidth
		}
		b.WriteString(accent.Render(boxTop(questInner)) + "\n")
		b.WriteString(accent.Render(boxLine(questTitle, questInner, accent)) + "\n")
		b.WriteString(accent.Render(boxLine(emptyLine, questInner, dim)) + "\n")
	} else {
		completedToday := 0
		for _, h := range u.Habits {
			if u.CompletedToday(h.ID) {
				completedToday++
			}
		}
		summaryLine := dim.Render(fmt.Sprintf("%d/%d completed today.", completedToday, len(u.Habits)))
		if w := lipgloss.Width(summaryLine) + boxPaddingRunes; w > questInner {
			questInner = w
		}
		// Build each quest line and track max width
		questLines := make([]string, 0, len(u.Habits)+2)
		questLines = append(questLines, questTitle, summaryLine)
		for i, h := range u.Habits {
			arrow := "   "
			if m.cursor == i {
				arrow = accent.Render(" ▸ ")
			}
			done := u.CompletedToday(h.ID)
			check := dim.Render("[ ]")
			if done {
				check = reward.Render("[✓]")
			}
			displayName := truncateQuestName(h.Name, maxQuestNameRunes)
			line := arrow + check + " " + displayName + "  " + dim.Render("→ ") + reward.Render(fmt.Sprintf("+%d EXP", store.EXPPerQuest))
			if w := lipgloss.Width(line) + boxPaddingRunes; w > questInner {
				questInner = w
			}
			questLines = append(questLines, line)
		}
		if questInner < boxMinInner {
			questInner = boxMinInner
		}
		if questInner > maxQuestBoxWidth {
			questInner = maxQuestBoxWidth
		}
		b.WriteString(accent.Render(boxTop(questInner)) + "\n")
		for _, line := range questLines {
			b.WriteString(accent.Render(boxLine(line, questInner, accent)) + "\n")
		}
	}
	b.WriteString(accent.Render(boxBottom(questInner)) + "\n\n")
	b.WriteString(dim.Render("  [a] add  [d] delete  [space] complete  [s] settings  [q] quit"))
	return boxBorder.Render(b.String())
}

func main() {
	hostKeyPath := "ssh_host_key"
	if _, err := os.Stat(hostKeyPath); err != nil {
		kp, err := keygen.New(hostKeyPath, keygen.WithKeyType(keygen.Ed25519), keygen.WithWrite())
		if err != nil {
			log.Fatalf("generate ssh host key: %v", err)
		}
		_ = kp
		log.Println("generated new SSH host key at", hostKeyPath)
	}
	s, err := wish.NewServer(
		wish.WithAddress(":23234"),
		wish.WithHostKeyPath(hostKeyPath),
		wish.WithMiddleware(
			logging.Middleware(),
			bubbletea.Middleware(func(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
				return initialModel(sess), []tea.ProgramOption{tea.WithAltScreen()}
			}),
		),
	)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("⚔ SYSTEM — Habit tracker listening on :23234")
	log.Println("   Connect: ssh -p 23234 user@localhost  (production: ssh system.hostagedown.com)")
	log.Println("   Then enter your username and password in the app.")
	log.Fatal(s.ListenAndServe())
}
