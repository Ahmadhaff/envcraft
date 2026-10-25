package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/ahmadhaffoudhi/envcraft/internal/checker"
)

func init() {
	rootCmd.AddCommand(newDoctorCmd())
}

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "doctor",
		Aliases: []string{"check"},
		Short:   "Check the local development stack",
		Long: `Dial the local stack at the same time and paint each result as it finishes.

Port rows are TCP reachability, not a protocol login. Docker, Go, and Node
are identified from their CLIs. The summary is wall-clock time. Each row is
that check's own latency.`,
		Example: `  envcraft doctor
  envcraft doctor --postgres-port 5433 --timeout 3s`,
		Args: cobra.NoArgs,
		RunE: runDoctor,
	}
	cmd.Flags().String("host", "127.0.0.1", "Host for port checks")
	cmd.Flags().Int("postgres-port", 5432, "PostgreSQL port")
	cmd.Flags().Int("redis-port", 6379, "Redis port")
	cmd.Flags().Duration("timeout", 2*time.Second, "Per-check timeout (50ms to 30s)")
	return cmd
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	host := stringSetting(cmd, "host", "ENVCRAFT_DOCTOR_HOST", "doctor.host", "127.0.0.1")
	if host == "" {
		return errors.New("host is required")
	}
	pg, err := intSetting(cmd, "postgres-port", "ENVCRAFT_DOCTOR_POSTGRES_PORT", "doctor.postgres_port", 5432)
	if err != nil {
		return err
	}
	redisPort, err := intSetting(cmd, "redis-port", "ENVCRAFT_DOCTOR_REDIS_PORT", "doctor.redis_port", 6379)
	if err != nil {
		return err
	}
	timeout, err := durationSetting(cmd, "timeout", "ENVCRAFT_DOCTOR_TIMEOUT", "doctor.timeout", 2*time.Second)
	if err != nil {
		return err
	}
	if err := validatePort("postgres", pg); err != nil {
		return err
	}
	if err := validatePort("redis", redisPort); err != nil {
		return err
	}
	if err := validateTimeout(timeout); err != nil {
		return err
	}

	probes := []checker.Probe{
		checker.Docker(),
		checker.Port("postgres", fmt.Sprintf("PostgreSQL :%d", pg), host, pg),
		checker.Port("redis", fmt.Sprintf("Redis :%d", redisPort), host, redisPort),
		checker.Node(),
		checker.Go(),
	}
	if debug {
		printProbePlan(cmd.ErrOrStderr(), probes)
	}

	base := context.Background()
	var stop context.CancelFunc
	if !interactive() {
		base, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
	}
	ctx, cancel := context.WithTimeout(base, timeout)
	defer cancel()

	events := checker.Run(ctx, probes)
	started := time.Now()
	if interactive() {
		return runDoctorTUI(probes, events, cancel, started)
	}
	return runDoctorPlain(cmd.OutOrStdout(), events, started, ctx)
}

func printProbePlan(w io.Writer, probes []checker.Probe) {
	for _, probe := range probes {
		fmt.Fprintf(w, "debug %s: %s\n", probe.Name, probe.Target)
	}
}

func runDoctorPlain(w io.Writer, events <-chan checker.Result, started time.Time, ctx context.Context) error {
	fmt.Fprintln(w, banner(version))
	fmt.Fprintln(w)
	fmt.Fprintln(w, heading("Stack"))
	fmt.Fprintln(w)
	passed, failed := 0, 0
	for res := range events {
		if res.OK {
			passed++
			fmt.Fprintln(w, passRow(res.Name, res.Detail, formatLatency(res.Duration)))
			continue
		}
		failed++
		fmt.Fprintln(w, failRow(res.Name, res.Detail, formatLatency(res.Duration)))
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		fmt.Fprintln(w)
		fmt.Fprintln(w, hint("canceled"))
		return &ExitError{Code: 130}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, rule())
	fmt.Fprintln(w)
	fmt.Fprintln(w, summary(passed, failed, time.Since(started)))
	if failed > 0 {
		return &ExitError{Code: 1}
	}
	return nil
}

type rowState struct {
	id   string
	name string
	res  *checker.Result
}

type doctorModel struct {
	spinner  spinner.Model
	rows     []rowState
	index    map[string]int
	events   <-chan checker.Result
	cancel   context.CancelFunc
	started  time.Time
	done     bool
	canceled bool
}

type resultMsg checker.Result
type finishedMsg struct{}

func runDoctorTUI(probes []checker.Probe, events <-chan checker.Result, cancel context.CancelFunc, started time.Time) error {
	rows := make([]rowState, len(probes))
	index := make(map[string]int, len(probes))
	for i, probe := range probes {
		rows[i] = rowState{id: probe.ID, name: probe.Name}
		index[probe.ID] = i
	}
	model := &doctorModel{
		spinner: newSpinner(),
		rows:    rows,
		index:   index,
		events:  events,
		cancel:  cancel,
		started: started,
	}
	program := tea.NewProgram(model)
	final, err := program.Run()
	if err != nil {
		cancel()
		if errors.Is(err, tea.ErrProgramKilled) || errors.Is(err, context.Canceled) {
			return &ExitError{Code: 130}
		}
		return err
	}
	done, ok := final.(*doctorModel)
	if !ok || !done.done {
		cancel()
		return &ExitError{Code: 130}
	}
	if failedCount(done) > 0 {
		return &ExitError{Code: 1}
	}
	return nil
}

func (m *doctorModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, listen(m.events))
}

func (m *doctorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.canceled = true
			m.cancel()
			return m, tea.Quit
		}
	case resultMsg:
		m.apply(checker.Result(msg))
		return m, listen(m.events)
	case finishedMsg:
		m.done = true
		return m, tea.Quit
	case spinner.TickMsg:
		if m.done || m.canceled {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *doctorModel) View() string {
	var b strings.Builder
	b.WriteString(banner(version))
	b.WriteString("\n\n")
	b.WriteString(heading("Stack"))
	b.WriteString("\n\n")
	for _, row := range m.rows {
		switch {
		case row.res == nil && m.canceled:
			b.WriteString(idleRow(row.name, "canceled", ""))
		case row.res == nil:
			b.WriteString(pendingRow(m.spinner.View(), row.name, "checking…"))
		case row.res.OK:
			b.WriteString(passRow(row.res.Name, row.res.Detail, formatLatency(row.res.Duration)))
		default:
			b.WriteString(failRow(row.res.Name, row.res.Detail, formatLatency(row.res.Duration)))
		}
		b.WriteString("\n")
	}
	if m.done {
		passed, failed := tally(m)
		b.WriteString("\n")
		b.WriteString(rule())
		b.WriteString("\n")
		b.WriteString(summary(passed, failed, time.Since(m.started)))
		b.WriteString("\n")
	} else if m.canceled {
		b.WriteString("\n")
		b.WriteString(hint("canceled"))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
		b.WriteString(hint("esc cancel"))
		b.WriteString("\n")
	}
	return b.String()
}

func (m *doctorModel) apply(res checker.Result) {
	i, ok := m.index[res.ID]
	if !ok {
		return
	}
	m.rows[i].res = &res
}

func listen(ch <-chan checker.Result) tea.Cmd {
	return func() tea.Msg {
		res, ok := <-ch
		if !ok {
			return finishedMsg{}
		}
		return resultMsg(res)
	}
}

func tally(m *doctorModel) (passed, failed int) {
	for _, row := range m.rows {
		if row.res != nil && row.res.OK {
			passed++
			continue
		}
		failed++
	}
	return passed, failed
}

func failedCount(m *doctorModel) int {
	_, failed := tally(m)
	return failed
}
