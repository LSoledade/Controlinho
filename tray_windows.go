//go:build windows

package main

// System-tray icon for the Windows build. With the windowsgui (no-console) build
// this is the only UI the background server has: it lets you reach the connect
// page, toggle auto-start, and quit — solving the "is it even running?" problem
// of a hidden Task-Scheduler launch.
//
// systray.Run owns the message loop and blocks until systray.Quit(), so runUI is
// called from the main goroutine at the end of main(); a termination signal also
// quits the tray, after which onExit runs the graceful shutdown.

import (
	"context"
	"log"

	"fyne.io/systray"
)

func runUI(ctx context.Context, httpAddr string, shutdown func()) {
	go func() { <-ctx.Done(); systray.Quit() }()
	systray.Run(func() { trayOnReady(httpAddr) }, shutdown)
}

func trayOnReady(httpAddr string) {
	if ico, err := clientFS.ReadFile("client/icon.ico"); err == nil {
		systray.SetIcon(ico)
	}
	systray.SetTitle("PC Remote")
	systray.SetTooltip("PC Remote — controle do PC pelo celular")

	mOpen := systray.AddMenuItem("Abrir página de conexão", "Mostra os QR codes para parear o celular")
	systray.AddSeparator()
	mAuto := systray.AddMenuItemCheckbox("Iniciar com o Windows", "Registra/remove a tarefa de logon", taskInstalled())
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Sair", "Encerra o servidor")

	qrURL := "http://" + probeHost(httpAddr) + portOf(httpAddr) + "/qr"

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openBrowser(qrURL)
			case <-mAuto.ClickedCh:
				if mAuto.Checked() {
					if err := uninstallSelf(); err != nil {
						log.Printf("tray: uninstall: %v", err)
					} else {
						mAuto.Uncheck()
					}
				} else {
					// An instance is already running (this one), so don't start another.
					if err := installSelf(false); err != nil {
						log.Printf("tray: install: %v", err)
					} else {
						mAuto.Check()
					}
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}
