// Copyright (c) 2019 Ashley Jeffs
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package service

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"

	"github.com/Jeffail/benthos/lib/api"
	"github.com/Jeffail/benthos/lib/buffer"
	"github.com/Jeffail/benthos/lib/cache"
	"github.com/Jeffail/benthos/lib/condition"
	"github.com/Jeffail/benthos/lib/config"
	"github.com/Jeffail/benthos/lib/input"
	"github.com/Jeffail/benthos/lib/log"
	"github.com/Jeffail/benthos/lib/manager"
	"github.com/Jeffail/benthos/lib/metrics"
	"github.com/Jeffail/benthos/lib/output"
	"github.com/Jeffail/benthos/lib/processor"
	"github.com/Jeffail/benthos/lib/ratelimit"
	"github.com/Jeffail/benthos/lib/stream"
	strmmgr "github.com/Jeffail/benthos/lib/stream/manager"
	"github.com/Jeffail/benthos/lib/tracer"
	uconfig "github.com/Jeffail/benthos/lib/util/config"
)

//------------------------------------------------------------------------------

// Build stamps.
var (
	Version   string
	DateBuilt string
)

//------------------------------------------------------------------------------

// Extra flags
var (
	showVersion = flag.Bool(
		"version", false, "Display version info, then exit",
	)
	showConfigJSON = flag.Bool(
		"print-json", false, "Print loaded configuration as JSON, then exit",
	)
	showConfigYAML = flag.Bool(
		"print-yaml", false, "Print loaded configuration as YAML, then exit",
	)
	showAll = flag.Bool(
		"all", false,
		`
Set whether all fields should be shown when printing configuration via
--print-yaml or --print-json, otherwise only used values will be printed.`[1:],
	)
	configPath = flag.String(
		"c", "", "Path to a configuration file",
	)
	lintConfig = flag.Bool(
		"lint", false, "Lint the target configuration file, then exit",
	)
	strictConfig = flag.Bool(
		"strict", false,
		`
Parse config files in strict mode, where any linting errors will cause Benthos
to fail`[1:],
	)
	examples = flag.String(
		"example", "",
		`
Add specific examples when printing a configuration file with --print-yaml or
--print-json by listing comma separated types. Types can be any input, buffer,
processor or output.

For example: 'benthos --print-yaml --example websocket,jmespath' would print a
config with a websocket input and output and a jmespath processor.`[1:],
	)
	printInputs = flag.Bool(
		"list-inputs", false,
		"Print a list of available input options, then exit",
	)
	printOutputs = flag.Bool(
		"list-outputs", false,
		"Print a list of available output options, then exit",
	)
	printBuffers = flag.Bool(
		"list-buffers", false,
		"Print a list of available buffer options, then exit",
	)
	printProcessors = flag.Bool(
		"list-processors", false,
		"Print a list of available processor options, then exit",
	)
	printConditions = flag.Bool(
		"list-conditions", false,
		"Print a list of available processor condition options, then exit",
	)
	printCaches = flag.Bool(
		"list-caches", false,
		"Print a list of available cache options, then exit",
	)
	printRateLimits = flag.Bool(
		"list-rate-limits", false,
		"Print a list of available rate_limit options, then exit",
	)
	printMetrics = flag.Bool(
		"list-metrics", false,
		"Print a list of available metrics options, then exit",
	)
	printTracers = flag.Bool(
		"list-tracers", false,
		"Print a list of available tracer options, then exit",
	)
	streamsMode = flag.Bool(
		"streams", false,
		`
Run Benthos in streams mode, where streams can be created, updated and removed
via REST HTTP endpoints. In streams mode the stream fields of a config file
(input, buffer, pipeline, output) will be ignored. Instead, any .yaml or .json
files inside the --streams-dir directory will be parsed as stream configs.`[1:],
	)
	streamsDir = flag.String(
		"streams-dir", "",
		`
When running Benthos in streams mode any files in this directory with a .json or
.yaml extension will be parsed as a stream configuration (input, buffer,
pipeline, output), where the filename less the extension will be the id of the
stream.`[1:],
	)
	// Plugin Flags
	printInputPlugins     bool
	printOutputPlugins    bool
	printProcessorPlugins bool
	printConditionPlugins bool
	printCachePlugins     bool
	printRateLimitPlugins bool
)

func registerPluginFlags() {
	if input.PluginCount() > 0 {
		flag.BoolVar(
			&printInputPlugins, "list-input-plugins", false,
			"Print a list of available input plugins, then exit",
		)
	}
	if output.PluginCount() > 0 {
		flag.BoolVar(
			&printOutputPlugins, "list-output-plugins", false,
			"Print a list of available output plugins, then exit",
		)
	}
	if processor.PluginCount() > 0 {
		flag.BoolVar(
			&printProcessorPlugins, "list-processor-plugins", false,
			"Print a list of available processor plugins, then exit",
		)
	}
	if condition.PluginCount() > 0 {
		flag.BoolVar(
			&printConditionPlugins, "list-condition-plugins", false,
			"Print a list of available condition plugins, then exit",
		)
	}
	if cache.PluginCount() > 0 {
		flag.BoolVar(
			&printCachePlugins, "list-cache-plugins", false,
			"Print a list of available cache plugins, then exit",
		)
	}
	if ratelimit.PluginCount() > 0 {
		flag.BoolVar(
			&printRateLimitPlugins, "list-rate-limit-plugins", false,
			"Print a list of available ratelimit plugins, then exit",
		)
	}
}

//------------------------------------------------------------------------------

// bootstrap reads cmd args and either parses a config file or prints helper
// text and exits.
func bootstrap() (config.Type, []string) {
	conf := config.New()

	// A list of default config paths to check for if not explicitly defined
	defaultPaths := []string{
		"/benthos.yaml",
		"/etc/benthos/config.yaml",
		"/etc/benthos.yaml",
	}

	// Override default help printing
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: benthos [flags...]")
		fmt.Fprintln(os.Stderr, "Flags:")
		flag.PrintDefaults()
	}

	flag.Parse()

	// If the user wants the version we print it.
	if *showVersion {
		fmt.Printf("Version: %v\nDate: %v\n", Version, DateBuilt)
		os.Exit(0)
	}

	var lints []string
	if len(*configPath) > 0 {
		var err error
		if lints, err = config.Read(*configPath, true, &conf); err != nil {
			fmt.Fprintf(os.Stderr, "Configuration file read error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Iterate default config paths
		for _, path := range defaultPaths {
			if _, err := os.Stat(path); err == nil {
				fmt.Fprintf(os.Stderr, "Config file not specified, reading from %v\n", path)

				if lints, err = config.Read(path, true, &conf); err != nil {
					fmt.Fprintf(os.Stderr, "Configuration file read error: %v\n", err)
					os.Exit(1)
				}
				break
			}
		}
	}
	if *lintConfig {
		if len(lints) > 0 {
			for _, l := range lints {
				fmt.Fprintln(os.Stderr, l)
			}
			os.Exit(1)
		}
		os.Exit(0)
	}

	// If the user wants the configuration to be printed we do so and then exit.
	if *showConfigJSON || *showConfigYAML {
		var outConf interface{}
		var err error

		if len(*examples) > 0 {
			config.AddExamples(&conf, strings.Split(*examples, ",")...)
		}

		if !*showAll {
			if outConf, err = conf.Sanitised(); err != nil {
				fmt.Fprintln(os.Stderr, fmt.Sprintf("Configuration sanitise error: %v", err))
				os.Exit(1)
			}
		} else {
			if len(conf.Input.Processors) == 0 &&
				len(conf.Pipeline.Processors) == 0 &&
				len(conf.Output.Processors) == 0 {
				conf.Pipeline.Processors = append(conf.Pipeline.Processors, processor.NewConfig())
			}
			manager.AddExamples(&conf.Manager)
			outConf = conf
		}

		if *showConfigJSON {
			if configJSON, err := json.Marshal(outConf); err == nil {
				fmt.Println(string(configJSON))
			} else {
				fmt.Fprintln(os.Stderr, fmt.Sprintf("Configuration marshal error: %v", err))
			}
			os.Exit(0)
		} else {
			if configYAML, err := uconfig.MarshalYAML(outConf); err == nil {
				fmt.Println(string(configYAML))
			} else {
				fmt.Fprintln(os.Stderr, fmt.Sprintf("Configuration marshal error: %v", err))
			}
			os.Exit(0)
		}
	}

	// If we only want to print our inputs or outputs we should exit afterwards
	if *printInputs || *printOutputs || *printBuffers || *printProcessors ||
		*printConditions || *printCaches || *printRateLimits ||
		*printMetrics || *printTracers || printInputPlugins ||
		printOutputPlugins || printProcessorPlugins || printConditionPlugins ||
		printCachePlugins || printRateLimitPlugins {
		if *printInputs {
			fmt.Println(input.Descriptions())
		}
		if *printProcessors {
			fmt.Println(processor.Descriptions())
		}
		if *printConditions {
			fmt.Println(condition.Descriptions())
		}
		if *printRateLimits {
			fmt.Println(ratelimit.Descriptions())
		}
		if *printBuffers {
			fmt.Println(buffer.Descriptions())
		}
		if *printOutputs {
			fmt.Println(output.Descriptions())
		}
		if *printCaches {
			fmt.Println(cache.Descriptions())
		}
		if *printMetrics {
			fmt.Println(metrics.Descriptions())
		}
		if *printTracers {
			fmt.Println(tracer.Descriptions())
		}
		if printInputPlugins {
			fmt.Println(input.PluginDescriptions())
		}
		if printProcessorPlugins {
			fmt.Println(processor.PluginDescriptions())
		}
		if printConditionPlugins {
			fmt.Println(condition.PluginDescriptions())
		}
		if printRateLimitPlugins {
			fmt.Println(ratelimit.PluginDescriptions())
		}
		if printOutputPlugins {
			fmt.Println(output.PluginDescriptions())
		}
		if printCachePlugins {
			fmt.Println(cache.PluginDescriptions())
		}
		os.Exit(0)
	}

	return conf, lints
}

type stoppableStreams interface {
	Stop(timeout time.Duration) error
}

// Run the Benthos service, if the pipeline is started successfully then this
// call blocks until either the pipeline shuts down or a termination signal is
// received.
func Run() {
	registerPluginFlags()

	// Bootstrap by reading cmd flags and configuration file.
	config, lints := bootstrap()

	// Logging and stats aggregation.
	var logger log.Modular

	// Note: Only log to Stderr if one of our outputs is stdout.
	if config.Output.Type == "stdout" {
		logger = log.New(os.Stderr, config.Logger)
	} else {
		logger = log.New(os.Stdout, config.Logger)
	}

	if len(lints) > 0 {
		lintlog := logger.NewModule(".linter")
		for _, lint := range lints {
			if *strictConfig {
				lintlog.Errorln(lint)
			} else {
				lintlog.Infoln(lint)
			}
		}
		if *strictConfig {
			lintlog.Errorln("Shutting down due to --strict mode")
			os.Exit(1)
		}
	}

	// Create our metrics type.
	var stats metrics.Type
	var err error
	stats, err = metrics.New(config.Metrics, metrics.OptSetLogger(logger))
	for err != nil {
		logger.Errorf("Failed to connect to metrics aggregator: %v\n", err)
		<-time.After(time.Second)
		stats, err = metrics.New(config.Metrics, metrics.OptSetLogger(logger))
	}
	defer func() {
		if sCloseErr := stats.Close(); sCloseErr != nil {
			logger.Errorf("Failed to cleanly close metrics aggregator: %v\n", sCloseErr)
		}
	}()

	// Create our tracer type.
	var trac tracer.Type
	if trac, err = tracer.New(config.Tracer); err != nil {
		logger.Errorf("Failed to initialise tracer: %v\n", err)
		os.Exit(1)
	}
	defer trac.Close()

	// Create HTTP API with a sanitised service config.
	sanConf, err := config.Sanitised()
	if err != nil {
		logger.Warnf("Failed to generate sanitised config: %v\n", err)
	}
	var httpServer *api.Type
	if httpServer, err = api.New(Version, DateBuilt, config.HTTP, sanConf, logger, stats); err != nil {
		logger.Errorf("Failed to initialise API: %v\n", err)
		os.Exit(1)
	}

	// Create resource manager.
	manager, err := manager.New(config.Manager, httpServer, logger, stats)
	if err != nil {
		logger.Errorf("Failed to create resource: %v\n", err)
		os.Exit(1)
	}

	var dataStream stoppableStreams
	dataStreamClosedChan := make(chan struct{})

	// Create data streams.
	if *streamsMode {
		streamMgr := strmmgr.New(
			strmmgr.OptSetAPITimeout(time.Second*5),
			strmmgr.OptSetLogger(logger),
			strmmgr.OptSetManager(manager),
			strmmgr.OptSetStats(stats),
		)
		var streamConfs map[string]stream.Config
		if len(*streamsDir) > 0 {
			if streamConfs, err = strmmgr.LoadStreamConfigsFromDirectory(true, *streamsDir); err != nil {
				logger.Errorf("Failed to load stream configs: %v\n", err)
				os.Exit(1)
			}
		}
		dataStream = streamMgr
		for id, conf := range streamConfs {
			if err = streamMgr.Create(id, conf); err != nil {
				logger.Errorf("Failed to create stream (%v): %v\n", id, err)
				os.Exit(1)
			}
		}
		logger.Infoln("Launching benthos in streams mode, use CTRL+C to close.")
		if lStreams := len(streamConfs); lStreams > 0 {
			logger.Infof("Created %v streams from directory: %v\n", lStreams, *streamsDir)
		}
	} else {
		if dataStream, err = stream.New(
			config.Config,
			stream.OptSetLogger(logger),
			stream.OptSetStats(stats),
			stream.OptSetManager(manager),
			stream.OptOnClose(func() {
				close(dataStreamClosedChan)
			}),
		); err != nil {
			logger.Errorf("Service closing due to: %v\n", err)
			os.Exit(1)
		}
		logger.Infoln("Launching a benthos instance, use CTRL+C to close.")
	}

	// Start HTTP server.
	httpServerClosedChan := make(chan struct{})
	go func() {
		logger.Infof(
			"Listening for HTTP requests at: %v\n",
			"http://"+config.HTTP.Address,
		)
		httpErr := httpServer.ListenAndServe()
		if httpErr != nil && httpErr != http.ErrServerClosed {
			logger.Errorf("HTTP Server error: %v\n", httpErr)
		}
		close(httpServerClosedChan)
	}()

	var exitTimeout time.Duration
	if tout := config.SystemCloseTimeout; len(tout) > 0 {
		var err error
		if exitTimeout, err = time.ParseDuration(tout); err != nil {
			logger.Errorf("Failed to parse shutdown timeout period string: %v\n", err)
			os.Exit(1)
		}
	}

	// Defer clean up.
	defer func() {
		go func() {
			httpServer.Shutdown(context.Background())
			select {
			case <-httpServerClosedChan:
			case <-time.After(exitTimeout / 2):
				logger.Warnln("Service failed to close HTTP server gracefully in time.")
			}
		}()

		go func() {
			<-time.After(exitTimeout + time.Second)
			logger.Warnln(
				"Service failed to close cleanly within allocated time." +
					" Exiting forcefully and dumping stack trace to stderr.",
			)
			pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
			os.Exit(1)
		}()

		timesOut := time.Now().Add(exitTimeout)
		if err := dataStream.Stop(exitTimeout); err != nil {
			os.Exit(1)
		}
		manager.CloseAsync()
		if err := manager.WaitForClose(time.Until(timesOut)); err != nil {
			logger.Warnf(
				"Service failed to close cleanly within allocated time: %v."+
					" Exiting forcefully and dumping stack trace to stderr.\n", err,
			)
			pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
			os.Exit(1)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for termination signal
	select {
	case <-sigChan:
		logger.Infoln("Received SIGTERM, the service is closing.")
	case <-dataStreamClosedChan:
		logger.Infoln("Pipeline has terminated. Shutting down the service.")
	case <-httpServerClosedChan:
		logger.Infoln("HTTP Server has terminated. Shutting down the service.")
	}
}

//------------------------------------------------------------------------------
