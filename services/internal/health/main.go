package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	log "github.com/sirupsen/logrus"

	"github.com/revel/cron"
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

	// TODO: Stop cron after when shuttingdown the server
	app.StartCron()
	app.Serve()
}

// Cron interface
type Cron interface {
	AddJob(spec string, cmd cron.Job) error
	Entries() []*cron.Entry
	Start()
}

// App is the application object
type App struct {
	config Config
	cron   Cron
}

// NewApp constructs an new App instance from the provided config.
func NewApp(config Config) (app *App, err error) {
	app = &App{
		config: config,
		cron:   cron.New(),
	}
	return
}

// HorizonCmp wrapper for HorizonCmp tool
type HorizonCmp struct {
	Name string `json:"name"`
}

func (cmp *HorizonCmp) Run() {
	cmd := exec.Command("horizon-cmp", "history", "-t", "https://horizon.stellar.org", "-b", "https://horizon.stellar.org", "--count", "1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Fatalf("cmd.Run() failed with %s\n", err)
	}
}

// StartCron starts cron job
func (a *App) StartCron() {
	a.cron.AddJob("@every 10s", &HorizonCmp{Name: "HorizonCmp"})
	a.cron.Start()
}

type CronHandler struct {
	// we should probably pass this through ctx
	app *App
}

func (handler CronHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := json.Marshal(handler.app.cron.Entries())
	if err != nil {
		w.Write([]byte(`{"error": "error reading cron job entries"}`))
	}

	w.Write(body)
}

// Serve starts the server
func (a *App) Serve() {
	// TODO: maybe we don't need all of this and we can go with a simple http.Server
	mux := supportHttp.NewAPIMux(supportLog.DefaultLogger)

	// Middlewares
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")

	mux.Method(http.MethodGet, "/", CronHandler{app: a})

	supportHttp.Run(supportHttp.Config{
		ListenAddr: fmt.Sprintf(":%d", *a.config.Port),
		Handler:    mux,
		OnStarting: func() {
			log.Infof("starting health-check server")
			log.Infof("listening on %d", *a.config.Port)
		},
	})
}
