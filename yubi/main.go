// Command yubi is a herdr plugin that copies text from the Target Pane's
// visible screen with Hints or Line Numbers.
//
//	yubi version        print <version>+<commit>[.dirty]
//	yubi action <mode>  run by herdr's plugin_action: open the overlay (mode: hint, line)
//	yubi ui             the overlay pane process (YUBI_MODE, YUBI_TARGET_PANE)
package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/config"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
	"github.com/wchoco/herdr-plugins/yubi/internal/herdr"
)

const pluginID = "wchoco.yubi"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "yubi:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "version":
		info, _ := debug.ReadBuildInfo()
		v, err := versionString(manifest, info)
		if err != nil {
			return err
		}
		fmt.Println(v)
		return nil
	case "action":
		if len(args) != 2 || (args[1] != string(session.ModeHint) && args[1] != string(session.ModeLine)) {
			return usage()
		}
		return action(session.Mode(args[1]))
	case "ui":
		return ui()
	}
	return usage()
}

func usage() error {
	return fmt.Errorf("usage: yubi version | yubi action <hint|line> | yubi ui")
}

// action runs without a TTY. It opens the overlay over the focused pane,
// which becomes the Target Pane, or, pressed again inside the overlay,
// toggles it closed or switches its mode (see launch).
func action(mode session.Mode) error {
	focused := os.Getenv("HERDR_PANE_ID")
	if focused == "" {
		return fmt.Errorf("HERDR_PANE_ID is not set; run yubi from a herdr keybinding")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := herdr.NewClient(os.Getenv("HERDR_SOCKET_PATH"))
	return launch(ctx, client, os.Getenv("HERDR_PLUGIN_STATE_DIR"), focused, mode,
		func(ctx context.Context, target string) (string, error) {
			return client.OpenOverlay(ctx, pluginID, target, mode)
		})
}

// ui is the overlay pane process.
func ui() error {
	tty, err := herdr.OpenTTY(os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	defer tty.Close()
	client := herdr.NewClient(os.Getenv("HERDR_SOCKET_PATH"))
	cfg := session.Config{
		Mode:   session.Mode(os.Getenv("YUBI_MODE")),
		Target: os.Getenv("YUBI_TARGET_PANE"),
		// herdr sets these to the overlay's own pane, tab and workspace.
		Overlay:   os.Getenv("HERDR_PANE_ID"),
		Tab:       os.Getenv("HERDR_TAB_ID"),
		Workspace: os.Getenv("HERDR_WORKSPACE_ID"),
	}
	// Read fresh on every launch: no reload step.
	data, err := herdr.ReadConfig(os.Getenv("HERDR_PLUGIN_CONFIG_DIR"))
	if err == nil {
		var settings config.Config
		if settings, err = config.Parse(data); err == nil {
			cfg.Settings = &settings
		}
	}
	cfg.SettingsErr = err
	return session.Run(context.Background(), cfg,
		session.Deps{
			Source:    client,
			Clipboard: herdr.NewOSC52(tty.Writer()),
			Terminal:  tty,
			Events:    client,
		})
}
