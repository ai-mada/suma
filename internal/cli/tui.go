package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

var sections = []string{"projects", "requirements", "tasks", "runs", "workflows"}

type loaded struct {
	generation int
	raw        json.RawMessage
	err        error
}
type pagePosition struct {
	cursor string
	offset int
}
type model struct {
	ctx                            context.Context
	client                         *client
	section, cursor, offset, total int
	width, height, scroll          int
	generation                     int
	loading                        bool
	items                          []map[string]any
	detailPath, detail             string
	pageCursor, nextCursor         string
	pageHistory                    []pagePosition
	err                            error
}

func newModel(ctx context.Context, c *client) model {
	copyClient := *c
	return model{ctx: ctx, client: &copyClient, width: 88, height: 24, loading: true, generation: 1}
}

func (m model) Init() tea.Cmd { return m.fetch() }
func (m model) fetch() tea.Cmd {
	c := *m.client
	path := m.detailPath
	if path == "" {
		path = "/api/v1/" + sections[m.section] + "?limit=25"
		if m.pageCursor != "" {
			path += "&cursor=" + url.QueryEscape(m.pageCursor)
		}
	}
	scoped := m.section != 0
	generation, ctx := m.generation, m.ctx
	return func() tea.Msg {
		raw, err := c.request(ctx, "GET", path, nil, scoped)
		return loaded{generation: generation, raw: raw, err: err}
	}
}

func (m model) reload() (tea.Model, tea.Cmd) {
	m.generation++
	m.loading = true
	m.err = nil
	return m, m.fetch()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case loaded:
		if msg.generation != m.generation {
			return m, nil
		}
		m.loading = false
		m.err = msg.err
		if msg.err != nil {
			m.items = nil
			m.detail = ""
			return m, nil
		}
		if m.detailPath != "" {
			var pretty strings.Builder
			var value any
			if json.Unmarshal(msg.raw, &value) == nil {
				writeJSON(&pretty, value, m.client.token)
			}
			m.detail = pretty.String()
		} else {
			var page struct {
				Items      []map[string]any `json:"items"`
				Total      int              `json:"total"`
				NextCursor string           `json:"next_cursor"`
			}
			if err := json.Unmarshal(msg.raw, &page); err != nil {
				m.err = &failure{Code: "response", Message: "invalid list response"}
				return m, nil
			}
			m.items, m.total = page.Items, page.Total
			m.nextCursor = page.NextCursor
			if m.cursor >= len(m.items) {
				m.cursor = max(0, len(m.items)-1)
			}
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab", "shift+tab":
			direction := 1
			if msg.String() == "shift+tab" {
				direction = -1
			}
			m.section = (m.section + len(sections) + direction) % len(sections)
			m.cursor = 0
			m.offset = 0
			m.pageCursor = ""
			m.nextCursor = ""
			m.pageHistory = nil
			m.detailPath = ""
			m.detail = ""
			m.scroll = 0
			m.items = nil
			return m.reload()
		case "r":
			return m.reload()
		case "esc", "backspace":
			if m.detailPath != "" {
				m.detailPath = ""
				m.detail = ""
				m.scroll = 0
				return m.reload()
			}
		case "up", "k":
			if m.detailPath != "" {
				m.scroll = max(0, m.scroll-1)
			} else {
				m.cursor = max(0, m.cursor-1)
			}
		case "down", "j":
			if m.detailPath != "" {
				m.scroll = min(max(0, len(strings.Split(m.detail, "\n"))-1), m.scroll+1)
			} else {
				m.cursor = min(max(0, len(m.items)-1), m.cursor+1)
			}
		case "n":
			if !m.loading && m.detailPath == "" && m.nextCursor != "" {
				m.pageHistory = append(m.pageHistory, pagePosition{cursor: m.pageCursor, offset: m.offset})
				m.pageCursor = m.nextCursor
				m.nextCursor = ""
				m.offset += len(m.items)
				m.cursor = 0
				return m.reload()
			}
		case "p":
			if !m.loading && m.detailPath == "" && len(m.pageHistory) > 0 {
				previous := m.pageHistory[len(m.pageHistory)-1]
				m.pageHistory = m.pageHistory[:len(m.pageHistory)-1]
				m.pageCursor = previous.cursor
				m.offset = previous.offset
				m.nextCursor = ""
				m.cursor = 0
				return m.reload()
			}
		case "enter":
			if m.loading || m.detailPath != "" || len(m.items) == 0 {
				return m, nil
			}
			item := m.items[m.cursor]
			id, _ := item["id"].(string)
			if id == "" {
				return m, nil
			}
			if m.section == 0 {
				m.client.project = id
				m.section = 1
				m.cursor = 0
				m.offset = 0
				m.pageCursor = ""
				m.nextCursor = ""
				m.pageHistory = nil
				m.items = nil
				return m.reload()
			}
			m.detailPath = "/api/v1/" + sections[m.section] + "/" + url.PathEscape(id)
			if m.section == 4 {
				if version, ok := item["version"].(float64); ok {
					m.detailPath += "?version=" + strconv.Itoa(int(version))
				}
			}
			m.scroll = 0
			return m.reload()
		case "e":
			if m.section == 3 && m.detailPath != "" && !strings.HasSuffix(m.detailPath, "/events") {
				m.detailPath += "/events"
				m.scroll = 0
				return m.reload()
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	var lines []string
	project := m.client.project
	if project == "" {
		project = m.client.text("（请选择项目）", "(select a project)")
	}
	lines = append(lines, m.client.text("速码 suma · 项目：", "Suma · project: ")+project+m.client.text(" · 只读", " · read-only"))
	var tabs []string
	for i, name := range sections {
		name = m.client.text([]string{"项目", "需求", "任务", "运行", "流程"}[i], name)
		if i == m.section {
			name = "[" + name + "]"
		}
		tabs = append(tabs, name)
	}
	lines = append(lines, strings.Join(tabs, "  "), "")
	if m.loading {
		lines = append(lines, m.client.text("正在加载…", "Loading…"))
	} else if m.err != nil {
		diagnostic := m.err.Error()
		if f, ok := m.err.(*failure); ok {
			diagnostic = localizeFailure(m.client.locale, f).Message
		}
		lines = append(lines, m.client.text("请求失败：", "Request failed: ")+diagnostic, m.client.text("按 r 重试；请检查项目和 SUMA_TOKEN 权限。", "Press r to retry; check project and SUMA_TOKEN permissions."))
	} else if m.detailPath != "" {
		body := strings.Split(m.detail, "\n")
		start := min(m.scroll, len(body))
		lines = append(lines, body[start:]...)
	} else if len(m.items) == 0 {
		lines = append(lines, m.client.text("暂无条目；可先从项目页选择项目。", "No items. Select a project from the projects tab if needed."))
	} else {
		lines = append(lines, fmt.Sprintf(m.client.text("第 %d–%d 项，共 %d 项", "Items %d–%d of %d"), m.offset+1, m.offset+len(m.items), m.total))
		visible := max(1, m.height-8)
		start := max(0, m.cursor-visible+1)
		for i := start; i < min(len(m.items), start+visible); i++ {
			item := m.items[i]
			label := stringField(item, "name")
			if label == "" {
				label = stringField(item, "title")
			}
			if label == "" {
				label = stringField(item, "workflow_name")
			}
			prefix := "  "
			if i == m.cursor {
				prefix = "> "
			}
			state := stringField(item, "state")
			if state == "" {
				state = stringField(item, "status")
			}
			lines = append(lines, prefix+stringField(item, "id")+"  "+label+"  "+state)
		}
	}
	contentHeight := max(1, m.height-3)
	if len(lines) > contentHeight {
		lines = lines[:contentHeight]
	}
	lines = append(lines, "", m.client.text("tab 切换 · ↑↓ 选择/滚动 · enter 打开/选项目 · esc 返回 · r 刷新 · n/p 翻页 · e 运行事件 · q 退出", "tab section · ↑↓ select/scroll · enter open/project · esc back · r refresh · n/p page · e run events · q quit"))
	for i, line := range lines {
		lines[i] = ansi.Truncate(cleanText(line, m.client.token), max(1, m.width), "…")
	}
	return strings.Join(lines, "\n") + "\n"
}

func stringField(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return value
}
func cleanText(value, token string) string {
	if token != "" {
		value = strings.ReplaceAll(value, token, "[redacted]")
	}
	return strings.Map(func(r rune) rune {
		if r < 32 || (r >= 127 && r <= 159) {
			return ' '
		}
		return r
	}, value)
}
func runTUI(ctx context.Context, c *client, in io.Reader, out io.Writer) error {
	_, err := tea.NewProgram(newModel(ctx, c), tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen()).Run()
	if err != nil {
		return &failure{Code: "terminal", Message: "terminal session ended or could not start"}
	}
	return nil
}
