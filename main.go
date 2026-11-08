package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const stageCount = 10

type Level struct {
	Title string
	Rows  []string
	SX    int
	SY    int
}

type tickMsg time.Time

type gameSpeed struct {
	Name     string
	Interval time.Duration
}

var speeds = []gameSpeed{
	{Name: "Slow", Interval: 260 * time.Millisecond},
	{Name: "Normal", Interval: 180 * time.Millisecond},
	{Name: "Fast", Interval: 110 * time.Millisecond},
}

type model struct {
	levels []Level

	level  int
	x      int
	y      int
	vy     int
	deaths int

	name   string
	input  string
	status string
	screen string

	selected     int
	speed        int
	pendingLevel int
	width        int
	height       int
	won          bool
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("6"))

	accent = lipgloss.NewStyle().
		Foreground(lipgloss.Color("3"))

	good = lipgloss.NewStyle().
		Foreground(lipgloss.Color("2"))

	danger = lipgloss.NewStyle().
		Foreground(lipgloss.Color("1"))

	wall = lipgloss.NewStyle().
		Foreground(lipgloss.Color("4"))
)

func loadLevels(path string) ([]Level, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1<<20)

	var levels []Level
	var title string
	var rows []string

	flush := func() error {
		if len(rows) == 0 {
			return nil
		}

		level := Level{
			Title: title,
			Rows:  append([]string(nil), rows...),
		}
		title = ""
		rows = nil

		starts := 0
		exits := 0
		maxWidth := 0

		for y, row := range level.Rows {
			tiles := []byte(row)
			if len(tiles) > maxWidth {
				maxWidth = len(tiles)
			}

			for x, tile := range tiles {
				switch tile {
				case '@':
					starts++
					level.SX, level.SY = x, y
					tiles[x] = ' '

				case 'E':
					exits++

				case '=', '!':
					tiles[x] = '^'

				case '#', '^', '?', ' ':
					// vaaalid tile

				default:
					return fmt.Errorf(
						"stage %d: invalid tile %q",
						len(levels)+1,
						tile,
					)
				}
			}

			level.Rows[y] = string(tiles)
		}

		if starts != 1 || exits < 1 {
			return fmt.Errorf(
				"stage %d needs exactly one @ and at least one E",
				len(levels)+1,
			)
		}

		for y, row := range level.Rows {
			if len(row) < maxWidth {
				level.Rows[y] += strings.Repeat(" ", maxWidth-len(row))
			}
		}

		if level.Title == "" {
			level.Title = fmt.Sprintf("Stage %02d", len(levels)+1)
		}

		levels = append(levels, level)
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "---":
			if err := flush(); err != nil {
				return nil, err
			}

		case strings.HasPrefix(trimmed, ";"):
			if len(rows) == 0 {
				title = strings.TrimSpace(strings.TrimPrefix(trimmed, ";"))
			}

		default:
			rows = append(rows, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(levels) != stageCount {
		return nil, fmt.Errorf(
			"expected %d stages, found %d",
			stageCount,
			len(levels),
		)
	}

	return levels, nil
}

func initial(levels []Level) model {
	return model{
		levels: levels,
		screen: "menu",
		status: "Ready",
		speed:  1,
	}
}

func (m model) nextTick() tea.Cmd {
	return tea.Tick(speeds[m.speed].Interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Init() tea.Cmd {
	return m.nextTick()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch value := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = value.Width
		m.height = value.Height

	case tickMsg:
		if m.screen == "game" && !m.won {
			m.gravity()
		}
		return m, m.nextTick()

	case tea.KeyMsg:
		key := value.String()

		if key == "ctrl+c" {
			return m, tea.Quit
		}

		if m.screen == "name" {
			m.updateName(key)
			return m, nil
		}

		if key == "q" || key == "esc" {
			if m.screen == "menu" {
				return m, tea.Quit
			}
			m.screen = "menu"
			return m, nil
		}

		switch m.screen {
		case "menu":
			switch key {
			case "up", "k":
				m.selected = (m.selected + 4) % 5
			case "down", "j":
				m.selected = (m.selected + 1) % 5
			case "enter", " ":
				switch m.selected {
				case 0:
					m.pendingLevel = 0
					m.input = ""
					m.screen = "name"
				case 1:
					m.screen = "select"
				case 2:
					m.screen = "speed"
				case 3:
					m.screen = "help"
				case 4:
					return m, tea.Quit
				}
			}

		case "speed":
			switch key {
			case "up", "left", "k", "p":
				m.speed = (m.speed + len(speeds) - 1) % len(speeds)
			case "down", "right", "j", "n":
				m.speed = (m.speed + 1) % len(speeds)
			case "enter", " ":
				m.screen = "menu"
			}

		case "help":
			if key == "enter" || key == " " {
				m.screen = "menu"
			}

		case "select":
			switch key {
			case "left", "p":
				m.level = (m.level + len(m.levels) - 1) % len(m.levels)
			case "right", "n":
				m.level = (m.level + 1) % len(m.levels)
			case "enter":
				if m.name == "" {
					m.pendingLevel = m.level
					m.input = ""
					m.screen = "name"
				} else {
					m.begin(m.level)
				}
			}

		case "game":
			switch key {
			case "left", "a":
				m.move(-1)
			case "right", "d":
				m.move(1)
			case "up", "w", " ":
				m.jump()
			case "r":
				m.begin(m.level)
			case "n":
				if m.level+1 < len(m.levels) {
					m.begin(m.level + 1)
				}
			case "p":
				if m.level > 0 {
					m.begin(m.level - 1)
				}
			}
		}
	}

	return m, nil
}

func (m *model) updateName(key string) {
	switch key {
	case "esc":
		m.screen = "menu"

	case "enter":
		if name := strings.TrimSpace(m.input); name != "" {
			m.name = name
			m.begin(m.pendingLevel)
		}

	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}

	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 &&
			len(m.input) < 20 {
			m.input += key
		}
	}
}

func (m *model) begin(index int) {
	m.level = index

	level := m.levels[index]
	m.x, m.y, m.vy = level.SX, level.SY, 0
	m.won = false
	m.screen = "game"
	m.status = "Reach E. Avoid traps."
}

func (m *model) tile(x, y int) byte {
	level := m.levels[m.level]

	if y < 0 || y >= len(level.Rows) || x < 0 {
		return '#'
	}
	if x >= len(level.Rows[y]) {
		return ' '
	}

	return level.Rows[y][x]
}

func (m *model) move(direction int) {
	if m.won {
		return
	}
	if m.tile(m.x+direction, m.y) != '#' {
		m.x += direction
		m.check()
	}
}

func (m *model) jump() {
	if m.won {
		return
	}

	below := m.tile(m.x, m.y+1)
	if m.vy == 0 && (below == '#' || below == '^') {
		m.vy = -3
	}
}

func (m *model) die() {
	m.deaths++
	m.begin(m.level)
	m.status = "Trap! Stage restarted."
}

func (m *model) check() {
	switch m.tile(m.x, m.y) {
	case '^', '?':
		m.die()
		return

	case 'E':
		if m.level+1 < len(m.levels) {
			m.begin(m.level + 1)
			m.status = "Stage clear!"
		} else {
			m.won = true
			m.status = fmt.Sprintf(
				"All %d stages cleared!",
				len(m.levels),
			)
		}
		return
	}

	if m.vy == 0 && m.tile(m.x, m.y+1) == '^' {
		m.die()
	}
}

func (m *model) gravity() {
	if m.vy < 0 {
		if m.tile(m.x, m.y-1) != '#' {
			m.y--
			m.vy++
		} else {
			m.vy = 0
		}
		m.check()
		return
	}

	below := m.tile(m.x, m.y+1)
	if below == '^' || below == '?' {
		m.die()
		return
	}
	if below == '#' {
		return
	}

	m.y++
	if m.y >= len(m.levels[m.level].Rows) {
		m.die()
		return
	}

	m.check()
}

func (m model) View() string {
	if m.width < 38 || m.height < 12 {
		return lipgloss.Place(
			m.width,
			m.height,
			lipgloss.Center,
			lipgloss.Center,
			"Terminal too small\nResize window to continue\nQ: quit",
		)
	}

	boxWidth := m.width - 4
	if boxWidth > 68 {
		boxWidth = 68
	}
	if boxWidth < 34 {
		boxWidth = m.width - 2
	}

	innerWidth := boxWidth - 4
	lines := []string{
		titleStyle.Render("💀 DIE AGAIN CLI / @EmadAsefi"),
	}

	switch m.screen {
	case "menu":
		lines = append(
			lines,
			fmt.Sprintf("%d stages", len(m.levels)),
			"",
		)

		items := []string{
			"Start game",
			"Select level",
			"Game speed: " + speeds[m.speed].Name,
			"How to play",
			"Quit",
		}
		for index, item := range items {
			if index == m.selected {
				lines = append(lines, accent.Render("> "+item))
			} else {
				lines = append(lines, "  "+item)
			}
		}
		lines = append(lines, "", "Up/Down + Enter   Q quit")

	case "name":
		lines = append(
			lines,
			"",
			"Enter player name:",
			accent.Render(m.input+"_"),
			"",
			"Type ASCII; Enter to continue",
		)

	case "speed":
		lines = append(lines, "", "GAME SPEED", "")
		for index, speed := range speeds {
			item := speed.Name
			if index == m.speed {
				lines = append(lines, accent.Render("> "+item))
			} else {
				lines = append(lines, "  "+item)
			}
		}
		lines = append(lines, "", "Up/Down to choose   Enter to return")

	case "help":
		lines = append(
			lines,
			"",
			"Reach E; avoid ^ spikes and ? decoys.",
			"Arrows/WASD move; Up/W/Space jump.",
			"R restart; N/P next/previous stage.",
			"Q/Esc returns to menu.",
			"",
			"Enter to return",
		)

	case "select":
		lines = append(
			lines,
			fmt.Sprintf("LEVEL %02d / %02d", m.level+1, len(m.levels)),
			accent.Render(m.levels[m.level].Title),
			"",
			"Left/Right or P/N select",
			"Enter to play",
		)

	case "game":
		lines = append(lines, m.gameView(innerWidth)...)
	}

	content := strings.Join(lines, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("6")).
		Padding(0, 1).
		Width(boxWidth - 2).
		Render(content)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		box,
	)
}

func (m model) gameView(innerWidth int) []string {
	level := m.levels[m.level]

	lines := []string{
		fmt.Sprintf(
			"%s | %s",
			m.name,
			accent.Render(
				fmt.Sprintf("STAGE %02d/%02d", m.level+1, len(m.levels)),
			),
		),
		fmt.Sprintf("DEATHS %d | %s", m.deaths, level.Title),
		"",
	}

	availableRows := m.height - 13
	if availableRows < 1 {
		availableRows = 1
	}

	start := 0
	if len(level.Rows) > availableRows {
		start = m.y - availableRows/2
		if start < 0 {
			start = 0
		}
		if start+availableRows > len(level.Rows) {
			start = len(level.Rows) - availableRows
		}
	}

	mapWidth := len(level.Rows[0])
	left := 0
	if mapWidth > innerWidth {
		left = m.x - innerWidth/2
		if left < 0 {
			left = 0
		}
		if left+innerWidth > mapWidth {
			left = mapWidth - innerWidth
		}
	}

	end := start + availableRows
	if end > len(level.Rows) {
		end = len(level.Rows)
	}

	for y := start; y < end; y++ {
		var row strings.Builder

		for x := left; x < left+innerWidth && x < mapWidth; x++ {
			if x == m.x && y == m.y {
				row.WriteString(accent.Render("@"))
				continue
			}

			switch tile := m.tile(x, y); tile {
			case '#':
				row.WriteString(wall.Render("#"))
			case '^':
				row.WriteString(danger.Render("^"))
			case '?':
				row.WriteString(accent.Render("?"))
			case 'E':
				row.WriteString(good.Render("E"))
			default:
				row.WriteByte(' ')
			}
		}

		lines = append(lines, row.String())
	}

	return append(
		lines,
		"",
		m.status,
		"Arrows/WASD move  Space jump",
		"R restart  N/P levels  Q menu",
	)
}

func main() {
	path := "levels.txt"

	if _, err := os.Stat(path); err != nil {
		if executable, execErr := os.Executable(); execErr == nil {
			path = filepath.Join(
				filepath.Dir(executable),
				"levels.txt",
			)
		}
	}

	levels, err := loadLevels(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	_, err = tea.NewProgram(
		initial(levels),
		tea.WithAltScreen(),
	).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
