package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kelseyhightower/envconfig"
	newrelic "github.com/newrelic/go-agent"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var build = "local"

type config struct {
	Addr            string        `default:":8080" required:"true" split_words:"true"`
	MetricsAddr     string        `default:":5000" required:"true" split_words:"true"`
	NewRelicApiKey  string        `default:"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" required:"true" split_words:"true"`
	NewRelicAppName string        `default:"go-api-local" required:"true" split_words:"true"`
	ReadTimeout     time.Duration `default:"30s" required:"true" split_words:"true"`
	WriteTimeout    time.Duration `default:"30s" required:"true" split_words:"true"`
}

func main() {
	l := log.NewJSONLogger(os.Stdout)
	l = log.WithPrefix(l, "build", build)
	l = log.WithPrefix(l, "date", log.DefaultTimestampUTC)

	var c config
	err := envconfig.Process("SERVER", &c)
	if err != nil {
		l.Log("level", "error", "msg", "could not process env", "err", err.Error())
		panic(err)
	}

	// Create a new relic instance so that we have distributed tracing throughout the application
	nrConfig := newrelic.NewConfig(c.NewRelicAppName, c.NewRelicApiKey)
	nrConfig.CrossApplicationTracer.Enabled = false
	nrConfig.DistributedTracer.Enabled = true
	nrConfig.Labels = map[string]string{
		"group": "make",
	}
	nr, err := newrelic.NewApplication(nrConfig)
	if err != nil {
		l.Log("level", "error", "msg", "could not create new relic application", "err", err.Error())
		os.Exit(1)
	}

	// We make a buffered channel of 2 so that each go routine has a chance to exit when the server stops.
	var errs = make(chan error, 2)

	// Setup our metric server to output prometheus metrics, as well as pprof and expvar.
	metricsServer := http.Server{
		Addr:         c.MetricsAddr,
		ReadTimeout:  time.Second * 30,
		WriteTimeout: time.Second * 30,
	}
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		l.Log("level", "info", "msg", "starting metrics server", "addr", c.MetricsAddr)
		errs <- metricsServer.ListenAndServe()
		l.Log("level", "info", "msg", "stopped metrics server")
	}()

	h := handler{
		l:              l,
		optionProxyURL: "https://slowgest-staging.make.rvapps.io/v1/webhooks/iterable",
	}

	appServer := http.Server{
		Addr:         c.Addr,
		Handler:      newRouter(h, nr),
		ReadTimeout:  c.ReadTimeout,
		WriteTimeout: c.WriteTimeout,
	}
	go func() {
		l.Log("level", "info", "msg", "starting application server", "addr", c.Addr)

		errs <- appServer.ListenAndServe()

		l.Log("level", "info", "msg", "stopped application server")
	}()

	osSignals := make(chan os.Signal)
	signal.Notify(osSignals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	select {
	case err := <-errs:
		l.Log("level", "error", "msg", "received error", "err", err.Error())
		panic(err)
	case s := <-osSignals:
		l.Log("level", "info", "msg", "received signal", "signal", s)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)

		l.Log("level", "info", "msg", "stopping metrics server")
		if err := metricsServer.Shutdown(ctx); err != nil {
			l.Log("level", "error", "msg", "could not shutdown metrics server", "err", err.Error())
			if err := metricsServer.Close(); err != nil {
				l.Log("level", "error", "msg", "could not close metrics server", "err", err.Error())
			}
		}

		l.Log("level", "info", "msg", "stopping application server")
		if err := appServer.Shutdown(ctx); err != nil {
			l.Log("level", "error", "msg", "could not shutdown application server", "err", err.Error())
			if err := appServer.Close(); err != nil {
				l.Log("level", "error", "msg", "could not close application server", "err", err.Error())
			}
		}

		cancel()
		os.Exit(0)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "application/json")
	w.Write([]byte("Healthy AF"))
	w.WriteHeader(http.StatusOK)
}
