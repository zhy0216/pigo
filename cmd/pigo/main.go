package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zhy0216/pigo/pkg/agent"
	"github.com/zhy0216/pigo/pkg/config"
	"github.com/zhy0216/pigo/pkg/skills"
	"github.com/zhy0216/pigo/pkg/types"
)

// Version is set at build time via ldflags.
var Version string

func main() {
	defer setupMemProfile()()

	cfg, err := config.Load()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	app := agent.NewAgent(cfg)

	fmt.Printf("%spigo%s - minimal AI coding assistant (model: %s, api: %s)\n", types.ColorGreen, types.ColorReset, app.GetModel(), cfg.APIType)
	fmt.Printf("Tools: %s\n", strings.Join(app.GetRegistry().List(), ", "))
	fmt.Printf("Commands: /q (quit), /c (clear), /model, /usage, /skills\n")
	if len(app.Skills) > 0 {
		var skillNames []string
		for _, s := range app.Skills {
			skillNames = append(skillNames, "/skill:"+s.Name)
		}
		fmt.Printf("Skills: %s\n", strings.Join(skillNames, ", "))
	}
	fmt.Println()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var turnCancel context.CancelFunc
	var lastSigTime time.Time
	var mu sync.Mutex

	go func() {
		for sig := range sigChan {
			mu.Lock()
			if sig == syscall.SIGTERM {
				mu.Unlock()
				fmt.Println("\nGoodbye!")
				os.Exit(0)
			}
			now := time.Now()
			if now.Sub(lastSigTime) < time.Second {
				mu.Unlock()
				fmt.Println("\nGoodbye!")
				os.Exit(0)
			}
			lastSigTime = now
			if turnCancel != nil {
				turnCancel()
				turnCancel = nil
				fmt.Fprintf(os.Stderr, "\n%s[interrupted]%s\n", types.ColorYellow, types.ColorReset)
			}
			mu.Unlock()
		}
	}()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s> %s", types.ColorBlue, types.ColorReset)
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		input = strings.TrimSpace(input)

		if input == "" {
			continue
		}

		handled, exit := app.HandleCommand(input)
		if exit {
			return
		}
		if handled {
			continue
		}

		if expanded, ok := skills.ExpandSkillCommand(input, app.Skills); ok {
			name := strings.TrimPrefix(input, "/skill:")
			if idx := strings.Index(name, " "); idx >= 0 {
				name = name[:idx]
			}
			fmt.Printf("%s[skill: %s]%s\n", types.ColorGray, name, types.ColorReset)
			input = expanded
		}

		turnCtx, cancel := context.WithCancel(context.Background())
		mu.Lock()
		turnCancel = cancel
		mu.Unlock()

		err = app.ProcessInput(turnCtx, input)
		cancel()

		mu.Lock()
		turnCancel = nil
		mu.Unlock()

		if err != nil {
			if turnCtx.Err() == context.Canceled {
				continue
			}
			fmt.Printf("%sError: %v%s\n", types.ColorYellow, err, types.ColorReset)
		}
	}
}
