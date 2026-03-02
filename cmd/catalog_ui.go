package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/silenceper/aikit/internal/web"
	"github.com/spf13/cobra"
)

var (
	uiHost string
	uiPort int
)

func init() {
	catalogUICmd.Flags().StringVar(&uiHost, "host", "localhost", "Host to bind to")
	catalogUICmd.Flags().IntVarP(&uiPort, "port", "p", 9001, "Port to listen on")
	catalogCmd.AddCommand(catalogUICmd)
}

var catalogUICmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch web UI to manage global catalog",
	Long:  "Start a local web server that provides a browser-based interface for managing the global asset catalog (~/.aikit/catalog.yaml).",
	RunE:  runCatalogUI,
}

func runCatalogUI(_ *cobra.Command, _ []string) error {
	srv := web.NewServer(uiHost, uiPort)

	go func() {
		browserHost := uiHost
		if browserHost == "" || browserHost == "0.0.0.0" {
			browserHost = "localhost"
		}
		addr := fmt.Sprintf("http://%s:%d", browserHost, uiPort)
		fmt.Printf("Catalog UI available at %s\n", addr)
		time.Sleep(500 * time.Millisecond)
		openBrowser(addr)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-quit
	fmt.Println("\nShutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
