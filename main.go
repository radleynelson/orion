package main

import (
	"embed"
	"flag"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

var projectFlag string

func main() {
	flag.StringVar(&projectFlag, "project", "", "Project directory to open")
	flag.Parse()

	if projectFlag == "" && flag.NArg() > 0 {
		projectFlag = flag.Arg(0)
	}

	if projectFlag != "" {
		os.Setenv("ORION_PROJECT", projectFlag)
	}

	app := NewApp()

	// Build menu bar.
	// IMPORTANT: Do NOT assign accelerators for Cmd+C, Cmd+V, Cmd+X, Cmd+A,
	// Cmd+T, Cmd+W, Cmd+D, Cmd+N — these are handled by JS in the webview.
	// Native menu accelerators intercept keydown before xterm.js can process them.
	// Only use Cmd+Shift combos or no accelerator for menu items.
	appMenu := menu.NewMenu()
	appMenu.Append(menu.AppMenu())

	// File menu — no accelerators that conflict with terminal
	fileMenu := appMenu.AddSubmenu("File")
	fileMenu.AddText("New Window", nil, func(_ *menu.CallbackData) {
		app.NewWindow()
	})
	fileMenu.AddText("Open Project...", nil, func(_ *menu.CallbackData) {
		wailsRuntime.EventsEmit(app.ctx, "menu:open-project")
	})
	fileMenu.AddSeparator()
	fileMenu.AddText("New Terminal Tab", nil, func(_ *menu.CallbackData) {
		wailsRuntime.EventsEmit(app.ctx, "menu:new-terminal")
	})
	fileMenu.AddText("Close Pane", nil, func(_ *menu.CallbackData) {
		wailsRuntime.EventsEmit(app.ctx, "menu:close-tab")
	})

	// Edit menu — required for macOS WKWebView paste/copy to work.
	// Uses built-in roles that connect to the responder chain.
	appMenu.Append(menu.EditMenu())

	// View menu
	viewMenu := appMenu.AddSubmenu("View")
	viewMenu.AddText("Toggle Sidebar", nil, func(_ *menu.CallbackData) {
		wailsRuntime.EventsEmit(app.ctx, "menu:toggle-sidebar")
	})
	viewMenu.AddSeparator()
	viewMenu.AddText("Workspaces", nil, func(_ *menu.CallbackData) {
		wailsRuntime.EventsEmit(app.ctx, "menu:show-workspaces")
	})
	viewMenu.AddText("File Explorer", nil, func(_ *menu.CallbackData) {
		wailsRuntime.EventsEmit(app.ctx, "menu:show-files")
	})
	viewMenu.AddText("Search", nil, func(_ *menu.CallbackData) {
		wailsRuntime.EventsEmit(app.ctx, "menu:show-search")
	})
	viewMenu.AddText("Git Changes", nil, func(_ *menu.CallbackData) {
		wailsRuntime.EventsEmit(app.ctx, "menu:show-git")
	})

	// Terminal menu
	termMenu := appMenu.AddSubmenu("Terminal")
	termMenu.AddText("Split Right", nil, func(_ *menu.CallbackData) {
		wailsRuntime.EventsEmit(app.ctx, "menu:split-right")
	})
	termMenu.AddText("Split Down", nil, func(_ *menu.CallbackData) {
		wailsRuntime.EventsEmit(app.ctx, "menu:split-down")
	})
	termMenu.AddSeparator()
	termMenu.AddText("Next Pane", nil, func(_ *menu.CallbackData) {
		wailsRuntime.EventsEmit(app.ctx, "menu:next-pane")
	})
	termMenu.AddText("Previous Pane", nil, func(_ *menu.CallbackData) {
		wailsRuntime.EventsEmit(app.ctx, "menu:prev-pane")
	})

	// Window menu
	appMenu.Append(menu.WindowMenu())

	err := wails.Run(&options.App{
		Title:     "Orion",
		Width:     1400,
		Height:    900,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 37, G: 37, B: 37, A: 255},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnShutdown:       app.shutdown,
		Frameless:        false,
		Menu:             appMenu,
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                 true,
				HideTitleBar:              false,
				FullSizeContent:           true,
				UseToolbar:                true,
			},
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   "Orion",
				Message: "Workspace Manager for Agentic Coding",
			},
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
