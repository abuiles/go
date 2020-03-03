package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/rcrowley/go-metrics"
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

// Cron interface
type Cron interface {
	AddJob(spec string, cmd cron.Job) error
	Entries() []*cron.Entry
	Start()
}

// App is the application object
type App struct {
	config  Config
	cron    Cron
	metrics metrics.Registry
}

// NewApp constructs an new App instance from the provided config.
func NewApp(config Config) (app *App, err error) {
	app = &App{
		config:  config,
		cron:    cron.New(),
		metrics: metrics.NewRegistry(),
	}
	return
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

	// TODO: Stop crons when shuttingdown the server gracefully
	app.StartCrons()
	app.Serve()
}

// Serve starts the server
func (a *App) Serve() {
	// TODO: maybe we don't need all of this and we can go with a simple http.Server
	mux := supportHttp.NewAPIMux(supportLog.DefaultLogger)

	// Middlewares
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")

	mux.Method(http.MethodGet, "/", IndexHandler{app: a})
	mux.Method(http.MethodGet, "/metrics", MetricsHandler{app: a})

	supportHttp.Run(supportHttp.Config{
		ListenAddr: fmt.Sprintf(":%d", *a.config.Port),
		Handler:    mux,
		OnStarting: func() {
			log.Infof("starting health-check server")
			log.Infof("listening on %d", *a.config.Port)
		},
	})
}

// ----------------------------------------------------------------------------------------------
// Commands infra
//

// Cmd wrapper for Cmd tool
type Cmd struct {
	Name   string `json:"name"`
	metric metrics.Gauge
	base   string // <--- this is used just for testing, we should pass the command as an arg
}

// Run runs command
func (c *Cmd) Run() {
	log.Infof("Running %s", c.Name)
	// TODO add a mutex and avoid running if there is a command currently running
	// TODO make  command configurable
	cmd := exec.Command("horizon-cmp", "history", "-t", "https://horizon.stellar.org", "-b", c.base, "--count", "4")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Errorf("cmd.Run() failed with %s\n", err)
		c.metric.Update(1)
		return
	}
	c.metric.Update(0)
}

// StartCrons starts cron job
func (a *App) StartCrons() {
	// TODO: read each command from a config file, metric type and frequency
	horizonCmp := Cmd{
		Name:   "HorizonCmp",
		metric: metrics.NewGauge(),
		base:   "https://horizon-testnet.stellar.org",
	}
	a.cron.AddJob("@every 20s", &horizonCmp)
	a.metrics.Register("horizon_cmp_testnet.failures", horizonCmp.metric)

	horizonCmpMain := Cmd{
		Name:   "HorizonCmpMain",
		metric: metrics.NewGauge(),
		base:   "https://horizon-blue.stellar.org",
	}
	a.cron.AddJob("@every 30s", &horizonCmpMain)
	a.metrics.Register("horizon_cmp_pubnet.failures", horizonCmpMain.metric)

	a.cron.Start()
}

// ----------------------------------------------------------------------------------------------
// HTTP Handlers
//

// MetricsHandler http handler for /metrics
type MetricsHandler struct {
	// we should probably pass this through ctx
	app *App
}

func (handler MetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler.app.metrics.Each(func(name string, i interface{}) {
		// Replace `.` with `_` to follow Prometheus metric name convention.
		name = strings.ReplaceAll(name, ".", "_")

		switch metric := i.(type) {
		case metrics.Gauge:
			fmt.Fprintf(w, "health_%s %d\n", name, metric.Value())
		}
		fmt.Fprintf(w, "\n")
	})
}

// IndexHandler handler for /
type IndexHandler struct {
	// we should probably pass this through ctx?
	app *App
}

func (handler IndexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := json.Marshal(handler.app.cron.Entries())
	if err != nil {
		w.Write([]byte(`{"error": "error reading cron job entries"}`))
	}

	w.Write(body)
}
