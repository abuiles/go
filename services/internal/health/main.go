package main

import (
	"fmt"
	"net/http"
	"runtime"

	log "github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
	supportHttp "github.com/stellar/go/support/http"
	supportLog "github.com/stellar/go/support/log"
)

var app *App
var rootCmd *cobra.Command

// Config contains config  of the health server
type Config struct {
	Port *int `valid:"required"`
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	rootCmd.Execute()
}

func init() {
	rootCmd = &cobra.Command{
		Use:   "health-check",
		Short: "health check server",
		Long:  `health check server`,
		Run:   run,
	}
}

func run(cmd *cobra.Command, args []string) {
	port := 8000
	cfg := Config{
		Port: &port,
	}
	app, err := NewApp(cfg)

	if err != nil {
		log.Fatal(err.Error())
		return
	}

	app.Serve()
}

// App is the application object
type App struct {
	config Config
}

// NewApp constructs an new App instance from the provided config.
func NewApp(config Config) (app *App, err error) {
	app = &App{
		config: config,
	}
	return
}

// Serve starts the server
func (a *App) Serve() {
	// TODO: maybe we don't need all of this and we can go with a simple http.Server
	mux := supportHttp.NewAPIMux(supportLog.DefaultLogger)

	// Middlewares
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")

	supportHttp.Run(supportHttp.Config{
		ListenAddr: fmt.Sprintf(":%d", *a.config.Port),
		Handler:    mux,
		OnStarting: func() {
			log.Infof("starting health-check server")
			log.Infof("listening on %d", *a.config.Port)
		},
	})
}
