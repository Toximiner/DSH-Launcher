//go:build !js

package main

import (
	"context"
	"flag"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func run(args []string, out io.Writer) error {
	f := flag.NewFlagSet("dsh-launcher-tui-go", flag.ContinueOnError)
	f.SetOutput(out)
	helpFlag := f.Bool("help", false, "показать справку")
	loginFlag := f.String("url", os.Getenv("DSH_TUI_URL"), "URL входа dsh для чтения настроек")
	noMouseFlag := f.Bool("no-mouse", false, "отключить мышь для обычного выделения текста")
	versionFlag := f.Bool("version", false, "показать версию")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *helpFlag {
		fmt.Fprintln(out, "dsh-launcher-tui-go [--help] [--version] [--no-mouse] [--url URL]\nGo-прототип: панели, Tab — разделы, Esc — чат, /help, /clear, /exit.\nНастройки dsh доступны только для чтения через --url или DSH_TUI_URL.\nЧат dsh пока не реализован. Node.js для запуска клиента не нужен.")
		return nil
	}
	if *versionFlag {
		fmt.Fprintln(out, version)
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("run this application in an interactive terminal")
	}
	if *loginFlag != "" {
		if _, err := settingsURL(*loginFlag); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := newModel()
	m.settingsLogin = *loginFlag
	m.settingsContext = ctx
	options := []tea.ProgramOption{tea.WithAltScreen(), tea.WithoutSignalHandler()}
	if !*noMouseFlag {
		options = append(options, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(m, options...)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	done := make(chan struct{})
	defer close(done)
	defer signal.Stop(signals)
	go func() {
		select {
		case <-signals:
			p.Send(quitMsg{})
		case <-done:
		}
	}()
	_, err := p.Run()
	return err
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "dsh-launcher-tui-go:", err)
		os.Exit(1)
	}
}
