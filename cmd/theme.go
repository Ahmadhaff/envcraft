package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

const (
	nameWidth    = 18
	detailWidth  = 20
	timeWidth    = 8
	contentWidth = 55
)

var (
	accent   = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	okColor  = lipgloss.AdaptiveColor{Light: "#047857", Dark: "#34D399"}
	badColor = lipgloss.AdaptiveColor{Light: "#BE123C", Dark: "#FB7185"}
	ink      = lipgloss.AdaptiveColor{Light: "#0F172A", Dark: "#E2E8F0"}
	muted    = lipgloss.AdaptiveColor{Light: "#475569", Dark: "#94A3B8"}
	faint    = lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#64748B"}
	line     = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#115E59"}
)

func applyColor(noColor bool) {
	if noColor || os.Getenv("NO_COLOR") != "" || !term.IsTerminal(int(os.Stdout.Fd())) {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
}

func interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func plain() bool {
	return lipgloss.ColorProfile() == termenv.Ascii
}

func banner(v string) string {
	title := lipgloss.JoinHorizontal(lipgloss.Bottom,
		lipgloss.NewStyle().Bold(true).Foreground(accent).Render("envcraft"),
		lipgloss.NewStyle().Foreground(muted).Render("  "+v),
	)
	sub := lipgloss.NewStyle().Foreground(faint).Render("env vaults  ·  stack doctor  ·  scaffolds")
	body := lipgloss.JoinVertical(lipgloss.Left, title, sub)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(line).
		Padding(0, 1).
		Render(body)
}

func heading(text string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(ink).Render(text)
}

func accentText(text string) string {
	return lipgloss.NewStyle().Foreground(accent).Bold(true).Render(text)
}

func errorText(msg string) string {
	label := lipgloss.NewStyle().Foreground(badColor).Bold(true).Render("error")
	return label + "  " + msg
}

func hint(text string) string {
	return "  " + lipgloss.NewStyle().Foreground(faint).Render(text)
}

func rule() string {
	glyph := "─"
	if plain() {
		glyph = "-"
	}
	return "  " + lipgloss.NewStyle().Foreground(line).Render(strings.Repeat(glyph, contentWidth))
}

func success(text string) string {
	return "  " + passMark() + "  " + lipgloss.NewStyle().Foreground(ink).Render(text)
}

func sub(text string) string {
	return "       " + lipgloss.NewStyle().Foreground(muted).Render(text)
}

func doneLine(text string) string {
	return "  " + lipgloss.NewStyle().Foreground(okColor).Render(text)
}

func passRow(name, detail, latency string) string {
	return renderRow(passMark(), name, detail, latency, lipgloss.NewStyle().Foreground(muted))
}

func failRow(name, detail, latency string) string {
	return renderRow(failMark(), name, detail, latency, lipgloss.NewStyle().Foreground(badColor))
}

func idleRow(name, detail, latency string) string {
	return renderRow(idleMark(), name, detail, latency, lipgloss.NewStyle().Foreground(faint))
}

func pendingRow(frame, name, detail string) string {
	return renderRow(pendingMark(frame), name, detail, "", lipgloss.NewStyle().Foreground(muted))
}

func summary(passed, failed int, elapsed time.Duration) string {
	passTxt := fmt.Sprintf("%d passed", passed)
	mid := ""
	failTxt := ""
	if failed > 0 {
		mid = "  ·  "
		failTxt = fmt.Sprintf("%d failed", failed)
	}
	right := fmt.Sprintf("completed in %s", formatLatency(elapsed))
	gap := contentWidth - len(passTxt) - len(mid) - len(failTxt) - len(right)
	if gap < 2 {
		gap = 2
	}
	left := lipgloss.NewStyle().Foreground(okColor).Render(passTxt) + mid
	if failTxt != "" {
		left += lipgloss.NewStyle().Foreground(badColor).Render(failTxt)
	}
	return "  " + left + strings.Repeat(" ", gap) + lipgloss.NewStyle().Foreground(faint).Render(right)
}

func renderRow(mark, name, detail, latency string, detailStyle lipgloss.Style) string {
	nameStyled := lipgloss.NewStyle().Foreground(ink).Render(pad(name, nameWidth))
	detailStyled := detailStyle.Render(pad(detail, detailWidth))
	timeStyled := lipgloss.NewStyle().Foreground(faint).Render(lpad(latency, timeWidth))
	return "  " + mark + "  " + nameStyled + "  " + detailStyled + "  " + timeStyled
}

func passMark() string {
	style := lipgloss.NewStyle().Foreground(okColor).Bold(true)
	if plain() {
		return style.Render("[+]")
	}
	return " " + style.Render("✓") + " "
}

func failMark() string {
	style := lipgloss.NewStyle().Foreground(badColor).Bold(true)
	if plain() {
		return style.Render("[x]")
	}
	return " " + style.Render("✗") + " "
}

func idleMark() string {
	style := lipgloss.NewStyle().Foreground(faint)
	if plain() {
		return style.Render("[-]")
	}
	return " " + style.Render("·") + " "
}

func pendingMark(frame string) string {
	if plain() {
		return lipgloss.NewStyle().Foreground(accent).Render("[" + frame + "]")
	}
	return " " + frame + " "
}

func arrow() string {
	if plain() {
		return "->"
	}
	return "→"
}

func formatLatency(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < 10*time.Millisecond:
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < 10*time.Second:
		return fmt.Sprintf("%.1fs", float64(d)/float64(time.Second))
	default:
		return fmt.Sprintf("%ds", int(d.Round(time.Second)/time.Second))
	}
}

func formatBytes(n int64) string {
	switch {
	case n < 0:
		return ""
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

func pad(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		if n <= 1 {
			return string(r[:n])
		}
		return string(r[:n-1]) + "…"
	}
	return s + strings.Repeat(" ", n-len(r))
}

func lpad(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[len(r)-n:])
	}
	return strings.Repeat(" ", n-len(r)) + s
}

func newSpinner() spinner.Model {
	s := spinner.New()
	if plain() {
		s.Spinner = spinner.Line
	} else {
		s.Spinner = spinner.Dot
	}
	s.Style = lipgloss.NewStyle().Foreground(accent)
	return s
}

func formTheme() *huh.Theme {
	t := huh.ThemeCharm()
	t.Focused.Title = t.Focused.Title.Foreground(accent).Bold(true)
	t.Focused.Description = t.Focused.Description.Foreground(muted)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(accent)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(accent)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(ink).Background(accent)
	return t
}
